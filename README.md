# Go Agents

A powerful and flexible agent-based system library for Go that leverages goroutines for high-performance concurrent agent execution.

## Features

- 🚀 **High Performance**: Built on Go's powerful goroutines for efficient concurrent execution
- 🔧 **Modular Design**: Clean interfaces and pluggable components for maximum flexibility
- 📨 **Message Passing**: Robust message routing and communication between agents
- 🤖 **LLM Integration**: Support for multiple LLM providers (OpenAI, Claude, Ollama, etc.)
- 🧠 **Advanced Memory**: Persistent storage, vector search, and RAG capabilities
- 🔍 **Agent Discovery**: Advanced registry and discovery mechanisms
- 📊 **Supervision**: Built-in agent lifecycle management and health monitoring
- 🛡️ **Error Handling**: Comprehensive error handling with retry mechanisms
- 📝 **Structured Logging**: Flexible logging system with multiple output formats
- ⚙️ **Configuration**: Type-safe configuration management
- 🔄 **Context Management**: Hierarchical context management with cancellation support
- 🔧 **Tool Integration**: Extensible tool system with built-in file system, HTTP, and MCP support

## Quick Start

### Installation

```bash
go mod init your-project
go get github.com/ira-ai-automation/go-agents
```

### Basic Agent Example

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/ira-ai-automation/go-agents/agent"
)

func main() {
    // Create a simple agent
    config := agent.BaseAgentConfig{
        Name:        "hello-agent",
        MailboxSize: 50,
        Logger:      agent.NewDefaultLogger(),
        OnMessage: func(msg agent.Message) error {
            log.Printf("Received: %s", msg.Type())
            return nil
        },
    }
    
    myAgent := agent.NewBaseAgent(config)
    
    // Start the agent
    ctx := context.Background()
    if err := myAgent.Start(ctx); err != nil {
        log.Fatal(err)
    }
    
    // Send a message
    msg := agent.NewMessage().
        Type("hello").
        From("system").
        To(myAgent.ID()).
        WithPayload("Hello, World!").
        Build()
    
    myAgent.SendMessage(msg)
    
    // Clean shutdown
    time.Sleep(1 * time.Second)
    myAgent.Stop()
}
```

### Multi-Agent System with Supervisor

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/ira-ai-automation/go-agents/agent"
)

func main() {
    // Create supervisor
    supervisor := agent.NewSupervisor(agent.SupervisorConfig{
        Logger: agent.NewDefaultLogger(),
    })
    
    supervisor.Start()
    
    // Create agent factory
    factory := agent.NewSimpleAgentFactory("worker", func(config agent.Config) (agent.Agent, error) {
        return agent.NewBaseAgent(agent.BaseAgentConfig{
            Name:   config.GetString("name"),
            Logger: agent.NewDefaultLogger(),
            OnMessage: func(msg agent.Message) error {
                fmt.Printf("Worker %s processed: %s\n", 
                    config.GetString("name"), msg.Type())
                return nil
            },
        }), nil
    })
    
    // Register factory
    supervisor.RegisterFactory(factory)
    
    // Spawn agents
    for i := 1; i <= 3; i++ {
        config := agent.NewMapConfig()
        config.Set("name", fmt.Sprintf("worker-%d", i))
        
        worker, err := supervisor.Spawn(factory, config)
        if err != nil {
            panic(err)
        }
        
        fmt.Printf("Spawned: %s\n", worker.Name())
    }
    
    // Cleanup
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    supervisor.Shutdown(ctx)
}
```

### LLM-Powered Agents

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/ira-ai-automation/go-agents/agent"
    "github.com/ira-ai-automation/go-agents/llm"
    "github.com/ira-ai-automation/go-agents/llm/providers"
)

