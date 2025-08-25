package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BaseToolExecutor provides a basic implementation of ToolExecutor.
type BaseToolExecutor struct {
	registry      ToolRegistry
	config        *ToolConfig
	mu            sync.RWMutex
	executing     map[string]int // Track concurrent executions per tool
	maxConcurrent int
}

// NewBaseToolExecutor creates a new BaseToolExecutor.
func NewBaseToolExecutor(registry ToolRegistry, config *ToolConfig) *BaseToolExecutor {
	if config == nil {
		config = DefaultToolConfig()
	}

	return &BaseToolExecutor{
		registry:      registry,
		config:        config,
		executing:     make(map[string]int),
		maxConcurrent: config.MaxConcurrent,
	}
}

// Execute runs a tool with the given parameters.
func (e *BaseToolExecutor) Execute(ctx context.Context, toolName string, params map[string]interface{}) (*ToolResult, error) {
	return e.ExecuteWithTimeout(ctx, toolName, params, e.config.DefaultTimeout)
}

// ExecuteWithTimeout runs a tool with a timeout.
func (e *BaseToolExecutor) ExecuteWithTimeout(ctx context.Context, toolName string, params map[string]interface{}, timeout time.Duration) (*ToolResult, error) {
	start := time.Now()

	// Check if tool is blocked
	if e.isToolBlocked(toolName) {
		return &ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("tool '%s' is blocked by security policy", toolName),
			Duration:  time.Since(start),
			Timestamp: start,
		}, NewToolError(ErrPermissionDenied, "tool is blocked", toolName)
	}

	// Check if tool is allowed
	if !e.isToolAllowed(toolName) {
		return &ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("tool '%s' is not in the allowed tools list", toolName),
			Duration:  time.Since(start),
			Timestamp: start,
		}, NewToolError(ErrPermissionDenied, "tool is not allowed", toolName)
	}

	// Get the tool
	tool, exists := e.registry.Get(toolName)
	if !exists {
		return &ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("tool '%s' not found", toolName),
			Duration:  time.Since(start),
			Timestamp: start,
		}, NewToolError(ErrToolNotFound, "tool not found", toolName)
	}

	// Check concurrency limits
	if err := e.checkConcurrencyLimit(toolName); err != nil {
		return &ToolResult{
			Success:   false,
			Error:     err.Error(),
			Duration:  time.Since(start),
			Timestamp: start,
		}, err
	}

	// Validate parameters
	if err := tool.Validate(params); err != nil {
		return &ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("invalid parameters: %v", err),
			Duration:  time.Since(start),
			Timestamp: start,
		}, NewToolError(ErrInvalidParameters, err.Error(), toolName)
	}

	// Track execution
	e.incrementExecution(toolName)
	defer e.decrementExecution(toolName)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retry logic
	var result *ToolResult
	var err error

	for attempt := 0; attempt <= e.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-execCtx.Done():
				return &ToolResult{
					Success:   false,
					Error:     "execution cancelled or timed out",
					Duration:  time.Since(start),
					Timestamp: start,
				}, NewToolError(ErrTimeout, "execution cancelled or timed out", toolName)
			case <-time.After(e.config.RetryDelay):
				// Continue to retry
			}
		}

		// Execute the tool
		result, err = tool.Execute(execCtx, params)
		if err == nil && result != nil && result.Success {
			result.Duration = time.Since(start)
			result.Timestamp = start
			return result, nil
		}

		// Check if we should retry
		if attempt < e.config.RetryAttempts && e.shouldRetry(err) {
			continue
		}

		break
	}

	// Execution failed
	if result == nil {
		result = &ToolResult{
			Success:   false,
			Error:     fmt.Sprintf("execution failed: %v", err),
			Duration:  time.Since(start),
			Timestamp: start,
		}
	} else {
		result.Duration = time.Since(start)
		result.Timestamp = start
	}

	if err == nil {
		err = NewToolError(ErrExecutionFailed, "tool execution failed", toolName)
	}

	return result, err
}

// GetRegistry returns the tool registry.
func (e *BaseToolExecutor) GetRegistry() ToolRegistry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.registry
}

// SetRegistry sets the tool registry.
func (e *BaseToolExecutor) SetRegistry(registry ToolRegistry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

// GetConfig returns the current configuration.
func (e *BaseToolExecutor) GetConfig() *ToolConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// SetConfig updates the configuration.
func (e *BaseToolExecutor) SetConfig(config *ToolConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
	e.maxConcurrent = config.MaxConcurrent
}

// isToolBlocked checks if a tool is in the blocked list.
func (e *BaseToolExecutor) isToolBlocked(toolName string) bool {
	if e.config.BlockedTools == nil {
		return false
	}

	for _, blocked := range e.config.BlockedTools {
		if blocked == toolName {
			return true
		}
	}
	return false
}

// isToolAllowed checks if a tool is in the allowed list (if specified).
func (e *BaseToolExecutor) isToolAllowed(toolName string) bool {
	if e.config.AllowedTools == nil {
		return true // Allow all if no explicit allow list
	}

	for _, allowed := range e.config.AllowedTools {
		if allowed == toolName {
			return true
		}
	}
	return false
}

// checkConcurrencyLimit checks if the tool can be executed based on concurrency limits.
func (e *BaseToolExecutor) checkConcurrencyLimit(toolName string) error {
	e.mu.RLock()
	current := e.executing[toolName]
	e.mu.RUnlock()

	if current >= e.maxConcurrent {
		return NewToolError(ErrExecutionFailed,
			fmt.Sprintf("tool '%s' has reached maximum concurrent executions (%d)", toolName, e.maxConcurrent),
			toolName)
	}

	return nil
}

// incrementExecution increments the execution counter for a tool.
func (e *BaseToolExecutor) incrementExecution(toolName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executing[toolName]++
}

// decrementExecution decrements the execution counter for a tool.
func (e *BaseToolExecutor) decrementExecution(toolName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.executing[toolName] > 0 {
		e.executing[toolName]--
		if e.executing[toolName] == 0 {
			delete(e.executing, toolName)
		}
	}
}

// shouldRetry determines if an error is retryable.
func (e *BaseToolExecutor) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	if toolErr, ok := err.(*ToolError); ok {
		switch toolErr.Code {
		case ErrNetworkError, ErrTimeout:
			return true
		case ErrToolNotFound, ErrPermissionDenied, ErrInvalidParameters:
			return false
		default:
			return true // Retry unknown errors
		}
	}

	return true // Retry non-tool errors
}

// GetExecutionStats returns current execution statistics.
func (e *BaseToolExecutor) GetExecutionStats() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := make(map[string]int)
	for tool, count := range e.executing {
		stats[tool] = count
	}
	return stats
}
