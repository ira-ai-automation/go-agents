// Package llm provides interfaces and implementations for Large Language Model integrations.
package llm

import (
	"context"
	"time"
)

// Provider represents an LLM provider interface that all LLM implementations must satisfy.
type Provider interface {
	// Name returns the name of the LLM provider
	Name() string

	// GenerateResponse generates a response to the given prompt
	GenerateResponse(ctx context.Context, request Request) (*Response, error)

	// GenerateStreamResponse generates a streaming response to the given prompt
	GenerateStreamResponse(ctx context.Context, request Request) (<-chan StreamChunk, error)

	// IsConfigured returns true if the provider is properly configured
	IsConfigured() bool

	// GetCapabilities returns the capabilities of this provider
	GetCapabilities() Capabilities
}

// Request represents a request to an LLM provider.
type Request struct {
	// Messages contains the conversation history
	Messages []Message `json:"messages"`

	// SystemPrompt is an optional system prompt
	SystemPrompt string `json:"system_prompt,omitempty"`

	// Model specifies which model to use (provider-specific)
	Model string `json:"model,omitempty"`

	// MaxTokens limits the response length
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls randomness (0.0 to 1.0)
	Temperature float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling
	TopP float64 `json:"top_p,omitempty"`

	// Stream indicates if streaming response is requested
	Stream bool `json:"stream,omitempty"`

	// Metadata for additional provider-specific parameters
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Message represents a single message in a conversation.
type Message struct {
	// Role of the message sender (system, user, assistant, tool)
	Role string `json:"role"`

	// Content of the message
	Content string `json:"content"`

	// Name of the speaker (optional)
	Name string `json:"name,omitempty"`

	// ToolCalls for function calling (if supported)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID for tool responses
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call request.
type ToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function FunctionCall           `json:"function"`
	Args     map[string]interface{} `json:"args,omitempty"`
}

// FunctionCall represents a function call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Response represents a response from an LLM provider.
type Response struct {
	// Content is the generated text response
	Content string `json:"content"`

	// Role of the response (usually "assistant")
	Role string `json:"role"`

	// FinishReason indicates why the generation stopped
	FinishReason string `json:"finish_reason"`

	// Usage statistics
	Usage Usage `json:"usage"`

	// Model used for generation
	Model string `json:"model"`

	// ToolCalls if function calling was used
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Metadata for provider-specific response data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StreamChunk represents a chunk in a streaming response.
type StreamChunk struct {
	// Content is the incremental text content
	Content string `json:"content"`

	// Delta contains the changes from previous chunk
	Delta Delta `json:"delta"`

	// FinishReason indicates if this is the final chunk
	FinishReason string `json:"finish_reason,omitempty"`

	// Index of the choice (for multiple completions)
	Index int `json:"index"`

	// Error if there was an issue with this chunk
	Error error `json:"error,omitempty"`
}

// Delta represents incremental changes in streaming.
type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Capabilities represents what an LLM provider supports.
type Capabilities struct {
	// Streaming indicates if the provider supports streaming responses
	Streaming bool `json:"streaming"`

	// FunctionCalling indicates if the provider supports function calling
	FunctionCalling bool `json:"function_calling"`

	// Vision indicates if the provider supports image inputs
	Vision bool `json:"vision"`

	// MaxTokens is the maximum number of tokens supported
	MaxTokens int `json:"max_tokens"`

	// SupportedModels lists available models
	SupportedModels []string `json:"supported_models"`

	// DefaultModel is the default model for this provider
	DefaultModel string `json:"default_model"`
}

// Config represents configuration for an LLM provider.
type Config interface {
	// GetAPIKey returns the API key for the provider
	GetAPIKey() string

	// GetBaseURL returns the base URL for API calls
	GetBaseURL() string

	// GetModel returns the default model to use
	GetModel() string

	// GetTimeout returns the request timeout
	GetTimeout() time.Duration

	// GetRetryAttempts returns the number of retry attempts
	GetRetryAttempts() int

	// GetMetadata returns additional configuration metadata
	GetMetadata() map[string]interface{}

	// Set sets a configuration value
	Set(key string, value interface{})

	// Get gets a configuration value
	Get(key string) interface{}
}

// Manager manages multiple LLM providers and handles provider selection.
type Manager interface {
	// RegisterProvider registers an LLM provider
	RegisterProvider(provider Provider) error

	// GetProvider returns a provider by name
	GetProvider(name string) (Provider, error)

	// GetDefaultProvider returns the default provider
	GetDefaultProvider() Provider

	// SetDefaultProvider sets the default provider
	SetDefaultProvider(name string) error

	// ListProviders returns all registered providers
	ListProviders() []string

	// GenerateResponse generates a response using the specified or default provider
	GenerateResponse(ctx context.Context, providerName string, request Request) (*Response, error)
}

// Common message roles
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Common finish reasons
const (
	FinishReasonStop          = "stop"
	FinishReasonLength        = "length"
	FinishReasonToolCalls     = "tool_calls"
	FinishReasonContentFilter = "content_filter"
	FinishReasonError         = "error"
)

// Common provider names
const (
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
	ProviderLlama  = "llama"
	ProviderOllama = "ollama"
	ProviderGroq   = "groq"
)

// Helper functions for creating messages
func NewSystemMessage(content string) Message {
	return Message{
		Role:    RoleSystem,
		Content: content,
	}
}

func NewUserMessage(content string) Message {
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

func NewAssistantMessage(content string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: content,
	}
}

func NewToolMessage(content, toolCallID string) Message {
	return Message{
		Role:       RoleTool,
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// RequestBuilder helps build LLM requests with a fluent API.
type RequestBuilder struct {
	request Request
}

// NewRequest creates a new request builder.
func NewRequest() *RequestBuilder {
	return &RequestBuilder{
		request: Request{
			Messages:    make([]Message, 0),
			Temperature: 0.7,
			Metadata:    make(map[string]interface{}),
		},
	}
}

// WithMessage adds a message to the request.
func (b *RequestBuilder) WithMessage(msg Message) *RequestBuilder {
	b.request.Messages = append(b.request.Messages, msg)
	return b
}

// WithMessages adds multiple messages to the request.
func (b *RequestBuilder) WithMessages(msgs ...Message) *RequestBuilder {
	b.request.Messages = append(b.request.Messages, msgs...)
	return b
}

// WithSystemPrompt sets the system prompt.
func (b *RequestBuilder) WithSystemPrompt(prompt string) *RequestBuilder {
	b.request.SystemPrompt = prompt
	return b
}

// WithModel sets the model to use.
func (b *RequestBuilder) WithModel(model string) *RequestBuilder {
	b.request.Model = model
	return b
}

// WithMaxTokens sets the maximum tokens.
func (b *RequestBuilder) WithMaxTokens(maxTokens int) *RequestBuilder {
	b.request.MaxTokens = maxTokens
	return b
}

// WithTemperature sets the temperature.
func (b *RequestBuilder) WithTemperature(temp float64) *RequestBuilder {
	b.request.Temperature = temp
	return b
}

// WithTopP sets the top-p value.
func (b *RequestBuilder) WithTopP(topP float64) *RequestBuilder {
	b.request.TopP = topP
	return b
}

// WithStreaming enables or disables streaming.
func (b *RequestBuilder) WithStreaming(stream bool) *RequestBuilder {
	b.request.Stream = stream
	return b
}

// WithMetadata adds metadata to the request.
func (b *RequestBuilder) WithMetadata(key string, value interface{}) *RequestBuilder {
	b.request.Metadata[key] = value
	return b
}

// Build creates the final request.
func (b *RequestBuilder) Build() Request {
	return b.request
}