func main() {
    // Setup LLM manager
    llmManager := llm.NewManager()
    
    // Add OpenAI provider
    openAIConfig := llm.NewProviderConfig()
    if providerConfig, ok := openAIConfig.(*llm.ProviderConfig); ok {
        providerConfig.SetAPIKey(os.Getenv("OPENAI_API_KEY"))
        providerConfig.SetModel("gpt-3.5-turbo")
    }
    
    openAIProvider := providers.NewOpenAIProvider(openAIConfig)
    llmManager.RegisterProvider(openAIProvider)
    
    // Create LLM-powered agent
    assistant := agent.NewSimpleLLMAgent(
        "ai-assistant",
        "You are a helpful AI assistant.",
        llmManager,
    )
    
    ctx := context.Background()
    assistant.Start(ctx)
    
    // Chat with the agent
    response, err := assistant.Ask(ctx, "What is Go good for?")
    if err != nil {
        panic(err)
    }
    
    fmt.Println("AI Response:", response)
    
    assistant.Stop()
}
```

### Memory-Powered Agents

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/ira-ai-automation/go-agents/agent"
    "github.com/ira-ai-automation/go-agents/llm"
    "github.com/ira-ai-automation/go-agents/memory"
)

func main() {
    // Setup LLM manager
    llmManager := llm.NewManager()
    
    // Create memory-enhanced agent with RAG
    memoryAgent, err := agent.NewSimpleMemoryLLMAgent(
        "memory-assistant",
        "You are an AI with perfect memory and access to a knowledge base.",
        llmManager,
        "./data/agent_memory.json",
    )
    
    ctx := context.Background()
    memoryAgent.Start(ctx)
    
    // Store knowledge for future reference
    knowledge := memory.KnowledgeEntry{
        ID:       "go_concurrency",
        Title:    "Go Concurrency",
        Content:  "Go uses goroutines and channels for concurrency...",
        Category: "programming",
        Tags:     []string{"go", "concurrency"},
    }
    
    memoryAgent.StoreKnowledge(ctx, knowledge)
    
    // Agent will use stored knowledge and conversation history
    response, _ := memoryAgent.Ask(ctx, "What do you know about Go concurrency?")
    fmt.Println("Response:", response)
    
    // Search knowledge base
    results, _ := memoryAgent.SearchKnowledge(ctx, "concurrency", 5)
    fmt.Printf("Found %d relevant entries\n", len(results))
    
    memoryAgent.Stop()
}
```

### Tool-Enabled Agents

```go
package main

import (
    "context"
    "fmt"

    "github.com/ira-ai-automation/go-agents/agent"
    "github.com/ira-ai-automation/go-agents/llm"
    "github.com/ira-ai-automation/go-agents/tools"
    "github.com/ira-ai-automation/go-agents/tools/builtin"
)

func main() {
    // Setup LLM manager
    llmManager := llm.NewManager()
    // ... register LLM providers

    // Setup tools
    toolRegistry := tools.NewBaseToolRegistry()
    toolConfig := tools.DefaultToolConfig()
    toolExecutor := tools.NewBaseToolExecutor(toolRegistry, toolConfig)

    // Register built-in tools
    fsTools := builtin.NewFileSystemTool("./workspace")
    httpTool := builtin.NewHTTPTool(30*time.Second, 1024*1024)
    toolRegistry.Register(fsTools)
    toolRegistry.Register(httpTool)

    // Create tool-enabled agent
    systemPrompt := `You are an AI assistant with access to file system and web tools.
    Help users by reading files, making web requests, and processing data.`

    toolAgent := agent.NewSimpleToolLLMAgent(
        "assistant",
        systemPrompt,
        llmManager,
        toolExecutor,
    )

    ctx := context.Background()
    toolAgent.Start(ctx)

    // The agent can now use tools automatically based on LLM responses
    fmt.Printf("Available tools: %v\n", toolAgent.ListAvailableTools())
    
    toolAgent.Stop()
}
```

## Core Components

### Agent Interface

The `Agent` interface is the foundation of the system:

```go
type Agent interface {
    ID() string
    Name() string
    Start(ctx context.Context) error
    Stop() error
    IsRunning() bool
    SendMessage(msg Message) error
    GetMailbox() <-chan Message
}
```

### Message System

Messages are the primary communication mechanism:

