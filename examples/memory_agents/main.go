package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ira-ai-automation/go-agents/agent"
	"github.com/ira-ai-automation/go-agents/llm"
	"github.com/ira-ai-automation/go-agents/llm/providers"
	"github.com/ira-ai-automation/go-agents/memory"
)

func main() {
	fmt.Println("Go Agents - Memory-Powered Agents Example")
	fmt.Println("===========================================")
	fmt.Println()

	// Check for real LLM providers or use mock
	useRealLLM := os.Getenv("OPENAI_API_KEY") != ""
	if !useRealLLM {
		fmt.Println("🔧 Using mock LLM provider (set OPENAI_API_KEY for real LLM)")
		fmt.Println()
	}

	// Run different examples
	fmt.Println("1. Basic Memory Agent Example")
	runBasicMemoryExample(useRealLLM)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("2. Knowledge Base Agent Example")
	runKnowledgeBaseExample(useRealLLM)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("3. RAG-Enhanced Agent Example")
	runRAGExample(useRealLLM)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("4. Conversation Memory Example")
	runConversationMemoryExample(useRealLLM)

	fmt.Println("\nAll memory agent examples completed!")
}

func runBasicMemoryExample(useRealLLM bool) {
	fmt.Println("Creating agent with persistent memory...")

	// Setup LLM manager
	llmManager := setupLLMManager(useRealLLM)

	// Create memory-enhanced agent
	memoryPath := "./data/basic_agent_memory.json"
	agent, err := agent.NewSimpleMemoryLLMAgent(
		"memory-assistant",
		"You are a helpful assistant with perfect memory. You remember all our previous conversations.",
		llmManager,
		memoryPath,
	)
	if err != nil {
		log.Fatalf("Failed to create memory agent: %v", err)
	}

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}
	defer agent.Stop()

	// Store some knowledge
	knowledge := memory.KnowledgeEntry{
		ID:       "go_concurrency",
		Title:    "Go Concurrency",
		Content:  "Go's concurrency model is based on goroutines and channels. Goroutines are lightweight threads managed by the Go runtime.",
		Category: "programming",
		Tags:     []string{"go", "concurrency", "goroutines"},
		Source:   "documentation",
	}

	if err := agent.StoreKnowledge(ctx, knowledge); err != nil {
		log.Printf("Failed to store knowledge: %v", err)
	} else {
		fmt.Println("✓ Stored knowledge about Go concurrency")
	}

	// Ask questions that should use memory
	questions := []string{
		"What do you know about Go's concurrency model?",
		"Tell me more about goroutines",
		"What did we just discuss about Go?",
	}

	for i, question := range questions {
		fmt.Printf("\n--- Question %d ---\n", i+1)
		fmt.Printf("Q: %s\n", question)

		response, err := agent.Ask(ctx, question)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("A: %s\n", response)
	}

	// Show memory stats
	stats := agent.GetMemoryStats()
	fmt.Printf("\nMemory Stats: %d entries, Hit Rate: %.2f%%\n", stats.Size, stats.HitRate*100)
}

