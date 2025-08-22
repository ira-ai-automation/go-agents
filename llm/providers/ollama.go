package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agentarium/core/llm"
)

// OllamaProvider implements the LLM Provider interface for Ollama (local LLMs).
type OllamaProvider struct {
	config     llm.Config
	httpClient *http.Client
}

// Ollama API request/response structures
type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages,omitempty"`
	Prompt   string          `json:"prompt,omitempty"`
	System   string          `json:"system,omitempty"`
	Template string          `json:"template,omitempty"`
	Context  []int           `json:"context,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
	Raw      bool            `json:"raw,omitempty"`
	Format   string          `json:"format,omitempty"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Model     string         `json:"model"`
	CreatedAt string         `json:"created_at"`
	Message   *ollamaMessage `json:"message,omitempty"`
	Response  string         `json:"response,omitempty"`
	Done      bool           `json:"done"`
	Context   []int          `json:"context,omitempty"`
	Error     string         `json:"error,omitempty"`

	// Streaming fields
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(config llm.Config) *OllamaProvider {
	if config == nil {
		config = llm.NewProviderConfig()
		// Set default model
		config.Set("model", "llama3.2")
		// Set default base URL for local Ollama
		config.Set("base_url", "http://localhost:11434")
	}

	return &OllamaProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.GetTimeout(),
		},
	}
}

// Name returns the name of the LLM provider.
func (p *OllamaProvider) Name() string {
	return llm.ProviderOllama
}

// IsConfigured returns true if the provider is properly configured.
func (p *OllamaProvider) IsConfigured() bool {
	// Ollama doesn't require API keys, just check if we can reach the server
	return p.config.GetBaseURL() != ""
}

// GetCapabilities returns the capabilities of this provider.
func (p *OllamaProvider) GetCapabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:       true,
		FunctionCalling: false, // Ollama doesn't support function calling directly
		Vision:          false, // Depends on the model
		MaxTokens:       8192,  // Varies by model
		SupportedModels: []string{
			"llama3.2",
			"llama3.1",
			"llama3",
			"llama2",
			"codellama",
			"mistral",
			"phi3",
			"qwen2",
			"gemma2",
		},
		DefaultModel: "llama3.2",
	}
}

// GenerateResponse generates a response to the given prompt.
func (p *OllamaProvider) GenerateResponse(ctx context.Context, request llm.Request) (*llm.Response, error) {
	// Convert to Ollama format
	ollamaReq := p.convertRequest(request)

	// Make API call
	respData, err := p.makeAPICall(ctx, "/api/chat", ollamaReq)
	if err != nil {
		return nil, err
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respData, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	if ollamaResp.Error != "" {
		return nil, fmt.Errorf("Ollama API error: %s", ollamaResp.Error)
	}

	// Convert response
	return p.convertResponse(ollamaResp), nil
}

// GenerateStreamResponse generates a streaming response to the given prompt.
func (p *OllamaProvider) GenerateStreamResponse(ctx context.Context, request llm.Request) (<-chan llm.StreamChunk, error) {
	// Convert to Ollama format and enable streaming
	ollamaReq := p.convertRequest(request)
	ollamaReq.Stream = true

	// Create response channel
	respChan := make(chan llm.StreamChunk, 10)

	// Start streaming in a goroutine
	go func() {
		defer close(respChan)

		if err := p.streamAPICall(ctx, "/api/chat", ollamaReq, respChan); err != nil {
			respChan <- llm.StreamChunk{
				Error: err,
			}
		}
	}()

	return respChan, nil
}

// convertRequest converts a generic request to Ollama format.
func (p *OllamaProvider) convertRequest(request llm.Request) ollamaRequest {
	ollamaReq := ollamaRequest{
		Model:  request.Model,
		Stream: request.Stream,
		System: request.SystemPrompt,
		Options: ollamaOptions{
			Temperature: request.Temperature,
			TopP:        request.TopP,
		},
	}

	// Use default model if not specified
	if ollamaReq.Model == "" {
		ollamaReq.Model = p.config.GetModel()
		if ollamaReq.Model == "" {
			ollamaReq.Model = "llama3.2"
		}
	}

	// Set num_predict (max tokens) if specified
	if request.MaxTokens > 0 {
		ollamaReq.Options.NumPredict = request.MaxTokens
	}

	// Convert messages
	ollamaReq.Messages = make([]ollamaMessage, 0, len(request.Messages))
	for _, msg := range request.Messages {
		if msg.Role != "system" { // System prompt is handled separately
			ollamaReq.Messages = append(ollamaReq.Messages, ollamaMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return ollamaReq
}

// convertResponse converts an Ollama response to generic format.
func (p *OllamaProvider) convertResponse(ollamaResp ollamaResponse) *llm.Response {
	content := ""
	role := "assistant"

	if ollamaResp.Message != nil {
		content = ollamaResp.Message.Content
		role = ollamaResp.Message.Role
	} else if ollamaResp.Response != "" {
		content = ollamaResp.Response
	}

	finishReason := llm.FinishReasonStop
	if ollamaResp.Done {
		finishReason = llm.FinishReasonStop
	}

	// Calculate approximate token usage (Ollama doesn't always provide this)
	promptTokens := ollamaResp.PromptEvalCount
	completionTokens := ollamaResp.EvalCount

	response := &llm.Response{
		Content:      content,
		Role:         role,
		FinishReason: finishReason,
		Model:        ollamaResp.Model,
		Usage: llm.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
		Metadata: map[string]interface{}{
			"created_at":           ollamaResp.CreatedAt,
			"total_duration":       ollamaResp.TotalDuration,
			"load_duration":        ollamaResp.LoadDuration,
			"prompt_eval_duration": ollamaResp.PromptEvalDuration,
			"eval_duration":        ollamaResp.EvalDuration,
		},
	}

	return response
}

// makeAPICall makes a standard API call to Ollama.
func (p *OllamaProvider) makeAPICall(ctx context.Context, endpoint string, requestData interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

// streamAPICall makes a streaming API call to Ollama.
func (p *OllamaProvider) streamAPICall(ctx context.Context, endpoint string, requestData interface{}, respChan chan<- llm.StreamChunk) error {
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := p.config.GetBaseURL()
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

// processStream processes the streaming response from Ollama.
func (p *OllamaProvider) processStream(reader io.Reader, respChan chan<- llm.StreamChunk) error {
	decoder := json.NewDecoder(reader)

	for {
		var ollamaResp ollamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if ollamaResp.Error != "" {
			return fmt.Errorf("Ollama stream error: %s", ollamaResp.Error)
		}

		content := ""
		if ollamaResp.Message != nil {
			content = ollamaResp.Message.Content
		} else if ollamaResp.Response != "" {
			content = ollamaResp.Response
		}

		chunk := llm.StreamChunk{
			Content: content,
			Delta: llm.Delta{
				Content: content,
			},
		}

		if ollamaResp.Done {
			chunk.FinishReason = llm.FinishReasonStop
		}

		select {
		case respChan <- chunk:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timeout sending chunk")
		}

		if ollamaResp.Done {
			break
		}
	}

	return nil
}
