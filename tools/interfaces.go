// Package tools provides tool execution capabilities for agents to interact with external resources.
package tools

import (
	"context"
	"time"
)

// Tool represents a tool that can be executed by an agent.
type Tool interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Schema returns the JSON schema for the tool's parameters
	Schema() *ToolSchema

	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)

	// Validate checks if the given parameters are valid for this tool
	Validate(params map[string]interface{}) error
}

// ToolSchema defines the JSON schema for a tool's parameters.
type ToolSchema struct {
	Type        string                   `json:"type"`
	Properties  map[string]*PropertySpec `json:"properties"`
	Required    []string                 `json:"required,omitempty"`
	Description string                   `json:"description,omitempty"`
}

// PropertySpec defines a property in the tool schema.
type PropertySpec struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Format      string      `json:"format,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`
	MinLength   *int        `json:"minLength,omitempty"`
	MaxLength   *int        `json:"maxLength,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Success   bool                   `json:"success"`
	Data      interface{}            `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Timestamp time.Time              `json:"timestamp"`
}

// ToolRegistry manages available tools for agents.
type ToolRegistry interface {
	// Register adds a tool to the registry
	Register(tool Tool) error

	// Unregister removes a tool from the registry
	Unregister(name string) error

	// Get retrieves a tool by name
	Get(name string) (Tool, bool)

	// List returns all registered tools
	List() []Tool

	// ListNames returns the names of all registered tools
	ListNames() []string

	// Find searches for tools by name pattern or description
	Find(query string) []Tool
}

// ToolExecutor handles tool execution for agents.
type ToolExecutor interface {
	// Execute runs a tool with the given parameters
	Execute(ctx context.Context, toolName string, params map[string]interface{}) (*ToolResult, error)

	// ExecuteWithTimeout runs a tool with a timeout
	ExecuteWithTimeout(ctx context.Context, toolName string, params map[string]interface{}, timeout time.Duration) (*ToolResult, error)

	// GetRegistry returns the tool registry
	GetRegistry() ToolRegistry

	// SetRegistry sets the tool registry
	SetRegistry(registry ToolRegistry)
}

// ToolCall represents a request to execute a tool.
type ToolCall struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
	Timestamp  time.Time              `json:"timestamp"`
}

// ToolCallResult represents the result of a tool call.
type ToolCallResult struct {
	ID        string      `json:"id"`
	ToolName  string      `json:"tool_name"`
	Result    *ToolResult `json:"result"`
	Timestamp time.Time   `json:"timestamp"`
}

// MCPClient represents a Model Context Protocol client for external tool integration.
type MCPClient interface {
	// Connect establishes a connection to an MCP server
	Connect(ctx context.Context, serverURL string) error

	// Disconnect closes the connection to the MCP server
	Disconnect() error

	// ListTools retrieves available tools from the MCP server
	ListTools(ctx context.Context) ([]Tool, error)

	// CallTool executes a tool on the MCP server
	CallTool(ctx context.Context, toolCall *ToolCall) (*ToolResult, error)

	// IsConnected returns true if connected to an MCP server
	IsConnected() bool

	// GetServerInfo returns information about the connected MCP server
	GetServerInfo() *MCPServerInfo
}

// MCPServerInfo contains information about an MCP server.
type MCPServerInfo struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	URL          string            `json:"url"`
}

// ToolConfig holds configuration for tool execution.
type ToolConfig struct {
	DefaultTimeout time.Duration `json:"default_timeout"`
	MaxConcurrent  int           `json:"max_concurrent"`
	RetryAttempts  int           `json:"retry_attempts"`
	RetryDelay     time.Duration `json:"retry_delay"`
	EnableLogging  bool          `json:"enable_logging"`
	LogLevel       string        `json:"log_level"`
	AllowedTools   []string      `json:"allowed_tools,omitempty"`
	BlockedTools   []string      `json:"blocked_tools,omitempty"`
	SecurityPolicy string        `json:"security_policy,omitempty"`
}

// DefaultToolConfig returns a default tool configuration.
func DefaultToolConfig() *ToolConfig {
	return &ToolConfig{
		DefaultTimeout: 30 * time.Second,
		MaxConcurrent:  10,
		RetryAttempts:  3,
		RetryDelay:     1 * time.Second,
		EnableLogging:  true,
		LogLevel:       "info",
		AllowedTools:   nil, // Allow all tools by default
		BlockedTools:   nil, // No blocked tools by default
		SecurityPolicy: "default",
	}
}

// ToolError represents an error that occurred during tool execution.
type ToolError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	ToolName   string `json:"tool_name"`
	Details    string `json:"details,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Error implements the error interface.
func (e *ToolError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// NewToolError creates a new ToolError.
func NewToolError(code, message, toolName string) *ToolError {
	return &ToolError{
		Code:     code,
		Message:  message,
		ToolName: toolName,
	}
}

// Common tool error codes
const (
	ErrToolNotFound      = "TOOL_NOT_FOUND"
	ErrInvalidParameters = "INVALID_PARAMETERS"
	ErrExecutionFailed   = "EXECUTION_FAILED"
	ErrTimeout           = "TIMEOUT"
	ErrPermissionDenied  = "PERMISSION_DENIED"
	ErrResourceNotFound  = "RESOURCE_NOT_FOUND"
	ErrNetworkError      = "NETWORK_ERROR"
	ErrInvalidResponse   = "INVALID_RESPONSE"
)
