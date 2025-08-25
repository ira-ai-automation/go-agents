// Package mcp provides Model Context Protocol client implementation for external tool integration.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ira-ai-automation/go-agents/tools"
)

// MCPClient implements the Model Context Protocol client.
type MCPClient struct {
	mu         sync.RWMutex
	serverURL  string
	serverInfo *tools.MCPServerInfo
	client     *http.Client
	connected  bool
	timeout    time.Duration
}

// NewMCPClient creates a new MCP client.
func NewMCPClient(timeout time.Duration) *MCPClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &MCPClient{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// Connect establishes a connection to an MCP server.
func (c *MCPClient) Connect(ctx context.Context, serverURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate URL
	if _, err := url.Parse(serverURL); err != nil {
		return fmt.Errorf("invalid server URL: %v", err)
	}

	c.serverURL = serverURL

	// Get server info
	info, err := c.getServerInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get server info: %v", err)
	}

	c.serverInfo = info
	c.connected = true

	return nil
}

// Disconnect closes the connection to the MCP server.
func (c *MCPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	c.serverInfo = nil
	c.serverURL = ""

	return nil
}

// ListTools retrieves available tools from the MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]tools.Tool, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected to MCP server")
	}
	serverURL := c.serverURL
	c.mu.RUnlock()

	// Make request to list tools endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", serverURL+"/tools", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %s", resp.Status)
	}

	var response MCPToolListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert MCP tools to our tool interface
	var mcpTools []tools.Tool
	for _, toolDef := range response.Tools {
		mcpTool := &MCPTool{
			client:      c,
			name:        toolDef.Name,
			description: toolDef.Description,
			schema:      toolDef.InputSchema,
		}
		mcpTools = append(mcpTools, mcpTool)
	}

	return mcpTools, nil
}

// CallTool executes a tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, toolCall *tools.ToolCall) (*tools.ToolResult, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected to MCP server")
	}
	serverURL := c.serverURL
	c.mu.RUnlock()

	// Prepare the request
	request := MCPToolCallRequest{
		Method: "tools/call",
		Params: MCPToolCallParams{
			Name:      toolCall.Name,
			Arguments: toolCall.Parameters,
		},
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/tools/call", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return &tools.ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("request failed: %v", err),
			Duration:  time.Since(start),
			Timestamp: start,
		}, nil
	}
	defer resp.Body.Close()

	var response MCPToolCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return &tools.ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to decode response: %v", err),
			Duration:  time.Since(start),
			Timestamp: start,
		}, nil
	}

	// Convert MCP response to tool result
	result := &tools.ToolResult{
		Success:   resp.StatusCode == http.StatusOK && response.Error == "",
		Data:      response.Content,
		Duration:  time.Since(start),
		Timestamp: start,
		Metadata: map[string]interface{}{
			"mcp_request_id": response.ID,
			"mcp_method":     request.Method,
		},
	}

	if response.Error != "" {
		result.Error = response.Error
	}

	return result, nil
}

// IsConnected returns true if connected to an MCP server.
func (c *MCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetServerInfo returns information about the connected MCP server.
func (c *MCPClient) GetServerInfo() *tools.MCPServerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverInfo
}

// getServerInfo retrieves server information from the MCP server.
func (c *MCPClient) getServerInfo(ctx context.Context) (*tools.MCPServerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.serverURL+"/info", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %s", resp.Status)
	}

	var info tools.MCPServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	info.URL = c.serverURL
	return &info, nil
}

// MCP Protocol Types

// MCPToolDefinition represents a tool definition from MCP server.
type MCPToolDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema *tools.ToolSchema `json:"inputSchema"`
}

// MCPToolListResponse represents the response from listing tools.
type MCPToolListResponse struct {
	Tools []MCPToolDefinition `json:"tools"`
}

// MCPToolCallRequest represents a tool call request to MCP server.
type MCPToolCallRequest struct {
	Method string            `json:"method"`
	Params MCPToolCallParams `json:"params"`
	ID     string            `json:"id,omitempty"`
}

// MCPToolCallParams represents parameters for tool call.
type MCPToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolCallResponse represents the response from tool call.
type MCPToolCallResponse struct {
	ID      string      `json:"id,omitempty"`
	Content interface{} `json:"content,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// MCPTool wraps an MCP tool to implement our Tool interface.
type MCPTool struct {
	client      *MCPClient
	name        string
	description string
	schema      *tools.ToolSchema
}

// Name returns the tool name.
func (t *MCPTool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *MCPTool) Description() string {
	return t.description
}

// Schema returns the tool schema.
func (t *MCPTool) Schema() *tools.ToolSchema {
	return t.schema
}

// Execute executes the tool via MCP.
func (t *MCPTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	toolCall := &tools.ToolCall{
		ID:         fmt.Sprintf("mcp_%d", time.Now().UnixNano()),
		Name:       t.name,
		Parameters: params,
		Timestamp:  time.Now(),
	}

	return t.client.CallTool(ctx, toolCall)
}

// Validate validates the tool parameters.
func (t *MCPTool) Validate(params map[string]interface{}) error {
	if t.schema == nil {
		return nil // No schema to validate against
	}

	// Basic validation - check required fields
	for _, required := range t.schema.Required {
		if _, ok := params[required]; !ok {
			return fmt.Errorf("required parameter '%s' is missing", required)
		}
	}

	// TODO: Add more comprehensive JSON schema validation
	return nil
}
