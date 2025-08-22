package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/agentarium/core/llm"
)

// LLMAgent extends BaseAgent with LLM capabilities.
type LLMAgent struct {
	*BaseAgent
	llmProvider         llm.Provider  // Agent-specific LLM provider
	llmManager          llm.Manager   // Global LLM manager
	systemPrompt        string        // System prompt for the agent
	conversationHistory []llm.Message // Conversation memory
	maxHistory          int           // Maximum conversation history to keep
}

// LLMAgentConfig holds configuration for creating an LLM-enabled agent.
type LLMAgentConfig struct {
	BaseAgentConfig
	LLMProvider  llm.Provider // Optional: agent-specific LLM provider
	LLMManager   llm.Manager  // Global LLM manager
	SystemPrompt string       // System prompt for the agent
	MaxHistory   int          // Maximum conversation history (default: 20)
	LLMModel     string       // Specific model to use
	Temperature  float64      // LLM temperature setting
}

// NewLLMAgent creates a new LLM-enabled agent.
func NewLLMAgent(config LLMAgentConfig) *LLMAgent {
	// Set default max history
	if config.MaxHistory <= 0 {
		config.MaxHistory = 20
	}

	// Create base agent configuration
	baseConfig := config.BaseAgentConfig
	if baseConfig.OnMessage == nil {
		baseConfig.OnMessage = func(msg Message) error {
			// This will be overridden by the LLM agent
			return nil
		}
	}

	llmAgent := &LLMAgent{
		llmProvider:         config.LLMProvider,
		llmManager:          config.LLMManager,
		systemPrompt:        config.SystemPrompt,
		conversationHistory: make([]llm.Message, 0),
		maxHistory:          config.MaxHistory,
	}

	// Override the message handler to use LLM processing
	baseConfig.OnMessage = llmAgent.handleLLMMessage

	llmAgent.BaseAgent = NewBaseAgent(baseConfig)

	return llmAgent
}

// handleLLMMessage processes messages using the LLM.
func (a *LLMAgent) handleLLMMessage(msg Message) error {
	// Convert agent message to LLM message
	llmMsg := llm.NewUserMessage(fmt.Sprintf("%s", msg.Payload()))

	// Add to conversation history
	a.addToHistory(llmMsg)

	// Generate LLM response
	response, err := a.generateLLMResponse(context.Background())
	if err != nil {
		a.Logger().Error("Failed to generate LLM response",
			Field{Key: "error", Value: err},
			Field{Key: "message_id", Value: msg.ID()},
		)
		return err
	}

	// Add assistant response to history
	assistantMsg := llm.NewAssistantMessage(response.Content)
	a.addToHistory(assistantMsg)

	// Log the LLM response
	a.Logger().Info("LLM response generated",
		Field{Key: "agent_id", Value: a.ID()},
		Field{Key: "message_type", Value: msg.Type()},
		Field{Key: "response_length", Value: len(response.Content)},
		Field{Key: "model", Value: response.Model},
		Field{Key: "tokens_used", Value: response.Usage.TotalTokens},
	)

	// If this message came from another agent, we could send a response back
	if msg.Sender() != "system" {
		// For now, just log the response (in a real system, you'd route it)
		a.Logger().Debug("LLM response prepared",
			Field{Key: "response_to", Value: msg.Sender()},
			Field{Key: "content", Value: response.Content},
		)
	}

	return nil
}

// generateLLMResponse generates a response using the configured LLM.
func (a *LLMAgent) generateLLMResponse(ctx context.Context) (*llm.Response, error) {
	// Build the request
	request := llm.NewRequest().
		WithSystemPrompt(a.systemPrompt).
		WithMessages(a.conversationHistory...).
		WithTemperature(0.7).
		WithMaxTokens(1000).
		Build()

	// Use agent-specific provider if available, otherwise use global manager
	if a.llmProvider != nil {
		if !a.llmProvider.IsConfigured() {
			return nil, fmt.Errorf("agent LLM provider is not configured")
		}
		return a.llmProvider.GenerateResponse(ctx, request)
	}

	if a.llmManager != nil {
		return a.llmManager.GenerateResponse(ctx, "", request)
	}

	return nil, fmt.Errorf("no LLM provider available")
}

