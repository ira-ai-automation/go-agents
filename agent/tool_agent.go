package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ira-ai-automation/go-agents/llm"
	"github.com/ira-ai-automation/go-agents/tools"
)

// ToolLLMAgent extends LLMAgent with tool execution capabilities.
type ToolLLMAgent struct {
	*LLMAgent
	toolExecutor  tools.ToolExecutor
	toolRegistry  tools.ToolRegistry
	enabledTools  []string // If nil, all tools are enabled
	maxToolCalls  int      // Maximum tool calls per conversation
	toolCallCount int      // Current tool call count
}

// ToolLLMAgentConfig holds configuration for ToolLLMAgent.
type ToolLLMAgentConfig struct {
	LLMAgentConfig
	ToolExecutor tools.ToolExecutor
	ToolRegistry tools.ToolRegistry
	EnabledTools []string // Specific tools to enable (nil = all)
	MaxToolCalls int      // Maximum tool calls per conversation
}

// NewToolLLMAgent creates a new ToolLLMAgent.
func NewToolLLMAgent(config ToolLLMAgentConfig) *ToolLLMAgent {
	if config.MaxToolCalls == 0 {
		config.MaxToolCalls = 50 // Default limit
	}

	llmAgent := NewLLMAgent(config.LLMAgentConfig)

	toolAgent := &ToolLLMAgent{
		LLMAgent:      llmAgent,
		toolExecutor:  config.ToolExecutor,
		toolRegistry:  config.ToolRegistry,
		enabledTools:  config.EnabledTools,
		maxToolCalls:  config.MaxToolCalls,
		toolCallCount: 0,
	}

	// Override the original LLM agent's message handler to include tool processing
	toolAgent.LLMAgent.BaseAgent.onMessage = toolAgent.handleMessage

	return toolAgent
}

// NewSimpleToolLLMAgent creates a ToolLLMAgent with simple configuration.
func NewSimpleToolLLMAgent(name, systemPrompt string, llmManager llm.Manager, toolExecutor tools.ToolExecutor) *ToolLLMAgent {
	config := ToolLLMAgentConfig{
		LLMAgentConfig: LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{
				Name:        name,
				MailboxSize: 100,
				Logger:      NewDefaultLogger(),
				Config:      NewMapConfig(),
			},
			SystemPrompt: systemPrompt,
			LLMManager:   llmManager,
		},
		ToolExecutor: toolExecutor,
		ToolRegistry: toolExecutor.GetRegistry(),
	}

	return NewToolLLMAgent(config)
}

// handleMessage processes messages and handles tool calls.
func (a *ToolLLMAgent) handleMessage(msg Message) error {
	switch msg.Type() {
	case "ask", "query", "question":
		return a.handleAskWithTools(msg)
	case "tool_call":
		return a.handleToolCall(msg)
	case "reset_tools":
		return a.handleResetTools(msg)
	default:
		// Delegate to parent LLM agent for other message types
		return a.LLMAgent.handleLLMMessage(msg)
	}
}

// handleAskWithTools processes queries with tool execution capabilities.
func (a *ToolLLMAgent) handleAskWithTools(msg Message) error {
	query, ok := msg.Payload().(string)
	if !ok {
		if payload, ok := msg.Payload().(map[string]interface{}); ok {
			if q, ok := payload["query"].(string); ok {
				query = q
			} else {
				return NewAgentError(ErrInvalidMessage, "query payload must be a string or contain 'query' field")
			}
		} else {
			return NewAgentError(ErrInvalidMessage, "invalid query payload")
		}
	}

	// Get conversation context with tools
	ctx := context.Background()
	response, err := a.askWithTools(ctx, query)
	if err != nil {
		a.Logger().Error("Failed to process query with tools",
			Field{Key: "error", Value: err},
			Field{Key: "query", Value: query},
		)
		return err
	}

	// Send response back to sender
	responseMsg := NewMessage().
		Type("response").
		From(a.ID()).
		To(msg.Sender()).
		WithPayload(map[string]interface{}{
			"response":       response,
			"tool_calls":     a.toolCallCount,
			"max_tool_calls": a.maxToolCalls,
		}).
		Build()

	return a.SendMessage(responseMsg)
}