```go
// Create a message
msg := agent.NewMessage().
    Type("task").
    From("sender-id").
    To("recipient-id").
    WithPayload(map[string]interface{}{
        "task_id": 123,
        "data":    "process this",
    }).
    WithPriority(5).
    Build()

// Send the message
agent.SendMessage(msg)
```

### Registry and Discovery

Manage and discover agents:

```go
registry := agent.NewAgentRegistry(logger)

// Register an agent
registry.Register(myAgent)

// Find agents by pattern
agents := registry.Find("worker-.*")

// Find agents by tag
registry.SetTags(agentID, []string{"worker", "cpu-intensive"})
workers := registry.FindByTag("worker")
```

### LLM Integration

Support for multiple LLM providers with unified interface:

```go
// Setup LLM manager
llmManager := llm.NewManager()

// Register multiple providers
openAIProvider := providers.NewOpenAIProvider(openAIConfig)
claudeProvider := providers.NewClaudeProvider(claudeConfig)
ollamaProvider := providers.NewOllamaProvider(ollamaConfig)

llmManager.RegisterProvider(openAIProvider)
llmManager.RegisterProvider(claudeProvider)
llmManager.RegisterProvider(ollamaProvider)

// Create LLM-enabled agent
assistant := agent.NewSimpleLLMAgent(
    "ai-assistant",
    "You are a helpful AI assistant specializing in Go programming.",
    llmManager,
)

// Ask questions
response, err := assistant.Ask(ctx, "Explain Go's concurrency model")

// Streaming responses
streamChan, err := assistant.GenerateStreamResponse(ctx, "Tell me about agents")
for chunk := range streamChan {
    fmt.Print(chunk.Content)
}
```

Supported LLM providers:
- **OpenAI**: GPT-3.5, GPT-4, GPT-4o models
- **Anthropic Claude**: Claude-3 Haiku, Sonnet, Opus models  
- **Ollama**: Local LLM serving (Llama, Mistral, CodeLlama, etc.)
- **Extensible**: Easy to add custom providers

### Memory & RAG System

Powerful memory capabilities with vector search and retrieval-augmented generation:

```go
// Create memory configuration
memConfig := memory.NewMemoryConfig().
    WithPersistentStorage("file", "./data/agent_memory.json", nil).
    WithCache("memory", 1000, 24*time.Hour).
    WithVectorDB("memory", 384, nil).
    WithEmbeddingProvider(embeddingProvider)

// Create memory manager
memoryManager := memory.NewMemoryManager()
agentMemory, _ := memoryManager.CreateMemory("agent-1", *memConfig)

// Store knowledge with automatic embedding
knowledge := memory.KnowledgeEntry{
    Title:    "Go Concurrency",
    Content:  "Detailed explanation of Go's concurrency model...",
    Category: "programming",
    Tags:     []string{"go", "concurrency"},
}
agentMemory.StoreWithEmbedding(ctx, "go_concurrency", knowledge)

// Semantic search across memories
results, _ := agentMemory.SimilaritySearch(ctx, "how does concurrency work?", 5)

// RAG system for enhanced responses
ragSystem := memory.NewRAGSystem(vectorMemory, embeddingProvider, ragConfig)
ragResponse, _ := ragSystem.Retrieve(ctx, "explain goroutines")
```

Memory features:
- **Persistent Storage**: File-based, SQLite, PostgreSQL support
- **Vector Search**: Semantic similarity search with embeddings
- **Cache Layer**: LRU cache with TTL for fast access
- **RAG System**: Retrieval-Augmented Generation for context-aware responses
- **Multiple Providers**: OpenAI, local, and custom embedding providers

### Error Handling

Robust error handling with custom error types:

```go
// Create custom errors
err := agent.NewAgentError(agent.ErrAgentNotFound, "agent not found").
    WithContext("agent_id", agentID)

// Handle errors with retry
retry := agent.NewRetry(
    3, // max attempts
    agent.NewExponentialBackoff(100*time.Millisecond, 5*time.Second, 2.0),
    logger,
)

err := retry.Do(func() error {
    return someOperationThatMightFail()
})
```

