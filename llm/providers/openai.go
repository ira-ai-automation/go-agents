// Package providers contains implementations of LLM providers.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/agentarium/core/llm"
)

// OpenAIProvider implements the LLM Provider interface for OpenAI API.
type OpenAIProvider struct {
	config     llm.Config
	httpClient *http.Client
}

// OpenAI API request/response structures
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []openAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	User        string          `json:"user,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
	Error   *openAIError   `json:"error,omitempty"`
}

type openAIChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message"`
	Delta        *openAIMessage `json:"delta,omitempty"`
	FinishReason string         `json:"finish_reason"`
	LogProbs     interface{}    `json:"logprobs"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(config llm.Config) *OpenAIProvider {
	if config == nil {
		config = llm.NewProviderConfig()
		// Try to get API key from environment
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			config.Set("api_key", apiKey)
		}
		// Set default model
		config.Set("model", "gpt-3.5-turbo")
		// Set default base URL
		config.Set("base_url", "https://api.openai.com/v1")
	}

	return &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.GetTimeout(),
		},
	}
}

// Name returns the name of the LLM provider.
func (p *OpenAIProvider) Name() string {
	return llm.ProviderOpenAI
}

// IsConfigured returns true if the provider is properly configured.
func (p *OpenAIProvider) IsConfigured() bool {
	return p.config.GetAPIKey() != ""
}

// GetCapabilities returns the capabilities of this provider.
func (p *OpenAIProvider) GetCapabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:       true,
		FunctionCalling: true,
		Vision:          true,
		MaxTokens:       128000, // GPT-4 context length
		SupportedModels: []string{
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
			"gpt-3.5-turbo-16k",
		},
		DefaultModel: "gpt-3.5-turbo",
	}
}

// GenerateResponse generates a response to the given prompt.
func (p *OpenAIProvider) GenerateResponse(ctx context.Context, request llm.Request) (*llm.Response, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI provider is not configured")
	}

	// Convert to OpenAI format
	openAIReq := p.convertRequest(request)

	// Make API call
	respData, err := p.makeAPICall(ctx, "/chat/completions", openAIReq)
	if err != nil {
		return nil, err
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respData, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	// Convert response
	return p.convertResponse(openAIResp), nil
}

// GenerateStreamResponse generates a streaming response to the given prompt.
func (p *OpenAIProvider) GenerateStreamResponse(ctx context.Context, request llm.Request) (<-chan llm.StreamChunk, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI provider is not configured")
	}

	// Convert to OpenAI format and enable streaming
	openAIReq := p.convertRequest(request)
	openAIReq.Stream = true

	// Create response channel
	respChan := make(chan llm.StreamChunk, 10)

	// Start streaming in a goroutine
	go func() {
		defer close(respChan)

		if err := p.streamAPICall(ctx, "/chat/completions", openAIReq, respChan); err != nil {
			respChan <- llm.StreamChunk{
				Error: err,
			}
		}
	}()

	return respChan, nil
}

// convertRequest converts a generic request to OpenAI format.
func (p *OpenAIProvider) convertRequest(request llm.Request) openAIRequest {
	openAIReq := openAIRequest{
		Model:       request.Model,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream,
	}

	// Use default model if not specified
	if openAIReq.Model == "" {
		openAIReq.Model = p.config.GetModel()
		if openAIReq.Model == "" {
			openAIReq.Model = "gpt-3.5-turbo"
		}
	}

	// Set max tokens
	if request.MaxTokens > 0 {
		openAIReq.MaxTokens = request.MaxTokens
	}

	// Convert messages
	openAIReq.Messages = make([]openAIMessage, 0, len(request.Messages))

	// Add system prompt as system message if provided
	if request.SystemPrompt != "" {
		openAIReq.Messages = append(openAIReq.Messages, openAIMessage{
			Role:    "system",
			Content: request.SystemPrompt,
		})
	}

	// Convert regular messages
	for _, msg := range request.Messages {
		openAIMsg := openAIMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
		}

		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			openAIMsg.ToolCalls = make([]openAIToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				openAIMsg.ToolCalls[i] = openAIToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: openAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		openAIReq.Messages = append(openAIReq.Messages, openAIMsg)
	}

	return openAIReq
}

// convertResponse converts an OpenAI response to generic format.
func (p *OpenAIProvider) convertResponse(openAIResp openAIResponse) *llm.Response {
	choice := openAIResp.Choices[0]

	response := &llm.Response{
		Content:      choice.Message.Content,
		Role:         choice.Message.Role,
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
		Usage: llm.Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
		Metadata: map[string]interface{}{
			"id":      openAIResp.ID,
			"object":  openAIResp.Object,
			"created": openAIResp.Created,
		},
	}

	// Convert tool calls
	if len(choice.Message.ToolCalls) > 0 {
		response.ToolCalls = make([]llm.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			response.ToolCalls[i] = llm.ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: llm.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return response
}

// makeAPICall makes a standard API call to OpenAI.
func (p *OpenAIProvider) makeAPICall(ctx context.Context, endpoint string, requestData interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.GetAPIKey())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// streamAPICall makes a streaming API call to OpenAI.
func (p *OpenAIProvider) streamAPICall(ctx context.Context, endpoint string, requestData interface{}, respChan chan<- llm.StreamChunk) error {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.GetAPIKey())
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Process streaming response
	return p.processStream(resp.Body, respChan)
}

// processStream processes the streaming response from OpenAI.
func (p *OpenAIProvider) processStream(reader io.Reader, respChan chan<- llm.StreamChunk) error {
	buf := make([]byte, 4096)
	var incomplete string

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		data := incomplete + string(buf[:n])
		lines := strings.Split(data, "\n")

		// Keep the last potentially incomplete line
		incomplete = lines[len(lines)-1]
		lines = lines[:len(lines)-1]

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")

				var streamResp openAIResponse
				if err := json.Unmarshal([]byte(jsonStr), &streamResp); err != nil {
					continue // Skip malformed chunks
				}

				if len(streamResp.Choices) > 0 {
					choice := streamResp.Choices[0]
					chunk := llm.StreamChunk{
						Index:        choice.Index,
						FinishReason: choice.FinishReason,
					}

					if choice.Delta != nil {
						chunk.Content = choice.Delta.Content
						chunk.Delta = llm.Delta{
							Role:    choice.Delta.Role,
							Content: choice.Delta.Content,
						}

						// Convert tool calls in delta
						if len(choice.Delta.ToolCalls) > 0 {
							chunk.Delta.ToolCalls = make([]llm.ToolCall, len(choice.Delta.ToolCalls))
							for i, tc := range choice.Delta.ToolCalls {
								chunk.Delta.ToolCalls[i] = llm.ToolCall{
									ID:   tc.ID,
									Type: tc.Type,
									Function: llm.FunctionCall{
										Name:      tc.Function.Name,
										Arguments: tc.Function.Arguments,
									},
								}
							}
						}
					}

					select {
					case respChan <- chunk:
					case <-time.After(5 * time.Second):
						return fmt.Errorf("timeout sending chunk")
					}
				}
			}
		}
	}

	return nil
}