// askWithTools processes a query with tool execution support.
func (a *ToolLLMAgent) askWithTools(ctx context.Context, query string) (string, error) {
	// Build system prompt with tool information
	systemPrompt := a.buildSystemPromptWithTools()

	// Create conversation with tools
	conversation := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: query},
	}

	maxIterations := 10 // Prevent infinite loops
	iteration := 0

	for iteration < maxIterations {
		iteration++

		// Check tool call limit
		if a.toolCallCount >= a.maxToolCalls {
			return "I've reached the maximum number of tool calls allowed. Please start a new conversation.", nil
		}

		// Create LLM request
		request := llm.Request{
			Messages:    conversation,
			MaxTokens:   2000,
			Temperature: 0.7,
		}

		// Get LLM response using the provider
		var response *llm.Response
		var err error
		if a.llmProvider != nil {
			response, err = a.llmProvider.GenerateResponse(ctx, request)
		} else if a.llmManager != nil {
			// Use the default provider from manager
			response, err = a.llmManager.GenerateResponse(ctx, "", request)
		} else {
			return "", fmt.Errorf("no LLM provider or manager configured")
		}
		if err != nil {
			return "", fmt.Errorf("failed to generate response: %w", err)
		}

		conversation = append(conversation, llm.Message{
			Role:    "assistant",
			Content: response.Content,
		})

		// Check if the response contains tool calls
		toolCalls := a.extractToolCalls(response.Content)
		if len(toolCalls) == 0 {
			// No tool calls, return the response
			return response.Content, nil
		}

		// Execute tool calls
		toolResults := make([]string, 0, len(toolCalls))
		for _, toolCall := range toolCalls {
			if a.toolCallCount >= a.maxToolCalls {
				break
			}

			result, err := a.executeToolCall(ctx, toolCall)
			if err != nil {
				toolResults = append(toolResults, fmt.Sprintf("Tool call failed: %v", err))
			} else {
				resultStr := a.formatToolResult(toolCall, result)
				toolResults = append(toolResults, resultStr)
			}
			a.toolCallCount++
		}

		// Add tool results to conversation
		if len(toolResults) > 0 {
			toolResultsMsg := fmt.Sprintf("Tool execution results:\n%s", strings.Join(toolResults, "\n\n"))
			conversation = append(conversation, llm.Message{
				Role:    "user",
				Content: toolResultsMsg,
			})
		}
	}

	return "I apologize, but I couldn't complete the request due to too many iterations. Please try a simpler query.", nil
}

// buildSystemPromptWithTools creates a system prompt that includes tool information.
func (a *ToolLLMAgent) buildSystemPromptWithTools() string {
	basePrompt := a.systemPrompt
	if basePrompt == "" {
		basePrompt = "You are a helpful AI assistant with access to tools."
	}

	if a.toolRegistry == nil {
		return basePrompt
	}

	// Get available tools
	availableTools := a.getAvailableTools()
	if len(availableTools) == 0 {
		return basePrompt
	}

	// Build tool descriptions
	var toolDescriptions []string
	for _, tool := range availableTools {
		schema := tool.Schema()
		schemaStr := ""
		if schema != nil {
			if schemaBytes, err := json.MarshalIndent(schema, "", "  "); err == nil {
				schemaStr = string(schemaBytes)
			}
		}

		toolDesc := fmt.Sprintf("- %s: %s\n  Schema: %s", tool.Name(), tool.Description(), schemaStr)
		toolDescriptions = append(toolDescriptions, toolDesc)
	}

	toolsPrompt := fmt.Sprintf(`

You have access to the following tools:
%s

To use a tool, format your response as:
TOOL_CALL: tool_name
PARAMETERS: {"param1": "value1", "param2": "value2"}

You can make multiple tool calls in a single response. Wait for the tool results before providing your final answer.`, strings.Join(toolDescriptions, "\n"))

	return basePrompt + toolsPrompt
}

// getAvailableTools returns the list of tools available to this agent.
func (a *ToolLLMAgent) getAvailableTools() []tools.Tool {
	if a.toolRegistry == nil {
		return nil
	}

	allTools := a.toolRegistry.List()
	if a.enabledTools == nil {
		return allTools // All tools enabled
	}

	// Filter to enabled tools only
	var enabled []tools.Tool
	enabledMap := make(map[string]bool)
	for _, name := range a.enabledTools {
		enabledMap[name] = true
	}

	for _, tool := range allTools {
		if enabledMap[tool.Name()] {
			enabled = append(enabled, tool)
		}
	}

	return enabled
}