## Architecture

The library is built around several key interfaces:

- **Agent**: Core agent lifecycle and message handling
- **Message**: Communication between agents
- **Registry**: Agent registration and discovery
- **Supervisor**: Agent lifecycle management
- **Context**: Execution context and shared state
- **Logger**: Structured logging

## Examples

Check out the `/examples` directory for complete working examples:

- `simple_agent/`: Basic agent creation and message handling
- `multi_agent_system/`: Producer-consumer system with multiple agents
- `llm_agents/`: LLM-powered agents with multiple provider support
- `memory_agents/`: Advanced memory, RAG, and knowledge base examples
- `tool_agents/`: Tool-enabled agents with file system and web capabilities

## Best Practices

### Agent Design

1. **Keep agents focused**: Each agent should have a single responsibility
2. **Handle errors gracefully**: Use the error handling utilities
3. **Use structured logging**: Include relevant context in log messages
4. **Implement proper lifecycle**: Handle start/stop events cleanly

### Message Handling

1. **Define clear message types**: Use constants for message types
2. **Validate payloads**: Check message payload structure
3. **Handle unknown messages**: Log warnings for unexpected message types
4. **Use priorities**: Set appropriate message priorities for important messages

### Performance

1. **Configure mailbox sizes**: Set appropriate buffer sizes for your use case
2. **Monitor agent health**: Use the supervisor's monitoring capabilities
3. **Use appropriate logging levels**: Avoid debug logging in production
4. **Handle backpressure**: Implement proper flow control mechanisms

## Configuration

Configure agents using the Config interface:

```go
config := agent.NewMapConfig()
config.Set("worker_count", 10)
config.Set("timeout", "30s")
config.Set("retry_enabled", true)

// Type-safe access
workerCount := config.GetInt("worker_count")
timeout := config.GetDuration("timeout")
retryEnabled := config.GetBool("retry_enabled")
```

## Environment Variables

For LLM providers, set these environment variables:

```bash
# OpenAI
export OPENAI_API_KEY="your-openai-api-key"

# Anthropic Claude
export ANTHROPIC_API_KEY="your-anthropic-api-key"

# For Ollama (local)
# No API key needed, just ensure Ollama is running on localhost:11434
```

## Logging

Multiple logging implementations available:

```go
// Default text logger
logger := agent.NewDefaultLogger()

// Structured logger with custom encoder
logger := agent.NewStructuredLogger(
    agent.LogLevelInfo,
    jsonEncoder,
    fileOutput,
)

// Multi-logger (log to multiple destinations)
logger := agent.NewMultiLogger(
    consoleLogger,
    fileLogger,
    remoteLogger,
)
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Roadmap

- [x] **LLM Integration**: Multi-provider LLM support (OpenAI, Claude, Ollama)
- [x] **Advanced Memory**: Persistent storage, cache, and vector search
- [x] **RAG System**: Retrieval-Augmented Generation with semantic search
- [x] **Vector Embeddings**: Local and OpenAI embedding providers
- [x] **Tool Integration**: Built-in tools (filesystem, HTTP) and MCP client support
- [ ] **Additional LLM Providers**: Google Gemini, Groq, local Hugging Face models
- [ ] **Network-based Communication**: Agent communication across networks
- [ ] **Distributed Systems**: Multi-node agent deployments
- [ ] **Performance Monitoring**: Real-time metrics and dashboards
- [ ] **Agent Persistence**: State recovery and checkpointing
- [ ] **Plugin System**: Extensible architecture for custom components
- [ ] **Web Interface**: GUI for monitoring and managing agents

## Support

- 📖 **Documentation**: [docs.go-agents.dev](https://docs.go-agents.dev)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/ira-ai-automation/go-agents/discussions)
- 🐛 **Issues**: [GitHub Issues](https://github.com/ira-ai-automation/go-agents/issues)
- 📧 **Email**: support@ira-ai-automation.dev