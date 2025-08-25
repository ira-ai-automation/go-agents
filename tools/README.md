# Go Agents Tools

The tools package provides powerful external resource access capabilities for Go Agents, enabling agents to interact with file systems, web APIs, and external services through a standardized interface.

## Features

- 🔧 **Extensible Tool Interface**: Clean, standardized interface for creating custom tools
- 🗃️ **Built-in Tools**: File system operations, HTTP requests, and more
- 🌐 **MCP Integration**: Model Context Protocol client for external tool servers
- 🔒 **Security**: Configurable tool permissions, sandboxing, and validation
- ⚡ **Performance**: Concurrent execution with limits and timeouts
- 🔄 **Retry Logic**: Automatic retry with exponential backoff
- 📊 **Monitoring**: Comprehensive logging and execution statistics

## Quick Start

### Basic Tool Usage

```go
package main

import (
    "context"
    "fmt"

    "github.com/ira-ai-automation/go-agents/tools"
    "github.com/ira-ai-automation/go-agents/tools/builtin"
)

func main() {
    // Create tool registry and executor
    registry := tools.NewBaseToolRegistry()
    config := tools.DefaultToolConfig()
    executor := tools.NewBaseToolExecutor(registry, config)

    // Register built-in tools
    fsTools := builtin.NewFileSystemTool("./sandbox")
    registry.Register(fsTools)

    httpTool := builtin.NewHTTPTool(30*time.Second, 1024*1024)
    registry.Register(httpTool)

    // Execute a tool
    ctx := context.Background()
    result, err := executor.Execute(ctx, "filesystem", map[string]interface{}{
        "operation": "write",
        "path":      "hello.txt",
        "content":   "Hello, World!",
    })

    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }

    fmt.Printf("Success: %v\n", result.Success)
    fmt.Printf("Data: %v\n", result.Data)
}
```

### Tool-Enabled Agents

```go
package main

import (
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

    // Register tools
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

    // Start the agent
    ctx := context.Background()
    toolAgent.Start(ctx)

    // Send a query that will use tools
    queryMsg := agent.NewMessage().
        Type("ask").
        From("user").
        To(toolAgent.ID()).
        WithPayload("Read the README.md file and summarize it").
        Build()

    toolAgent.SendMessage(queryMsg)
}
```

## Built-in Tools

### File System Tool

Provides secure file system operations with sandboxing:

```go
fsTools := builtin.NewFileSystemTool("./sandbox") // Restrict to sandbox directory
registry.Register(fsTools)

// Supported operations:
// - read: Read file contents
// - write: Write content to file
// - list: List directory contents
// - exists: Check if file/directory exists
// - mkdir: Create directories
// - delete: Delete files/directories
// - stat: Get file information
```

**Example Usage:**
```go
// Write a file
result, _ := executor.Execute(ctx, "filesystem", map[string]interface{}{
    "operation": "write",
    "path":      "data.txt",
    "content":   "Hello, World!",
})

// Read a file
result, _ := executor.Execute(ctx, "filesystem", map[string]interface{}{
    "operation": "read",
    "path":      "data.txt",
})

// List directory
result, _ := executor.Execute(ctx, "filesystem", map[string]interface{}{
    "operation": "list",
    "path":      ".",
    "recursive": true,
})
```

### HTTP Tool

Provides HTTP client capabilities for web API integration:

```go
httpTool := builtin.NewHTTPTool(30*time.Second, 1024*1024) // 30s timeout, 1MB max response
registry.Register(httpTool)
```

**Example Usage:**
```go
// GET request
result, _ := executor.Execute(ctx, "http", map[string]interface{}{
    "method": "GET",
    "url":    "https://api.example.com/data",
    "headers": map[string]string{
        "Authorization": "Bearer token",
    },
})

// POST with JSON
result, _ := executor.Execute(ctx, "http", map[string]interface{}{
    "method": "POST",
    "url":    "https://api.example.com/submit",
    "json": map[string]interface{}{
        "name": "John",
        "age":  30,
    },
})
```

## MCP (Model Context Protocol) Integration

Connect to external MCP servers for additional tools:

```go
package main

import (
    "context"
    "github.com/ira-ai-automation/go-agents/tools/mcp"
)

func main() {
    // Create MCP client
    mcpClient := mcp.NewMCPClient(30 * time.Second)
    
    // Connect to MCP server
    ctx := context.Background()
    err := mcpClient.Connect(ctx, "http://localhost:8080")
    if err != nil {
        panic(err)
    }

    // List available tools from MCP server
    mcpTools, err := mcpClient.ListTools(ctx)
    if err != nil {
        panic(err)
    }

    // Register MCP tools in your registry
    for _, tool := range mcpTools {
        registry.Register(tool)
    }
}
```

## Creating Custom Tools

Implement the `Tool` interface to create custom tools:

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/ira-ai-automation/go-agents/tools"
)

type CalculatorTool struct{}

func (c *CalculatorTool) Name() string {
    return "calculator"
}

func (c *CalculatorTool) Description() string {
    return "Performs basic mathematical calculations"
}

