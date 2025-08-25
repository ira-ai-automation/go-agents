package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ira-ai-automation/go-agents/tools"
)

// HTTPTool provides HTTP client operations.
type HTTPTool struct {
	client      *http.Client
	maxBodySize int64
	timeout     time.Duration
}

// NewHTTPTool creates a new HTTP tool.
func NewHTTPTool(timeout time.Duration, maxBodySize int64) *HTTPTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if maxBodySize == 0 {
		maxBodySize = 10 * 1024 * 1024 // 10MB default
	}

	return &HTTPTool{
		client: &http.Client{
			Timeout: timeout,
		},
		maxBodySize: maxBodySize,
		timeout:     timeout,
	}
}

// Name returns the tool name.
func (h *HTTPTool) Name() string {
	return "http"
}

// Description returns the tool description.
func (h *HTTPTool) Description() string {
	return "Performs HTTP requests (GET, POST, PUT, DELETE, etc.) to web APIs and endpoints"
}

// Schema returns the tool schema.
func (h *HTTPTool) Schema() *tools.ToolSchema {
	return &tools.ToolSchema{
		Type: "object",
		Properties: map[string]*tools.PropertySpec{
			"method": {
				Type:        "string",
				Description: "HTTP method",
				Enum:        []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"},
				Default:     "GET",
			},
			"url": {
				Type:        "string",
				Description: "The URL to request",
				Format:      "uri",
			},
			"headers": {
				Type:        "object",
				Description: "HTTP headers to include",
			},
			"body": {
				Type:        "string",
				Description: "Request body (for POST, PUT, PATCH)",
			},
			"json": {
				Type:        "object",
				Description: "JSON payload (alternative to body)",
			},
			"timeout": {
				Type:        "number",
				Description: "Request timeout in seconds",
				Minimum:     func() *float64 { v := 0.1; return &v }(),
				Maximum:     func() *float64 { v := 300.0; return &v }(),
			},
			"follow_redirects": {
				Type:        "boolean",
				Description: "Whether to follow redirects",
				Default:     true,
			},
		},
		Required:    []string{"url"},
		Description: "Make HTTP requests",
	}
}

// Validate checks if the parameters are valid.
func (h *HTTPTool) Validate(params map[string]interface{}) error {
	// Validate URL
	urlStr, ok := params["url"].(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("url parameter is required and must be a string")
	}

	if _, err := url.Parse(urlStr); err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Validate method if provided
	if method, ok := params["method"]; ok {
		if methodStr, ok := method.(string); ok {
			validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
			valid := false
			for _, m := range validMethods {
				if strings.ToUpper(methodStr) == m {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid HTTP method: %s", methodStr)
			}
		}
	}

	// Validate that body and json are not both provided
	if _, hasBody := params["body"]; hasBody {
		if _, hasJSON := params["json"]; hasJSON {
			return fmt.Errorf("cannot specify both 'body' and 'json' parameters")
		}
	}

	return nil
}

// Execute performs the HTTP request.
func (h *HTTPTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	// Extract parameters
	urlStr := params["url"].(string)
	method := h.getString(params, "method", "GET")
	method = strings.ToUpper(method)

	// Create request
	var body io.Reader
	var contentType string

	// Handle request body
	if jsonData, ok := params["json"]; ok {
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			return h.errorResult(fmt.Sprintf("failed to marshal JSON: %v", err)), nil
		}
		body = bytes.NewReader(jsonBytes)
		contentType = "application/json"
	} else if bodyStr, ok := params["body"]; ok {
		if bodyString, ok := bodyStr.(string); ok {
			body = strings.NewReader(bodyString)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return h.errorResult(fmt.Sprintf("failed to create request: %v", err)), nil
	}

	// Set content type
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add custom headers
	if headers, ok := params["headers"]; ok {
		if headerMap, ok := headers.(map[string]interface{}); ok {
			for key, value := range headerMap {
				if valueStr, ok := value.(string); ok {
					req.Header.Set(key, valueStr)
				}
			}
		}
	}

	// Configure client for this request
	client := h.client
	if timeoutSecs, ok := params["timeout"]; ok {
		if timeout, ok := timeoutSecs.(float64); ok {
			client = &http.Client{
				Timeout: time.Duration(timeout * float64(time.Second)),
			}
		}
	}

	// Handle redirect policy
	if followRedirects, ok := params["follow_redirects"]; ok {
		if follow, ok := followRedirects.(bool); ok && !follow {
			client = &http.Client{
				Timeout: client.Timeout,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		}
	}

	// Make the request
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return h.errorResult(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, h.maxBodySize)
	responseBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return h.errorResult(fmt.Sprintf("failed to read response: %v", err)), nil
	}

	// Parse response headers
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Try to parse JSON response
	var jsonResponse interface{}
	contentTypeHeader := resp.Header.Get("Content-Type")
	if strings.Contains(contentTypeHeader, "application/json") {
		json.Unmarshal(responseBody, &jsonResponse)
	}

	result := &tools.ToolResult{
		Success: resp.StatusCode >= 200 && resp.StatusCode < 300,
		Data: map[string]interface{}{
			"status_code":  resp.StatusCode,
			"status":       resp.Status,
			"headers":      headers,
			"body":         string(responseBody),
			"content_type": contentTypeHeader,
			"size":         len(responseBody),
			"duration_ms":  time.Since(start).Milliseconds(),
			"url":          urlStr,
			"method":       method,
		},
		Metadata: map[string]interface{}{
			"request_headers": req.Header,
			"response_size":   len(responseBody),
			"redirect_count":  len(resp.Request.URL.String()) != len(urlStr),
		},
	}

	// Add parsed JSON if available
	if jsonResponse != nil {
		result.Data.(map[string]interface{})["json"] = jsonResponse
	}

	// Set error message for non-success status codes
	if !result.Success {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return result, nil
}

// getString safely gets a string parameter with a default value.
func (h *HTTPTool) getString(params map[string]interface{}, key, defaultValue string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// errorResult creates an error result.
func (h *HTTPTool) errorResult(message string) *tools.ToolResult {
	return &tools.ToolResult{
		Success: false,
		Error:   message,
	}
}
