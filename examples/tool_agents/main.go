package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ira-ai-automation/go-agents/agent"
	"github.com/ira-ai-automation/go-agents/llm"
	"github.com/ira-ai-automation/go-agents/llm/providers"
	"github.com/ira-ai-automation/go-agents/tools"
	"github.com/ira-ai-automation/go-agents/tools/builtin"
	"github.com/ira-ai-automation/go-agents/tools/mcp"
)

func main() {
	fmt.Println("Go Agents - Tool-Enabled Agents Example")
	fmt.Println("=======================================")
	fmt.Println()

	// Check for real LLM providers or use mock
	useRealLLM := os.Getenv("OPENAI_API_KEY") != ""
	if !useRealLLM {
		fmt.Println("🔧 Using mock LLM provider (set OPENAI_API_KEY for real LLM)")
	} else {
		fmt.Println("🤖 Using OpenAI LLM provider")
	}

	// Create LLM manager
	llmManager := llm.NewManager()

	if useRealLLM {
		// Setup OpenAI provider
		openAIConfig := llm.NewProviderConfig()
		openAIConfig.(*llm.ProviderConfig).SetAPIKey(os.Getenv("OPENAI_API_KEY"))
		openAIConfig.(*llm.ProviderConfig).SetModel("gpt-3.5-turbo")

		openAIProvider := providers.NewOpenAIProvider(openAIConfig)
		llmManager.RegisterProvider(openAIProvider)
	} else {
		// Setup mock provider for demonstration
		mockProvider := &MockLLMProvider{}
		llmManager.RegisterProvider(mockProvider)
	}

	// Create tool registry and executor
	toolRegistry := tools.NewBaseToolRegistry()
	toolConfig := tools.DefaultToolConfig()
	toolExecutor := tools.NewBaseToolExecutor(toolRegistry, toolConfig)

	// Register built-in tools
	setupBuiltinTools(toolRegistry)

	// Setup MCP client (optional)
	if mcpServerURL := os.Getenv("MCP_SERVER_URL"); mcpServerURL != "" {
		setupMCPTools(toolRegistry, mcpServerURL)
	}

	// Create tool-enabled agent
	systemPrompt := `You are a helpful AI assistant with access to various tools including file system operations and web requests. 
You can help users with tasks like reading files, making web requests, and processing data.
Always explain what you're doing and show the results of tool calls clearly.`

	toolAgent := agent.NewSimpleToolLLMAgent(
		"tool-assistant",
		systemPrompt,
		llmManager,
		toolExecutor,
	)

	// Create supervisor for management
	supervisor := agent.NewSupervisor(agent.SupervisorConfig{
		Logger: agent.NewDefaultLogger(),
	})

	if err := supervisor.Start(); err != nil {
		log.Fatalf("Failed to start supervisor: %v", err)
	}

	// Register and start the tool agent
	registry := supervisor.GetRegistry()
	if err := registry.Register(toolAgent); err != nil {
		log.Fatalf("Failed to register tool agent: %v", err)
	}

	ctx := context.Background()
	if err := toolAgent.Start(ctx); err != nil {
		log.Fatalf("Failed to start tool agent: %v", err)
	}

	fmt.Printf("Tool agent started: %s\n", toolAgent.Name())
	fmt.Printf("Available tools: %v\n", toolAgent.ListAvailableTools())
	fmt.Println()

	// Demonstrate tool usage with various scenarios
	runToolDemonstrations(toolAgent)

	// Clean shutdown
	fmt.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := supervisor.Shutdown(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	fmt.Println("Tool agents example completed!")
}

// setupBuiltinTools registers built-in tools.
func setupBuiltinTools(registry tools.ToolRegistry) {
	// File system tool (with sandbox for security)
	currentDir, _ := os.Getwd()
	sandboxDir := filepath.Join(currentDir, "sandbox")
	os.MkdirAll(sandboxDir, 0755)

	fsTools := builtin.NewFileSystemTool(sandboxDir)
	if err := registry.Register(fsTools); err != nil {
		log.Printf("Failed to register filesystem tool: %v", err)
	}

	// HTTP client tool
	httpTool := builtin.NewHTTPTool(30*time.Second, 1024*1024) // 1MB max response
	if err := registry.Register(httpTool); err != nil {
		log.Printf("Failed to register HTTP tool: %v", err)
	}

	fmt.Printf("Registered built-in tools: filesystem, http\n")
}

// setupMCPTools configures MCP client for external tools.
func setupMCPTools(registry tools.ToolRegistry, serverURL string) {
	mcpClient := mcp.NewMCPClient(30 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mcpClient.Connect(ctx, serverURL); err != nil {
		log.Printf("Failed to connect to MCP server: %v", err)
		return
	}

	mcpTools, err := mcpClient.ListTools(ctx)
	if err != nil {
		log.Printf("Failed to list MCP tools: %v", err)
		return
	}

	for _, tool := range mcpTools {
		if err := registry.Register(tool); err != nil {
			log.Printf("Failed to register MCP tool %s: %v", tool.Name(), err)
		}
	}

	fmt.Printf("Registered %d MCP tools from %s\n", len(mcpTools), serverURL)
}

// runToolDemonstrations shows various tool usage scenarios.
func runToolDemonstrations(toolAgent *agent.ToolLLMAgent) {
	demonstrations := []struct {
		name  string
		query string
	}{
		{
			name:  "File Operations",
			query: "Create a file called 'hello.txt' with the content 'Hello, Tool World!' and then read it back to confirm it was created correctly.",
		},
		{
			name:  "Directory Listing",
			query: "List all files in the current directory and show me their details.",
		},
		{
			name:  "Web Request",
			query: "Make a GET request to https://httpbin.org/json and show me the response.",
		},
		{
			name:  "Data Processing",
			query: "Create a JSON file with some sample data about three people (name, age, city) and then read it back and tell me the average age.",
		},
	}

	for i, demo := range demonstrations {
		fmt.Printf("--- Demonstration %d: %s ---\n", i+1, demo.name)

		queryMsg := agent.NewMessage().
			Type("ask").
			From("user").
			To(toolAgent.ID()).
			WithPayload(demo.query).
			Build()

		if err := toolAgent.SendMessage(queryMsg); err != nil {
			log.Printf("Failed to send query: %v", err)
			continue
		}

		// Wait for response
		time.Sleep(3 * time.Second)

		// Check for response (in a real application, you'd handle this through message channels)
		fmt.Printf("Query: %s\n", demo.query)
		fmt.Println("(Response would be handled through message system)")
		fmt.Printf("Tool calls used: %d/%d\n", toolAgent.GetToolCallCount(), toolAgent.GetMaxToolCalls())
		fmt.Println()
	}

	// Reset tool counter for clean state
	resetMsg := agent.NewMessage().
		Type("reset_tools").
		From("user").
		To(toolAgent.ID()).
		Build()

	toolAgent.SendMessage(resetMsg)
}

// MockLLMProvider provides a mock LLM implementation for demonstration.
type MockLLMProvider struct{}

func (m *MockLLMProvider) Name() string {
	return "mock"
}

func (m *MockLLMProvider) GenerateResponse(ctx context.Context, request llm.Request) (*llm.Response, error) {
	// Simulate tool usage in responses
	messages := request.Messages
	lastMessage := messages[len(messages)-1]
	content := lastMessage.Content

	// Simple mock responses that demonstrate tool calls
	if len(messages) == 2 { // Initial query
		if contains(content, "file") && contains(content, "create") {
			response := `I'll help you create and read a file. Let me start by creating the file.

TOOL_CALL: filesystem
PARAMETERS: {"operation": "write", "path": "hello.txt", "content": "Hello, Tool World!"}`
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}

		if contains(content, "list") && contains(content, "directory") {
			response := `I'll list the files in the current directory for you.

TOOL_CALL: filesystem
PARAMETERS: {"operation": "list", "path": "."}`
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}

		if contains(content, "GET request") || contains(content, "httpbin") {
			response := `I'll make a GET request to httpbin.org/json for you.

TOOL_CALL: http
PARAMETERS: {"method": "GET", "url": "https://httpbin.org/json"}`
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}

		if contains(content, "JSON") && contains(content, "people") {
			response := `I'll create a JSON file with sample data and then analyze it.

TOOL_CALL: filesystem
PARAMETERS: {"operation": "write", "path": "people.json", "content": "[{\"name\":\"Alice\",\"age\":30,\"city\":\"New York\"},{\"name\":\"Bob\",\"age\":25,\"city\":\"Los Angeles\"},{\"name\":\"Charlie\",\"age\":35,\"city\":\"Chicago\"}]"}`
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}
	}

	// Follow-up responses after tool results
	if contains(content, "Tool execution results") {
		if contains(content, "filesystem") && contains(content, "hello.txt") {
			if contains(content, "write") {
				response := `Great! The file was created successfully. Now let me read it back to confirm the content.

TOOL_CALL: filesystem
PARAMETERS: {"operation": "read", "path": "hello.txt"}`
				return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
			} else if contains(content, "read") {
				response := "Perfect! I've successfully created the file 'hello.txt' with the content 'Hello, Tool World!' and confirmed it was written correctly by reading it back."
				return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
			}
		}

		if contains(content, "people.json") {
			response := `Now let me read the JSON file back and calculate the average age.

TOOL_CALL: filesystem
PARAMETERS: {"operation": "read", "path": "people.json"}`
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}

		if contains(content, "Alice") && contains(content, "Bob") {
			response := "I've read the JSON file with the three people. Looking at the ages: Alice (30), Bob (25), and Charlie (35), the average age is (30 + 25 + 35) / 3 = 30 years old."
			return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
		}

		response := "Tool execution completed successfully! The operation has been performed as requested."
		return &llm.Response{Content: response, Role: "assistant", FinishReason: "stop"}, nil
	}

	response := "I'm a mock LLM provider. In a real implementation, I would process your request using the available tools."
	return &llm.Response{
		Content:      response,
		Role:         "assistant",
		FinishReason: "stop",
	}, nil
}

func (m *MockLLMProvider) GenerateStreamResponse(ctx context.Context, request llm.Request) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	go func() {
		defer close(ch)
		response, _ := m.GenerateResponse(ctx, request)
		ch <- llm.StreamChunk{Content: response.Content}
	}()
	return ch, nil
}

func (m *MockLLMProvider) IsConfigured() bool {
	return true
}

func (m *MockLLMProvider) GetCapabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:       true,
		FunctionCalling: true,
		MaxTokens:       4096,
		SupportedModels: []string{"mock-model"},
	}
}

// contains checks if a string contains a substring (case insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