func (c *CalculatorTool) Schema() *tools.ToolSchema {
    return &tools.ToolSchema{
        Type: "object",
        Properties: map[string]*tools.PropertySpec{
            "operation": {
                Type: "string",
                Description: "Mathematical operation",
                Enum: []string{"add", "subtract", "multiply", "divide"},
            },
            "a": {
                Type: "number",
                Description: "First number",
            },
            "b": {
                Type: "number",
                Description: "Second number",
            },
        },
        Required: []string{"operation", "a", "b"},
    }
}

func (c *CalculatorTool) Validate(params map[string]interface{}) error {
    // Validate required parameters
    if _, ok := params["operation"]; !ok {
        return fmt.Errorf("operation is required")
    }
    if _, ok := params["a"]; !ok {
        return fmt.Errorf("parameter 'a' is required")
    }
    if _, ok := params["b"]; !ok {
        return fmt.Errorf("parameter 'b' is required")
    }
    return nil
}

func (c *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
    operation := params["operation"].(string)
    a := params["a"].(float64)
    b := params["b"].(float64)

    var result float64
    switch operation {
    case "add":
        result = a + b
    case "subtract":
        result = a - b
    case "multiply":
        result = a * b
    case "divide":
        if b == 0 {
            return &tools.ToolResult{
                Success: false,
                Error:   "division by zero",
            }, nil
        }
        result = a / b
    default:
        return &tools.ToolResult{
            Success: false,
            Error:   "unsupported operation",
        }, nil
    }

    return &tools.ToolResult{
        Success: true,
        Data: map[string]interface{}{
            "result":    result,
            "operation": operation,
            "inputs":    map[string]float64{"a": a, "b": b},
        },
        Duration:  time.Since(time.Now()),
        Timestamp: time.Now(),
    }, nil
}
```

## Configuration

Configure tool execution behavior:

```go
config := &tools.ToolConfig{
    DefaultTimeout:    30 * time.Second,  // Default execution timeout
    MaxConcurrent:     10,                // Max concurrent tool executions
    RetryAttempts:     3,                 // Number of retry attempts
    RetryDelay:        1 * time.Second,   // Delay between retries
    EnableLogging:     true,              // Enable execution logging
    LogLevel:          "info",            // Log level
    AllowedTools:      []string{"filesystem", "http"}, // Whitelist tools
    BlockedTools:      []string{"dangerous_tool"},     // Blacklist tools
    SecurityPolicy:    "strict",          // Security policy level
}

executor := tools.NewBaseToolExecutor(registry, config)
```

## Security Features

### Sandboxing

Restrict file system access to specific directories:

```go
// Only allow access to ./workspace directory
fsTools := builtin.NewFileSystemTool("./workspace")
```

### Tool Permissions

Control which tools are available:

```go
config := tools.DefaultToolConfig()
config.AllowedTools = []string{"filesystem", "http"} // Only allow these tools
config.BlockedTools = []string{"dangerous_tool"}     // Block dangerous tools
```

### Execution Limits

Prevent abuse with timeouts and concurrency limits:

```go
config := tools.DefaultToolConfig()
config.DefaultTimeout = 30 * time.Second  // 30 second timeout
config.MaxConcurrent = 5                  // Max 5 concurrent executions
```

## Agent Tool Calling

Tool-enabled agents automatically parse LLM responses for tool calls:

```
TOOL_CALL: filesystem
PARAMETERS: {"operation": "read", "path": "data.txt"}
```

The agent will:
1. Extract tool calls from LLM responses
2. Execute the tools with the specified parameters
3. Include tool results in the conversation context
4. Allow the LLM to respond based on tool results

## Examples

Check out the examples:
- `examples/tool_agents/`: Complete tool-enabled agent example
- `tools/builtin/`: Built-in tool implementations
- `tools/mcp/`: MCP client integration

## Error Handling

Tools provide comprehensive error information:

```go
result, err := executor.Execute(ctx, "filesystem", params)
if err != nil {
    if toolErr, ok := err.(*tools.ToolError); ok {
        fmt.Printf("Tool error: %s (%s)\n", toolErr.Message, toolErr.Code)
        fmt.Printf("Suggestion: %s\n", toolErr.Suggestion)
    }
}

if !result.Success {
    fmt.Printf("Tool failed: %s\n", result.Error)
}
```

## Performance Monitoring

Track tool execution statistics:

```go
stats := executor.GetExecutionStats()
for toolName, count := range stats {
    fmt.Printf("Tool %s: %d concurrent executions\n", toolName, count)
}
```

## Best Practices

1. **Security First**: Always use sandboxing and permission controls
2. **Timeout Management**: Set appropriate timeouts for external operations
3. **Error Handling**: Provide clear error messages and suggestions
4. **Validation**: Validate all tool parameters before execution
5. **Logging**: Enable logging for debugging and monitoring
6. **Resource Limits**: Set reasonable concurrency and size limits
7. **Testing**: Test tools in isolation before integration

## Contributing

To add new built-in tools:

1. Implement the `Tool` interface
2. Add comprehensive parameter validation
3. Include proper error handling
4. Add tests and documentation
5. Follow security best practices

For MCP integration:
1. Ensure compatibility with MCP protocol
2. Handle connection failures gracefully
3. Validate MCP tool schemas
4. Test with real MCP servers
