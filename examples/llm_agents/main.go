package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/agentarium/core/agent"
	"github.com/agentarium/core/llm"
	"github.com/agentarium/core/llm/providers"
)

func main() {
	fmt.Println("Starting LLM-powered agents example...")

	// Create LLM manager
	llmManager := llm.NewManager()

	// Setup LLM providers
	setupLLMProviders(llmManager)

	// Create supervisor with LLM support
	supervisorConfig := agent.SupervisorConfig{
		Logger:     agent.NewDefaultLogger(),
		LLMManager: llmManager,
	}
	supervisor := agent.NewSupervisor(supervisorConfig)

	if err := supervisor.Start(); err != nil {
		log.Fatalf("Failed to start supervisor: %v", err)
	}

	// Create different types of LLM agents
	createLLMAgents(supervisor, llmManager)

	// Demonstrate agent interactions
	demonstrateAgentInteractions(supervisor)

	// Clean shutdown
	fmt.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := supervisor.Shutdown(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	fmt.Println("LLM agents example completed!")
}

func setupLLMProviders(manager llm.Manager) {
	fmt.Println("Setting up LLM providers...")

	// Setup OpenAI provider (default)
	openAIConfig := llm.NewProviderConfig()
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		if providerConfig, ok := openAIConfig.(*llm.ProviderConfig); ok {
			providerConfig.SetAPIKey(apiKey)
			providerConfig.SetModel("gpt-3.5-turbo")
		}

		openAIProvider := providers.NewOpenAIProvider(openAIConfig)
		if err := manager.RegisterProvider(openAIProvider); err != nil {
			log.Printf("Failed to register OpenAI provider: %v", err)
		} else {
			fmt.Println("✓ OpenAI provider registered")
		}
	} else {
		fmt.Println("⚠ OPENAI_API_KEY not set, OpenAI provider not available")
	}

	// Setup Claude provider
	claudeConfig := llm.NewProviderConfig()
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		if providerConfig, ok := claudeConfig.(*llm.ProviderConfig); ok {
			providerConfig.SetAPIKey(apiKey)
			providerConfig.SetModel("claude-3-haiku-20240307")
		}

		claudeProvider := providers.NewClaudeProvider(claudeConfig)
		if err := manager.RegisterProvider(claudeProvider); err != nil {
			log.Printf("Failed to register Claude provider: %v", err)
		} else {
			fmt.Println("✓ Claude provider registered")
		}
	} else {
		fmt.Println("⚠ ANTHROPIC_API_KEY not set, Claude provider not available")
	}

	// Setup Ollama provider (local)
	ollamaConfig := llm.NewProviderConfig()
	if providerConfig, ok := ollamaConfig.(*llm.ProviderConfig); ok {
		providerConfig.SetBaseURL("http://localhost:11434")
		providerConfig.SetModel("llama3.2")
	}

	ollamaProvider := providers.NewOllamaProvider(ollamaConfig)
	if err := manager.RegisterProvider(ollamaProvider); err != nil {
		log.Printf("Failed to register Ollama provider: %v", err)
	} else {
		fmt.Println("✓ Ollama provider registered (requires local Ollama server)")
	}

	// Check what providers are available
	providers := manager.ListProviders()
	fmt.Printf("Available LLM providers: %v\n", providers)

	if defaultProvider := manager.GetDefaultProvider(); defaultProvider != nil {
		fmt.Printf("Default provider: %s\n", defaultProvider.Name())
	}
}

