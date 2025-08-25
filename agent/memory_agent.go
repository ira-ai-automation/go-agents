package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ira-ai-automation/go-agents/llm"
	"github.com/ira-ai-automation/go-agents/memory"
)

// MemoryLLMAgent extends LLMAgent with advanced memory capabilities including
// persistent storage, vector search, and retrieval-augmented generation.
type MemoryLLMAgent struct {
	*LLMAgent
	memoryManager memory.MemoryManager
	memory        memory.CompositeMemory
	config        MemoryAgentConfig
}

// MemoryAgentConfig holds configuration for memory-enhanced agents.
type MemoryAgentConfig struct {
	LLMAgentConfig
	MemoryConfig                memory.MemoryConfig
	EnableRAG                   bool     // Enable Retrieval-Augmented Generation
	RAGContextSize              int      // Number of similar memories to include in context
	RAGSimilarityThreshold      float32  // Minimum similarity score for RAG
	AutoSaveConversations       bool     // Automatically save conversation history
	ConversationSummaryInterval int      // Summarize conversations every N messages
	KnowledgeCategories         []string // Categories for organizing knowledge
	MaxMemoryContextLength      int      // Maximum context length from memory
	MemorySearchEnabled         bool     // Enable semantic memory search
}

// NewMemoryLLMAgent creates a new memory-enhanced LLM agent.
func NewMemoryLLMAgent(config MemoryAgentConfig) (*MemoryLLMAgent, error) {
	// Create base LLM agent
	llmAgent := NewLLMAgent(config.LLMAgentConfig)

	// Create memory manager if not provided
	var memManager memory.MemoryManager
	if config.MemoryConfig.EmbeddingProvider == nil {
		// Use local embedding provider as default
		embeddingProvider := memory.NewLocalEmbeddingProvider(384)
		config.MemoryConfig.EmbeddingProvider = embeddingProvider
	}

	memManager = memory.NewMemoryManager()

	// Create memory store for this agent
	agentMemory, err := memManager.CreateMemory(llmAgent.ID(), config.MemoryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory for agent: %w", err)
	}

	memoryAgent := &MemoryLLMAgent{
		LLMAgent:      llmAgent,
		memoryManager: memManager,
		memory:        agentMemory,
		config:        config,
	}

	// Override the base agent's message handler to include memory processing
	config.LLMAgentConfig.BaseAgentConfig.OnMessage = memoryAgent.handleMemoryMessage

	return memoryAgent, nil
}

// handleMemoryMessage processes messages with memory integration.
func (a *MemoryLLMAgent) handleMemoryMessage(msg Message) error {
	ctx := context.Background()

	// Convert message to memory format
	userMessage := fmt.Sprintf("%s", msg.Payload())

	// Store the incoming message in memory
	if a.config.AutoSaveConversations {
		messageKey := fmt.Sprintf("msg_%d", time.Now().UnixNano())
		messageData := memory.Message{
			ID:        msg.ID(),
			Role:      "user",
			Content:   userMessage,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"sender": msg.Sender(),
				"type":   msg.Type(),
			},
		}

		if err := a.memory.StoreWithEmbedding(ctx, messageKey, messageData); err != nil {
			a.Logger().Warn("Failed to store message in memory", Field{Key: "error", Value: err})
		}
	}

	// Generate LLM response with memory augmentation
	response, err := a.generateMemoryAugmentedResponse(ctx, userMessage)
	if err != nil {
		a.Logger().Error("Failed to generate memory-augmented response",
			Field{Key: "error", Value: err},
			Field{Key: "message_id", Value: msg.ID()},
		)
		return err
	}

	// Store the assistant response in memory
	if a.config.AutoSaveConversations {
		responseKey := fmt.Sprintf("response_%d", time.Now().UnixNano())
		responseData := memory.Message{
			ID:        fmt.Sprintf("resp_%s", msg.ID()),
			Role:      "assistant",
			Content:   response.Content,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"model":       response.Model,
				"tokens_used": response.Usage.TotalTokens,
			},
		}

		if err := a.memory.StoreWithEmbedding(ctx, responseKey, responseData); err != nil {
			a.Logger().Warn("Failed to store response in memory", Field{Key: "error", Value: err})
		}
	}

	a.Logger().Info("Memory-augmented response generated",
		Field{Key: "agent_id", Value: a.ID()},
		Field{Key: "message_type", Value: msg.Type()},
		Field{Key: "response_length", Value: len(response.Content)},
		Field{Key: "model", Value: response.Model},
		Field{Key: "tokens_used", Value: response.Usage.TotalTokens},
	)

	return nil
}