func runKnowledgeBaseExample(useRealLLM bool) {
	fmt.Println("Creating agent with specialized knowledge base...")

	llmManager := setupLLMManager(useRealLLM)

	// Create agent specialized in programming help
	agent, err := agent.NewSimpleMemoryLLMAgent(
		"programming-expert",
		"You are a programming expert with access to a comprehensive knowledge base. Use your knowledge to provide detailed, accurate answers.",
		llmManager,
		"./data/programming_expert_memory.json",
	)
	if err != nil {
		log.Fatalf("Failed to create programming expert: %v", err)
	}

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}
	defer agent.Stop()

	// Add programming knowledge
	knowledgeEntries := []memory.KnowledgeEntry{
		{
			ID:       "golang_basics",
			Title:    "Go Programming Basics",
			Content:  "Go is a statically typed, compiled programming language designed at Google. It features garbage collection, type safety, and excellent concurrency support through goroutines.",
			Category: "programming",
			Tags:     []string{"go", "basics", "google"},
			Source:   "golang.org",
		},
		{
			ID:       "golang_channels",
			Title:    "Go Channels",
			Content:  "Channels in Go are a typed conduit through which you can send and receive values with the channel operator <-. They're used for communication between goroutines and can be buffered or unbuffered.",
			Category: "programming",
			Tags:     []string{"go", "channels", "concurrency"},
			Source:   "go documentation",
		},
		{
			ID:       "golang_interfaces",
			Title:    "Go Interfaces",
			Content:  "Interfaces in Go are implemented implicitly. A type implements an interface by implementing its methods. This enables powerful composition patterns and makes Go code more flexible.",
			Category: "programming",
			Tags:     []string{"go", "interfaces", "composition"},
			Source:   "effective go",
		},
	}

	// Store knowledge
	for _, knowledge := range knowledgeEntries {
		if err := agent.StoreKnowledge(ctx, knowledge); err != nil {
			log.Printf("Failed to store knowledge %s: %v", knowledge.ID, err)
		} else {
			fmt.Printf("✓ Stored knowledge: %s\n", knowledge.Title)
		}
	}

	// Ask questions that should retrieve relevant knowledge
	questions := []string{
		"How do channels work in Go?",
		"What makes Go interfaces special?",
		"Explain Go's approach to concurrency",
	}

	for i, question := range questions {
		fmt.Printf("\n--- Programming Question %d ---\n", i+1)
		fmt.Printf("Q: %s\n", question)

		// Search relevant knowledge first
		relevantKnowledge, err := agent.SearchKnowledge(ctx, question, 3)
		if err == nil && len(relevantKnowledge) > 0 {
			fmt.Printf("📚 Found %d relevant knowledge entries\n", len(relevantKnowledge))
		}

		response, err := agent.Ask(ctx, question)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("A: %s\n", response)
	}
}

func runRAGExample(useRealLLM bool) {
	fmt.Println("Creating RAG-enhanced agent with document processing...")

	llmManager := setupLLMManager(useRealLLM)

	// Create agent with RAG capabilities
	memConfig := agent.DefaultMemoryAgentConfig()
	memConfig.LLMAgentConfig = agent.LLMAgentConfig{
		BaseAgentConfig: agent.BaseAgentConfig{
			Name:        "rag-assistant",
			MailboxSize: 50,
			Logger:      agent.NewDefaultLogger(),
			Config:      agent.NewMapConfig(),
		},
		LLMManager:   llmManager,
		SystemPrompt: "You are an AI assistant with access to a knowledge base. Use the provided context to answer questions accurately.",
		MaxHistory:   20,
		Temperature:  0.7,
	}
	memConfig.MemoryConfig.PersistentPath = "./data/rag_agent_memory.json"
	memConfig.EnableRAG = true
	memConfig.RAGContextSize = 5
	memConfig.RAGSimilarityThreshold = 0.3

	ragAgent, err := agent.NewMemoryLLMAgent(memConfig)
	if err != nil {
		log.Fatalf("Failed to create RAG agent: %v", err)
	}

	ctx := context.Background()
	if err := ragAgent.Start(ctx); err != nil {
		log.Fatalf("Failed to start RAG agent: %v", err)
	}
	defer ragAgent.Stop()

	// Setup RAG system
	vectorMemory := ragAgent.GetMemory().GetVectorMemory()
	embeddingProvider := memory.NewLocalEmbeddingProvider(384)
	ragConfig := memory.DefaultRAGConfig()
	ragSystem := memory.NewRAGSystem(vectorMemory, embeddingProvider, ragConfig)

	// Add documents to the RAG system
	documents := []memory.Document{
		{
			ID:       "agent_architecture",
			Title:    "Agent Architecture Patterns",
			Content:  "Modern agent architectures typically follow a layered approach with perception, reasoning, and action components. The perception layer processes inputs, the reasoning layer makes decisions, and the action layer executes responses. Memory systems enable agents to learn from past experiences and maintain context across interactions.",
			Category: "architecture",
			Tags:     []string{"agents", "architecture", "patterns"},
			Source:   "agent design handbook",
		},
		{
			ID:       "memory_systems",
			Title:    "Agent Memory Systems",
			Content:  "Agent memory systems can be categorized into short-term memory (immediate context), working memory (active processing), and long-term memory (persistent storage). Vector databases enable semantic search across stored memories, while traditional databases provide structured storage for specific data types.",
			Category: "memory",
			Tags:     []string{"memory", "vectors", "databases"},
			Source:   "memory systems guide",
		},
	}

	for _, doc := range documents {
		if err := ragSystem.AddDocument(ctx, doc); err != nil {
			log.Printf("Failed to add document %s: %v", doc.ID, err)
		} else {
			fmt.Printf("✓ Added document: %s\n", doc.Title)
		}
	}

	// Ask questions that should trigger RAG
	questions := []string{
		"How should I design an agent architecture?",
		"What types of memory do agents need?",
		"How do vector databases help with agent memory?",
	}

	for i, question := range questions {
		fmt.Printf("\n--- RAG Question %d ---\n", i+1)
		fmt.Printf("Q: %s\n", question)

		// Show RAG retrieval
		ragResponse, err := ragSystem.Retrieve(ctx, question)
		if err == nil && len(ragResponse.Results) > 0 {
			fmt.Printf("🔍 RAG retrieved %d relevant chunks (processing time: %v)\n",
				ragResponse.TotalChunks, ragResponse.ProcessingTime)
		}

		response, err := ragAgent.Ask(ctx, question)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("A: %s\n", response)
	}

	// Show RAG stats
	stats, err := ragSystem.GetStats(ctx)
	if err == nil {
		fmt.Printf("\nRAG Stats: %d documents, %d chunks, avg chunk size: %d chars\n",
			stats.TotalDocuments, stats.TotalChunks, stats.AverageChunkSize)
	}
}