// extractToolCalls parses tool calls from LLM response.
func (a *ToolLLMAgent) extractToolCalls(response string) []tools.ToolCall {
	var toolCalls []tools.ToolCall
	lines := strings.Split(response, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "TOOL_CALL:") {
			toolName := strings.TrimSpace(strings.TrimPrefix(line, "TOOL_CALL:"))

			// Look for parameters on the next line
			var params map[string]interface{}
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextLine, "PARAMETERS:") {
					paramStr := strings.TrimSpace(strings.TrimPrefix(nextLine, "PARAMETERS:"))
					json.Unmarshal([]byte(paramStr), &params)
					i++ // Skip the parameters line
				}
			}

			if params == nil {
				params = make(map[string]interface{})
			}

			toolCall := tools.ToolCall{
				ID:         fmt.Sprintf("call_%d", time.Now().UnixNano()),
				Name:       toolName,
				Parameters: params,
				Timestamp:  time.Now(),
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	return toolCalls
}

// executeToolCall executes a single tool call.
func (a *ToolLLMAgent) executeToolCall(ctx context.Context, toolCall tools.ToolCall) (*tools.ToolResult, error) {
	if a.toolExecutor == nil {
		return nil, fmt.Errorf("no tool executor configured")
	}

	a.Logger().Info("Executing tool call",
		Field{Key: "tool_name", Value: toolCall.Name},
		Field{Key: "parameters", Value: toolCall.Parameters},
	)

	result, err := a.toolExecutor.Execute(ctx, toolCall.Name, toolCall.Parameters)
	if err != nil {
		a.Logger().Error("Tool execution failed",
			Field{Key: "tool_name", Value: toolCall.Name},
			Field{Key: "error", Value: err},
		)
		return nil, err
	}

	a.Logger().Info("Tool execution completed",
		Field{Key: "tool_name", Value: toolCall.Name},
		Field{Key: "success", Value: result.Success},
		Field{Key: "duration", Value: result.Duration},
	)

	return result, nil
}

// formatToolResult formats a tool result for inclusion in the conversation.
func (a *ToolLLMAgent) formatToolResult(toolCall tools.ToolCall, result *tools.ToolResult) string {
	if result.Success {
		dataStr := ""
		if result.Data != nil {
			if dataBytes, err := json.MarshalIndent(result.Data, "", "  "); err == nil {
				dataStr = string(dataBytes)
			} else {
				dataStr = fmt.Sprintf("%v", result.Data)
			}
		}
		return fmt.Sprintf("Tool: %s\nStatus: Success\nData: %s", toolCall.Name, dataStr)
	} else {
		return fmt.Sprintf("Tool: %s\nStatus: Failed\nError: %s", toolCall.Name, result.Error)
	}
}

// handleToolCall processes direct tool call messages.
func (a *ToolLLMAgent) handleToolCall(msg Message) error {
	payload, ok := msg.Payload().(map[string]interface{})
	if !ok {
		return NewAgentError(ErrInvalidMessage, "tool_call payload must be a map")
	}

	toolName, ok := payload["tool_name"].(string)
	if !ok {
		return NewAgentError(ErrInvalidMessage, "tool_name is required")
	}

	params, ok := payload["parameters"].(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	ctx := context.Background()
	result, err := a.toolExecutor.Execute(ctx, toolName, params)

	// Send result back to sender
	responseMsg := NewMessage().
		Type("tool_result").
		From(a.ID()).
		To(msg.Sender()).
		WithPayload(map[string]interface{}{
			"tool_name": toolName,
			"result":    result,
			"error":     err,
		}).
		Build()

	return a.SendMessage(responseMsg)
}

// handleResetTools resets the tool call counter.
func (a *ToolLLMAgent) handleResetTools(msg Message) error {
	a.toolCallCount = 0
	a.Logger().Info("Tool call counter reset", Field{Key: "agent_id", Value: a.ID()})

	// Send confirmation back to sender
	responseMsg := NewMessage().
		Type("tools_reset").
		From(a.ID()).
		To(msg.Sender()).
		WithPayload("Tool call counter has been reset").
		Build()

	return a.SendMessage(responseMsg)
}

// GetToolCallCount returns the current tool call count.
func (a *ToolLLMAgent) GetToolCallCount() int {
	return a.toolCallCount
}

// GetMaxToolCalls returns the maximum allowed tool calls.
func (a *ToolLLMAgent) GetMaxToolCalls() int {
	return a.maxToolCalls
}

// SetMaxToolCalls sets the maximum allowed tool calls.
func (a *ToolLLMAgent) SetMaxToolCalls(max int) {
	a.maxToolCalls = max
}

// ListAvailableTools returns the names of available tools.
func (a *ToolLLMAgent) ListAvailableTools() []string {
	tools := a.getAvailableTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}