// generateMemoryAugmentedResponse generates a response using LLM with memory context.
func (a *MemoryLLMAgent) generateMemoryAugmentedResponse(ctx context.Context, userMessage string) (*llm.Response, error) {
	var contextMessages []llm.Message

	// Add system prompt
	if a.GetSystemPrompt() != "" {
		contextMessages = append(contextMessages, llm.NewSystemMessage(a.GetSystemPrompt()))
	}

	// Add relevant memories if RAG is enabled
	if a.config.EnableRAG && a.config.MemorySearchEnabled {
		relevantMemories, err := a.searchRelevantMemories(ctx, userMessage)
		if err != nil {
			a.Logger().Warn("Failed to search relevant memories", Field{Key: "error", Value: err})
		} else if len(relevantMemories) > 0 {
			memoryContext := a.buildMemoryContext(relevantMemories)
			if memoryContext != "" {
				contextMessages = append(contextMessages, llm.NewSystemMessage(
					fmt.Sprintf("Relevant context from memory:\n%s", memoryContext),
				))
			}
		}
	}

	// Add conversation history
	history := a.GetHistory()
	contextMessages = append(contextMessages, history...)

	// Add current user message
	contextMessages = append(contextMessages, llm.NewUserMessage(userMessage))

	// Build LLM request
	request := llm.NewRequest().
		WithMessages(contextMessages...).
		WithTemperature(0.7).
		WithMaxTokens(1000).
		Build()

	// Generate response using the base LLM agent's provider
	if a.GetLLMProvider() != nil {
		return a.GetLLMProvider().GenerateResponse(ctx, request)
	}

	if a.GetLLMManager() != nil {
		return a.GetLLMManager().GenerateResponse(ctx, "", request)
	}

	return nil, fmt.Errorf("no LLM provider available")
}