func createLLMAgents(supervisor agent.Supervisor, llmManager llm.Manager) {
	fmt.Println("\nCreating LLM-powered agents...")

	// Create LLM agent factory
	llmFactory := agent.NewLLMAgentFactory(llmManager, agent.LLMAgentConfig{
		BaseAgentConfig: agent.BaseAgentConfig{
			MailboxSize: 50,
			Logger:      agent.NewDefaultLogger(),
		},
		MaxHistory:  10,
		Temperature: 0.7,
	})

	supervisor.RegisterFactory(llmFactory)

	// Create a helpful assistant agent
	assistantConfig := agent.NewMapConfig()
	assistantConfig.Set("name", "assistant-agent")
	assistantConfig.Set("system_prompt", "You are a helpful AI assistant. Provide concise and accurate responses to user questions.")
	assistantConfig.Set("max_history", 15)

	assistant, err := supervisor.Spawn(llmFactory, assistantConfig)
	if err != nil {
		log.Printf("Failed to create assistant agent: %v", err)
		return
	}
	fmt.Printf("✓ Created assistant agent: %s\n", assistant.Name())

	// Create a creative writer agent with Claude (if available)
	writerConfig := agent.NewMapConfig()
	writerConfig.Set("name", "writer-agent")
	writerConfig.Set("system_prompt", "You are a creative writer. Help users with creative writing tasks, stories, and poetry.")
	writerConfig.Set("llm_provider", "claude") // Try to use Claude
	writerConfig.Set("temperature", 0.9)       // More creative

	writer, err := supervisor.Spawn(llmFactory, writerConfig)
	if err != nil {
		log.Printf("Failed to create writer agent (will fallback to default): %v", err)
		// Try with default provider
		writerConfig.Set("llm_provider", "")
		writer, err = supervisor.Spawn(llmFactory, writerConfig)
		if err != nil {
			log.Printf("Failed to create writer agent: %v", err)
			return
		}
	}
	fmt.Printf("✓ Created writer agent: %s\n", writer.Name())

	// Create a coding assistant agent
	coderConfig := agent.NewMapConfig()
	coderConfig.Set("name", "coder-agent")
	coderConfig.Set("system_prompt", "You are a programming assistant. Help users with coding questions, debugging, and code explanations. Always provide clear, commented code examples.")
	coderConfig.Set("temperature", 0.3) // More focused for coding

	coder, err := supervisor.Spawn(llmFactory, coderConfig)
	if err != nil {
		log.Printf("Failed to create coder agent: %v", err)
		return
	}
	fmt.Printf("✓ Created coder agent: %s\n", coder.Name())
}

func demonstrateAgentInteractions(supervisor agent.Supervisor) {
	fmt.Println("\nDemonstrating LLM agent interactions...")

	registry := supervisor.GetRegistry()
	agents := registry.List()

	for _, agentInstance := range agents {
		if llmAgent, ok := agentInstance.(agent.LLMCapableAgent); ok {
			demonstrateAgentChat(llmAgent)
		}
	}
}

func demonstrateAgentChat(llmAgent agent.LLMCapableAgent) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("\n--- Chatting with %s ---\n", llmAgent.Name())

	// Determine what to ask based on the agent type
	var question string
	switch {
	case contains(llmAgent.Name(), "assistant"):
		question = "What are the benefits of using Go for building concurrent systems?"
	case contains(llmAgent.Name(), "writer"):
		question = "Write a short poem about artificial intelligence and creativity."
	case contains(llmAgent.Name(), "coder"):
		question = "Show me a simple Go function that calculates the factorial of a number."
	default:
		question = "Hello! What can you help me with?"
	}

	fmt.Printf("Question: %s\n", question)

	// Check if provider is configured before making the request
	if provider := llmAgent.GetLLMProvider(); provider != nil {
		if !provider.IsConfigured() {
			fmt.Printf("Response: [LLM provider %s not configured - skipping interaction]\n", provider.Name())
			return
		}
	} else if manager := llmAgent.GetLLMManager(); manager != nil {
		if defaultProvider := manager.GetDefaultProvider(); defaultProvider != nil {
			if !defaultProvider.IsConfigured() {
				fmt.Printf("Response: [Default LLM provider %s not configured - skipping interaction]\n", defaultProvider.Name())
				return
			}
		} else {
			fmt.Printf("Response: [No LLM providers available - skipping interaction]\n")
			return
		}
	}

	// Try to get a response
	response, err := llmAgent.Ask(ctx, question)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response)

	// Follow up question
	followUp := "Thank you! Can you elaborate on that?"
	fmt.Printf("\nFollow-up: %s\n", followUp)

	response2, err := llmAgent.Ask(ctx, followUp)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response2)

	// Show conversation history
	history := llmAgent.GetHistory()
	fmt.Printf("Conversation history length: %d messages\n", len(history))
}

func contains(str, substr string) bool {
	return strings.Contains(str, substr)
}