func runConversationMemoryExample(useRealLLM bool) {
	fmt.Println("Creating agent with conversation memory and summarization...")

	llmManager := setupLLMManager(useRealLLM)

	agent, err := agent.NewSimpleMemoryLLMAgent(
		"conversation-expert",
		"You are an assistant who remembers all conversations and can reference previous discussions.",
		llmManager,
		"./data/conversation_memory.json",
	)
	if err != nil {
		log.Fatalf("Failed to create conversation agent: %v", err)
	}

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}
	defer agent.Stop()

	// Simulate a conversation session
	sessionID := "demo_session"

	conversationFlow := []struct {
		question string
		context  string
	}{
		{"Hello, I'm working on a Go project", "introduction"},
		{"I'm having trouble with goroutines", "problem statement"},
		{"Can you help me understand channels?", "seeking help"},
		{"What did I say I was working on earlier?", "memory test"},
		{"Summarize our conversation so far", "summary request"},
	}

	var messages []memory.Message

	for i, step := range conversationFlow {
		fmt.Printf("\n--- Conversation Step %d ---\n", i+1)
		fmt.Printf("User: %s\n", step.question)

		response, err := agent.Ask(ctx, step.question)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("Assistant: %s\n", response)

		// Store conversation messages
		messages = append(messages,
			memory.Message{
				ID:        fmt.Sprintf("user_%d", i),
				Role:      "user",
				Content:   step.question,
				Timestamp: time.Now(),
			},
			memory.Message{
				ID:        fmt.Sprintf("assistant_%d", i),
				Role:      "assistant",
				Content:   response,
				Timestamp: time.Now(),
			},
		)
	}

	// Store the complete conversation
	conversation := memory.ConversationMemory{
		AgentID:      agent.ID(),
		SessionID:    sessionID,
		Messages:     messages,
		Summary:      "Discussion about Go programming, specifically goroutines and channels",
		Topics:       []string{"go", "goroutines", "channels", "programming"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MessageCount: len(messages),
	}

	if err := agent.StoreConversation(ctx, conversation); err != nil {
		log.Printf("Failed to store conversation: %v", err)
	} else {
		fmt.Printf("\n✓ Stored conversation with %d messages\n", len(messages))
	}

	// Test conversation retrieval
	retrievedConv, err := agent.GetConversationHistory(ctx, sessionID)
	if err == nil {
		fmt.Printf("✓ Retrieved conversation: %d messages, topics: %v\n",
			retrievedConv.MessageCount, retrievedConv.Topics)
	}
}

func setupLLMManager(useRealLLM bool) llm.Manager {
	manager := llm.NewManager()

	if useRealLLM {
		// Setup OpenAI provider
		openAIConfig := llm.NewProviderConfig()
		if providerConfig, ok := openAIConfig.(*llm.ProviderConfig); ok {
			providerConfig.SetAPIKey(os.Getenv("OPENAI_API_KEY"))
			providerConfig.SetModel("gpt-3.5-turbo")
		}

		openAIProvider := providers.NewOpenAIProvider(openAIConfig)
		manager.RegisterProvider(openAIProvider)
	} else {
		// Use mock provider
		mockProvider := &MockLLMProvider{name: "mock-memory"}
		manager.RegisterProvider(mockProvider)
	}

	return manager
}

