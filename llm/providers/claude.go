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

	"github.com/ira-ai-automation/go-agents/llm"
)

// ClaudeProvider implements the LLM Provider interface for Anthropic's Claude API.
type ClaudeProvider struct {
	config     llm.Config
	httpClient *http.Client
}

// Claude API request/response structures
type claudeRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []claudeMessage `json:"messages"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Stop        []string        `json:"stop_sequences,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	Content      []claudeBlock `json:"content"`
	Model        string        `json:"model"`
	StopReason   string        `json:"stop_reason"`
	StopSequence string        `json:"stop_sequence"`
	Usage        claudeUsage   `json:"usage"`
	Error        *claudeError  `json:"error,omitempty"`
}

type claudeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type claudeStreamEvent struct {
	Type  string       `json:"type"`
	Index int          `json:"index,omitempty"`
	Delta *claudeBlock `json:"delta,omitempty"`
	Usage *claudeUsage `json:"usage,omitempty"`
	Error *claudeError `json:"error,omitempty"`
}

// NewClaudeProvider creates a new Claude provider.
func NewClaudeProvider(config llm.Config) *ClaudeProvider {
	if config == nil {
		config = llm.NewProviderConfig()
		// Try to get API key from environment
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			config.Set("api_key", apiKey)
		}
		// Set default model
		config.Set("model", "claude-3-haiku-20240307")
		// Set default base URL
		config.Set("base_url", "https://api.anthropic.com/v1")
	}

	return &ClaudeProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.GetTimeout(),
		},
	}
}

// Name returns the name of the LLM provider.
func (p *ClaudeProvider) Name() string {
	return llm.ProviderClaude
}

// IsConfigured returns true if the provider is properly configured.
func (p *ClaudeProvider) IsConfigured() bool {
	return p.config.GetAPIKey() != ""
}

// GetCapabilities returns the capabilities of this provider.
func (p *ClaudeProvider) GetCapabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:       true,
		FunctionCalling: false, // Claude doesn't support function calling in the same way
		Vision:          true,
		MaxTokens:       200000, // Claude-3 context length
		SupportedModels: []string{
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
			"claude-3-opus-20240229",
			"claude-3-sonnet-20240229",
			"claude-3-haiku-20240307",
		},
		DefaultModel: "claude-3-haiku-20240307",
	}
}

// GenerateResponse generates a response to the given prompt.
func (p *ClaudeProvider) GenerateResponse(ctx context.Context, request llm.Request) (*llm.Response, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Claude provider is not configured")
	}

	// Convert to Claude format
	claudeReq := p.convertRequest(request)

	// Make API call
	respData, err := p.makeAPICall(ctx, "/messages", claudeReq)
	if err != nil {
		return nil, err
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respData, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("Claude API error: %s", claudeResp.Error.Message)
	}

	// Convert response
	return p.convertResponse(claudeResp), nil
}

// GenerateStreamResponse generates a streaming response to the given prompt.
func (p *ClaudeProvider) GenerateStreamResponse(ctx context.Context, request llm.Request) (<-chan llm.StreamChunk, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Claude provider is not configured")
	}

	// Convert to Claude format and enable streaming
	claudeReq := p.convertRequest(request)
	claudeReq.Stream = true

	// Create response channel
	respChan := make(chan llm.StreamChunk, 10)

	// Start streaming in a goroutine
	go func() {
		defer close(respChan)

		if err := p.streamAPICall(ctx, "/messages", claudeReq, respChan); err != nil {
			respChan <- llm.StreamChunk{
				Error: err,
			}
		}
	}()

	return respChan, nil
}

// convertRequest converts a generic request to Claude format.
func (p *ClaudeProvider) convertRequest(request llm.Request) claudeRequest {
	claudeReq := claudeRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream,
		System:      request.SystemPrompt,
	}

	// Use default model if not specified
	if claudeReq.Model == "" {
		claudeReq.Model = p.config.GetModel()
		if claudeReq.Model == "" {
			claudeReq.Model = "claude-3-haiku-20240307"
		}
	}

	// Set default max tokens if not specified
	if claudeReq.MaxTokens == 0 {
		claudeReq.MaxTokens = 1024
	}

	// Convert messages (filter out system messages as they go in system field)
	claudeReq.Messages = make([]claudeMessage, 0, len(request.Messages))
	for _, msg := range request.Messages {
		if msg.Role != "system" { // System messages are handled separately in Claude
			claudeReq.Messages = append(claudeReq.Messages, claudeMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return claudeReq
}

// convertResponse converts a Claude response to generic format.
func (p *ClaudeProvider) convertResponse(claudeResp claudeResponse) *llm.Response {
	content := ""
	if len(claudeResp.Content) > 0 && claudeResp.Content[0].Type == "text" {
		content = claudeResp.Content[0].Text
	}

	// Map Claude stop reasons to standard finish reasons
	finishReason := claudeResp.StopReason
	switch finishReason {
	case "end_turn":
		finishReason = llm.FinishReasonStop
	case "max_tokens":
		finishReason = llm.FinishReasonLength
	}

	response := &llm.Response{
		Content:      content,
		Role:         claudeResp.Role,
		FinishReason: finishReason,
		Model:        claudeResp.Model,
		Usage: llm.Usage{
			PromptTokens:     claudeResp.Usage.InputTokens,
			CompletionTokens: claudeResp.Usage.OutputTokens,
			TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		},
		Metadata: map[string]interface{}{
			"id":            claudeResp.ID,
			"type":          claudeResp.Type,
			"stop_sequence": claudeResp.StopSequence,
		},
	}

	return response
}

// makeAPICall makes a standard API call to Claude.
func (p *ClaudeProvider) makeAPICall(ctx context.Context, endpoint string, requestData interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.GetAPIKey())
	req.Header.Set("anthropic-version", "2023-06-01")

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

// streamAPICall makes a streaming API call to Claude.
func (p *ClaudeProvider) streamAPICall(ctx context.Context, endpoint string, requestData interface{}, respChan chan<- llm.StreamChunk) error {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.GetAPIKey())
	req.Header.Set("anthropic-version", "2023-06-01")
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

// processStream processes the streaming response from Claude.
func (p *ClaudeProvider) processStream(reader io.Reader, respChan chan<- llm.StreamChunk) error {
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
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")

				var streamEvent claudeStreamEvent
				if err := json.Unmarshal([]byte(jsonStr), &streamEvent); err != nil {
					continue // Skip malformed chunks
				}

				chunk := llm.StreamChunk{
					Index: streamEvent.Index,
				}

				switch streamEvent.Type {
				case "content_block_delta":
					if streamEvent.Delta != nil && streamEvent.Delta.Type == "text_delta" {
						chunk.Content = streamEvent.Delta.Text
						chunk.Delta = llm.Delta{
							Content: streamEvent.Delta.Text,
						}
					}
				case "message_stop":
					chunk.FinishReason = llm.FinishReasonStop
				}

				if streamEvent.Error != nil {
					chunk.Error = fmt.Errorf("Claude stream error: %s", streamEvent.Error.Message)
				}

				select {
				case respChan <- chunk:
				case <-time.After(5 * time.Second):
					return fmt.Errorf("timeout sending chunk")
				}
			}
		}
	}

	return nil
}