// searchRelevantMemories searches for memories relevant to the current query.
func (a *MemoryLLMAgent) searchRelevantMemories(ctx context.Context, query string) ([]memory.SimilarityResult, error) {
	if !a.config.MemorySearchEnabled {
		return nil, nil
	}

	results, err := a.memory.SimilaritySearch(ctx, query, a.config.RAGContextSize)
	if err != nil {
		return nil, err
	}

	// Filter by similarity threshold
	var filtered []memory.SimilarityResult
	for _, result := range results {
		if result.Score >= a.config.RAGSimilarityThreshold {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

// buildMemoryContext builds context string from relevant memories.
func (a *MemoryLLMAgent) buildMemoryContext(memories []memory.SimilarityResult) string {
	var contextParts []string
	totalLength := 0

	for _, mem := range memories {
		var text string

		// Extract text based on memory type
		switch data := mem.Data.(type) {
		case memory.Message:
			text = fmt.Sprintf("[%s]: %s", data.Role, data.Content)
		case memory.KnowledgeEntry:
			text = fmt.Sprintf("[Knowledge - %s]: %s", data.Category, data.Content)
		case string:
			text = data
		default:
			text = fmt.Sprintf("%v", data)
		}

		// Check length limits
		if a.config.MaxMemoryContextLength > 0 {
			if totalLength+len(text) > a.config.MaxMemoryContextLength {
				break
			}
		}

		contextParts = append(contextParts, text)
		totalLength += len(text)
	}

	return strings.Join(contextParts, "\n\n")
}

// StoreKnowledge stores structured knowledge in the agent's memory.
func (a *MemoryLLMAgent) StoreKnowledge(ctx context.Context, knowledge memory.KnowledgeEntry) error {
	key := fmt.Sprintf("knowledge_%s", knowledge.ID)
	return a.memory.StoreWithEmbedding(ctx, key, knowledge)
}

// SearchKnowledge searches for knowledge entries by content similarity.
func (a *MemoryLLMAgent) SearchKnowledge(ctx context.Context, query string, limit int) ([]memory.KnowledgeEntry, error) {
	results, err := a.memory.SimilaritySearch(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	var knowledge []memory.KnowledgeEntry
	for _, result := range results {
		if entry, ok := result.Data.(memory.KnowledgeEntry); ok {
			knowledge = append(knowledge, entry)
		}
	}

	return knowledge, nil
}

// StoreConversation stores a complete conversation in memory.
func (a *MemoryLLMAgent) StoreConversation(ctx context.Context, conversation memory.ConversationMemory) error {
	key := fmt.Sprintf("conversation_%s_%s", conversation.AgentID, conversation.SessionID)
	return a.memory.StoreWithEmbedding(ctx, key, conversation)
}

// GetConversationHistory retrieves conversation history from memory.
func (a *MemoryLLMAgent) GetConversationHistory(ctx context.Context, sessionID string) (*memory.ConversationMemory, error) {
	key := fmt.Sprintf("conversation_%s_%s", a.ID(), sessionID)
	data, err := a.memory.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}

	if conv, ok := data.(memory.ConversationMemory); ok {
		return &conv, nil
	}

	return nil, fmt.Errorf("invalid conversation data")
}

// SaveMemoryToFile exports agent memory to a file.
func (a *MemoryLLMAgent) SaveMemoryToFile(ctx context.Context, filePath string) error {
	return a.memory.Backup(ctx, filePath)
}

// LoadMemoryFromFile imports agent memory from a file.
func (a *MemoryLLMAgent) LoadMemoryFromFile(ctx context.Context, filePath string) error {
	return a.memory.Restore(ctx, filePath)
}

// GetMemoryStats returns statistics about the agent's memory usage.
func (a *MemoryLLMAgent) GetMemoryStats() memory.CacheStats {
	return a.memory.Stats()
}

// ClearMemory clears all memory (with confirmation).
func (a *MemoryLLMAgent) ClearMemory(ctx context.Context, confirm bool) error {
	if !confirm {
		return fmt.Errorf("memory clear not confirmed")
	}

	return a.memory.Clear(ctx)
}

// GetMemory returns the underlying memory store.
func (a *MemoryLLMAgent) GetMemory() memory.CompositeMemory {
	return a.memory
}

// MemoryCapableAgent interface for agents with memory capabilities.
type MemoryCapableAgent interface {
	LLMCapableAgent

	// StoreKnowledge stores structured knowledge
	StoreKnowledge(ctx context.Context, knowledge memory.KnowledgeEntry) error

	// SearchKnowledge searches for knowledge by similarity
	SearchKnowledge(ctx context.Context, query string, limit int) ([]memory.KnowledgeEntry, error)

	// StoreConversation stores conversation history
	StoreConversation(ctx context.Context, conversation memory.ConversationMemory) error

	// GetConversationHistory retrieves conversation history
	GetConversationHistory(ctx context.Context, sessionID string) (*memory.ConversationMemory, error)

	// SaveMemoryToFile exports memory to file
	SaveMemoryToFile(ctx context.Context, filePath string) error

	// LoadMemoryFromFile imports memory from file
	LoadMemoryFromFile(ctx context.Context, filePath string) error

	// GetMemoryStats returns memory statistics
	GetMemoryStats() memory.CacheStats

	// ClearMemory clears all memory
	ClearMemory(ctx context.Context, confirm bool) error

	// GetMemory returns the memory store
	GetMemory() memory.CompositeMemory
}

// MemoryLLMAgentFactory creates memory-enhanced LLM agents.
type MemoryLLMAgentFactory struct {
	llmManager    llm.Manager
	memoryManager memory.MemoryManager
	defaultConfig MemoryAgentConfig
}

// NewMemoryLLMAgentFactory creates a new memory-enhanced LLM agent factory.
func NewMemoryLLMAgentFactory(llmManager llm.Manager, memoryManager memory.MemoryManager, defaultConfig MemoryAgentConfig) AgentFactory {
	defaultConfig.LLMAgentConfig.LLMManager = llmManager
	return &MemoryLLMAgentFactory{
		llmManager:    llmManager,
		memoryManager: memoryManager,
		defaultConfig: defaultConfig,
	}
}

// Create creates a new memory-enhanced LLM agent.
func (f *MemoryLLMAgentFactory) Create(config Config) (Agent, error) {
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

	// Memory-specific configuration
	if enableRAG := config.GetBool("enable_rag"); enableRAG {
		agentConfig.EnableRAG = enableRAG
	}

	if ragContextSize := config.GetInt("rag_context_size"); ragContextSize > 0 {
		agentConfig.RAGContextSize = ragContextSize
	}

	if autoSave := config.GetBool("auto_save_conversations"); autoSave {
		agentConfig.AutoSaveConversations = autoSave
	}

	// Configure memory storage path
	if memoryPath := config.GetString("memory_path"); memoryPath != "" {
		agentConfig.MemoryConfig.PersistentPath = memoryPath
	}

	// Create the agent
	agent, err := NewMemoryLLMAgent(agentConfig)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

// Type returns the type of agent this factory creates.
func (f *MemoryLLMAgentFactory) Type() string {
	return "memory_llm_agent"
}

// Helper function to create a simple memory-enhanced LLM agent
func NewSimpleMemoryLLMAgent(name string, systemPrompt string, llmManager llm.Manager, memoryPath string) (*MemoryLLMAgent, error) {
	// Configure memory
	memConfig := memory.NewMemoryConfig()
	memConfig.PersistentPath = memoryPath
	memConfig.EmbeddingProvider = memory.NewLocalEmbeddingProvider(384)

	config := MemoryAgentConfig{
		LLMAgentConfig: LLMAgentConfig{
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
		},
		MemoryConfig:                *memConfig,
		EnableRAG:                   true,
		RAGContextSize:              3,
		RAGSimilarityThreshold:      0.5,
		AutoSaveConversations:       true,
		ConversationSummaryInterval: 10,
		MaxMemoryContextLength:      2000,
		MemorySearchEnabled:         true,
	}

	return NewMemoryLLMAgent(config)
}

// DefaultMemoryAgentConfig returns a default configuration for memory agents.
func DefaultMemoryAgentConfig() MemoryAgentConfig {
	memConfig := memory.NewMemoryConfig()
	memConfig.EmbeddingProvider = memory.NewLocalEmbeddingProvider(384)

	return MemoryAgentConfig{
		MemoryConfig:                *memConfig,
		EnableRAG:                   true,
		RAGContextSize:              5,
		RAGSimilarityThreshold:      0.3,
		AutoSaveConversations:       true,
		ConversationSummaryInterval: 10,
		KnowledgeCategories:         []string{"general", "technical", "personal"},
		MaxMemoryContextLength:      2000,
		MemorySearchEnabled:         true,
	}
}