// GenerateStreamResponse generates a streaming response using the configured LLM.
func (a *LLMAgent) GenerateStreamResponse(ctx context.Context, userMessage string) (<-chan llm.StreamChunk, error) {
	// Add user message to history
	llmMsg := llm.NewUserMessage(userMessage)
	a.addToHistory(llmMsg)

	// Build the request
	request := llm.NewRequest().
		WithSystemPrompt(a.systemPrompt).
		WithMessages(a.conversationHistory...).
		WithTemperature(0.7).
		WithMaxTokens(1000).
		WithStreaming(true).
		Build()

	// Use agent-specific provider if available, otherwise use global manager
	if a.llmProvider != nil {
		if !a.llmProvider.IsConfigured() {
			return nil, fmt.Errorf("agent LLM provider is not configured")
		}
		return a.llmProvider.GenerateStreamResponse(ctx, request)
	}

	if a.llmManager != nil {
		// For streaming with manager, we need to get the default provider
		provider := a.llmManager.GetDefaultProvider()
		if provider == nil {
			return nil, fmt.Errorf("no default LLM provider available")
		}
		return provider.GenerateStreamResponse(ctx, request)
	}

	return nil, fmt.Errorf("no LLM provider available")
}

// Ask sends a message to the LLM and returns the response.
func (a *LLMAgent) Ask(ctx context.Context, question string) (string, error) {
	// Add user message to history
	llmMsg := llm.NewUserMessage(question)
	a.addToHistory(llmMsg)

	// Generate response
	response, err := a.generateLLMResponse(ctx)
	if err != nil {
		return "", err
	}

	// Add assistant response to history
	assistantMsg := llm.NewAssistantMessage(response.Content)
	a.addToHistory(assistantMsg)

	return response.Content, nil
}

// SetSystemPrompt updates the system prompt for the agent.
func (a *LLMAgent) SetSystemPrompt(prompt string) {
	a.systemPrompt = prompt
}

// GetSystemPrompt returns the current system prompt.
func (a *LLMAgent) GetSystemPrompt() string {
	return a.systemPrompt
}

// ClearHistory clears the conversation history.
func (a *LLMAgent) ClearHistory() {
	a.conversationHistory = make([]llm.Message, 0)
}

// GetHistory returns the current conversation history.
func (a *LLMAgent) GetHistory() []llm.Message {
	// Return a copy to prevent external modification
	history := make([]llm.Message, len(a.conversationHistory))
	copy(history, a.conversationHistory)
	return history
}

// addToHistory adds a message to the conversation history, maintaining the max history limit.
func (a *LLMAgent) addToHistory(msg llm.Message) {
	a.conversationHistory = append(a.conversationHistory, msg)

	// Trim history if it exceeds the maximum
	if len(a.conversationHistory) > a.maxHistory {
		// Remove oldest messages, but keep system messages
		var systemMessages []llm.Message
		var otherMessages []llm.Message

		for _, m := range a.conversationHistory {
			if m.Role == llm.RoleSystem {
				systemMessages = append(systemMessages, m)
			} else {
				otherMessages = append(otherMessages, m)
			}
		}

		// Keep system messages and the most recent other messages
		maxOtherMessages := a.maxHistory - len(systemMessages)
		if maxOtherMessages > 0 && len(otherMessages) > maxOtherMessages {
			otherMessages = otherMessages[len(otherMessages)-maxOtherMessages:]
		}

		// Rebuild history
		a.conversationHistory = make([]llm.Message, 0, len(systemMessages)+len(otherMessages))
		a.conversationHistory = append(a.conversationHistory, systemMessages...)
		a.conversationHistory = append(a.conversationHistory, otherMessages...)
	}
}

// GetLLMProvider returns the agent's LLM provider.
func (a *LLMAgent) GetLLMProvider() llm.Provider {
	return a.llmProvider
}

// SetLLMProvider sets the agent's LLM provider.
func (a *LLMAgent) SetLLMProvider(provider llm.Provider) {
	a.llmProvider = provider
}

// GetLLMManager returns the global LLM manager.
func (a *LLMAgent) GetLLMManager() llm.Manager {
	return a.llmManager
}