// MockLLMProvider for testing without real LLM API
type MockLLMProvider struct {
	name string
}

func (p *MockLLMProvider) Name() string {
	return p.name
}

func (p *MockLLMProvider) IsConfigured() bool {
	return true
}

func (p *MockLLMProvider) GetCapabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:       false,
		FunctionCalling: false,
		Vision:          false,
		MaxTokens:       4096,
		SupportedModels: []string{"mock-model"},
		DefaultModel:    "mock-model",
	}
}

func (p *MockLLMProvider) GenerateResponse(ctx context.Context, request llm.Request) (*llm.Response, error) {
	// Simulate processing time
	time.Sleep(200 * time.Millisecond)

	// Generate context-aware response based on the last user message
	var lastUserMessage string
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == llm.RoleUser {
			lastUserMessage = request.Messages[i].Content
			break
		}
	}

	response := generateContextualResponse(lastUserMessage, request.Messages)

	return &llm.Response{
		Content:      response,
		Role:         llm.RoleAssistant,
		FinishReason: llm.FinishReasonStop,
		Model:        "mock-model",
		Usage: llm.Usage{
			PromptTokens:     len(lastUserMessage) / 4,
			CompletionTokens: len(response) / 4,
			TotalTokens:      (len(lastUserMessage) + len(response)) / 4,
		},
	}, nil
}

func (p *MockLLMProvider) GenerateStreamResponse(ctx context.Context, request llm.Request) (<-chan llm.StreamChunk, error) {
	respChan := make(chan llm.StreamChunk, 1)
	go func() {
		defer close(respChan)
		response, _ := p.GenerateResponse(ctx, request)
		respChan <- llm.StreamChunk{
			Content:      response.Content,
			FinishReason: llm.FinishReasonStop,
		}
	}()
	return respChan, nil
}

func generateContextualResponse(userMessage string, history []llm.Message) string {
	userMessage = strings.ToLower(userMessage)

	// Check for memory-related queries
	if strings.Contains(userMessage, "what did") || strings.Contains(userMessage, "earlier") || strings.Contains(userMessage, "before") {
		return "Based on our conversation history, I can see we discussed Go programming, specifically goroutines and channels. You mentioned working on a Go project and having trouble with goroutines."
	}

	// Check for specific topics
	if strings.Contains(userMessage, "goroutine") {
		return "Goroutines are lightweight threads in Go. They're managed by the Go runtime and enable concurrent programming. You can start a goroutine by using the 'go' keyword before a function call."
	}

	if strings.Contains(userMessage, "channel") {
		return "Channels in Go are typed conduits for communication between goroutines. They can be created with make(chan Type) and used with the <- operator to send and receive values."
	}

	if strings.Contains(userMessage, "concurrency") || strings.Contains(userMessage, "concurrent") {
		return "Go's concurrency model is based on goroutines and channels, following the principle 'Don't communicate by sharing memory; share memory by communicating.'"
	}

	if strings.Contains(userMessage, "summarize") || strings.Contains(userMessage, "summary") {
		return "Our conversation has covered Go programming fundamentals, with focus on concurrency concepts like goroutines and channels. You're working on a Go project and seeking help with concurrent programming patterns."
	}

	if strings.Contains(userMessage, "architecture") {
		return "Agent architectures typically use layered designs with perception, reasoning, and action components. This allows for modular development and clear separation of concerns."
	}

	if strings.Contains(userMessage, "memory") && strings.Contains(userMessage, "agent") {
		return "Agent memory systems include short-term memory for immediate context, working memory for active processing, and long-term memory for persistent storage. Vector databases enable semantic search across memories."
	}

	if strings.Contains(userMessage, "hello") || strings.Contains(userMessage, "working") {
		return "Hello! I'd be happy to help you with your Go project. What specific aspects are you working on or having trouble with?"
	}

	// Default response
	return "I understand you're asking about " + userMessage + ". Based on my knowledge and our conversation, I can help you with Go programming concepts, especially concurrency patterns with goroutines and channels."
}