// SetLLMManager sets the global LLM manager.
func (a *LLMAgent) SetLLMManager(manager llm.Manager) {
	a.llmManager = manager
}

// LLMCapableAgent interface for agents that support LLM functionality.
type LLMCapableAgent interface {
	Agent

	// Ask sends a question to the LLM and returns the response
	Ask(ctx context.Context, question string) (string, error)

	// GenerateStreamResponse generates a streaming response
	GenerateStreamResponse(ctx context.Context, userMessage string) (<-chan llm.StreamChunk, error)

	// SetSystemPrompt updates the system prompt
	SetSystemPrompt(prompt string)

	// GetSystemPrompt returns the current system prompt
	GetSystemPrompt() string

	// ClearHistory clears the conversation history
	ClearHistory()

	// GetHistory returns the conversation history
	GetHistory() []llm.Message

	// GetLLMProvider returns the agent's LLM provider
	GetLLMProvider() llm.Provider

	// SetLLMProvider sets the agent's LLM provider
	SetLLMProvider(provider llm.Provider)

	// GetLLMManager returns the global LLM manager
	GetLLMManager() llm.Manager

	// SetLLMManager sets the global LLM manager
	SetLLMManager(manager llm.Manager)
}

// LLMAgentFactory creates LLM-enabled agents.
type LLMAgentFactory struct {
	llmManager    llm.Manager
	defaultConfig LLMAgentConfig
}

// NewLLMAgentFactory creates a new LLM agent factory.
func NewLLMAgentFactory(llmManager llm.Manager, defaultConfig LLMAgentConfig) AgentFactory {
	defaultConfig.LLMManager = llmManager
	return &LLMAgentFactory{
		llmManager:    llmManager,
		defaultConfig: defaultConfig,
	}
}

// Create creates a new LLM agent with the given configuration.
func (f *LLMAgentFactory) Create(config Config) (Agent, error) {
	agentConfig := f.defaultConfig

	// Override with config values
	if name := config.GetString("name"); name != "" {
		agentConfig.Name = name
	}

	if systemPrompt := config.GetString("system_prompt"); systemPrompt != "" {
		agentConfig.SystemPrompt = systemPrompt
	}

	if model := config.GetString("llm_model"); model != "" {
		agentConfig.LLMModel = model
	}

	if temp := config.Get("temperature"); temp != nil {
		if t, ok := temp.(float64); ok {
			agentConfig.Temperature = t
		}
	}

	if maxHistory := config.GetInt("max_history"); maxHistory > 0 {
		agentConfig.MaxHistory = maxHistory
	}

	if mailboxSize := config.GetInt("mailbox_size"); mailboxSize > 0 {
		agentConfig.MailboxSize = mailboxSize
	}

	// Check if a specific LLM provider is requested
	if providerName := config.GetString("llm_provider"); providerName != "" && f.llmManager != nil {
		provider, err := f.llmManager.GetProvider(providerName)
		if err != nil {
			return nil, fmt.Errorf("failed to get LLM provider '%s': %w", providerName, err)
		}
		agentConfig.LLMProvider = provider
	}

	return NewLLMAgent(agentConfig), nil
}

// Type returns the type of agent this factory creates.
func (f *LLMAgentFactory) Type() string {
	return "llm_agent"
}

// Helper function to create a simple LLM agent with minimal configuration
func NewSimpleLLMAgent(name string, systemPrompt string, llmManager llm.Manager) *LLMAgent {
	config := LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name:        name,
			MailboxSize: 50,
			Logger:      NewDefaultLogger(),
			Config:      NewMapConfig(),
		},
		LLMManager:   llmManager,
		SystemPrompt: systemPrompt,
		MaxHistory:   20,
		Temperature:  0.7,
	}

	return NewLLMAgent(config)
}

// ParseLLMMessageContent extracts structured content from LLM messages
func ParseLLMMessageContent(content string) map[string]interface{} {
	result := make(map[string]interface{})

	// Simple parsing - look for key-value pairs
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[key] = value
			}
		}
	}

	// If no structured content found, return the full content
	if len(result) == 0 {
		result["content"] = content
	}

	return result
}
