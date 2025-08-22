package agent

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"
)

// ErrorCode represents different types of agent errors.
type ErrorCode int

const (
	// ErrUnknown represents an unknown error
	ErrUnknown ErrorCode = iota

	// ErrAgentNotFound represents an agent not found error
	ErrAgentNotFound

	// ErrAgentExists represents an agent already exists error
	ErrAgentExists

	// ErrInvalidAgent represents an invalid agent error
	ErrInvalidAgent

	// ErrAgentStopped represents an agent stopped error
	ErrAgentStopped

	// ErrAgentStarting represents an agent already starting error
	ErrAgentStarting

	// ErrAgentRunning represents an agent already running error
	ErrAgentRunning

	// ErrInvalidMessage represents an invalid message error
	ErrInvalidMessage

	// ErrMessageDeliveryFailed represents a message delivery failure
	ErrMessageDeliveryFailed

	// ErrMailboxFull represents a full mailbox error
	ErrMailboxFull

	// ErrContextCancelled represents a cancelled context error
	ErrContextCancelled

	// ErrTimeout represents a timeout error
	ErrTimeout

	// ErrConfigNotFound represents a configuration not found error
	ErrConfigNotFound

	// ErrPropertyNotFound represents a property not found error
	ErrPropertyNotFound

	// ErrInvalidConfiguration represents an invalid configuration error
	ErrInvalidConfiguration

	// ErrResourceExhausted represents a resource exhausted error
	ErrResourceExhausted

	// ErrPermissionDenied represents a permission denied error
	ErrPermissionDenied

	// ErrOperationNotSupported represents an unsupported operation error
	ErrOperationNotSupported
)

// String returns a string representation of the error code.
func (e ErrorCode) String() string {
	switch e {
	case ErrUnknown:
		return "unknown"
	case ErrAgentNotFound:
		return "agent_not_found"
	case ErrAgentExists:
		return "agent_exists"
	case ErrInvalidAgent:
		return "invalid_agent"
	case ErrAgentStopped:
		return "agent_stopped"
	case ErrAgentStarting:
		return "agent_starting"
	case ErrAgentRunning:
		return "agent_running"
	case ErrInvalidMessage:
		return "invalid_message"
	case ErrMessageDeliveryFailed:
		return "message_delivery_failed"
	case ErrMailboxFull:
		return "mailbox_full"
	case ErrContextCancelled:
		return "context_cancelled"
	case ErrTimeout:
		return "timeout"
	case ErrConfigNotFound:
		return "config_not_found"
	case ErrPropertyNotFound:
		return "property_not_found"
	case ErrInvalidConfiguration:
		return "invalid_configuration"
	case ErrResourceExhausted:
		return "resource_exhausted"
	case ErrPermissionDenied:
		return "permission_denied"
	case ErrOperationNotSupported:
		return "operation_not_supported"
	default:
		return "unknown"
	}
}

// AgentError represents an error that occurred in the agent system.
type AgentError struct {
	Code       ErrorCode
	Message    string
	Cause      error
	StackTrace string
	Context    map[string]interface{}
}

// NewAgentError creates a new agent error.
func NewAgentError(code ErrorCode, message string) *AgentError {
	return &AgentError{
		Code:       code,
		Message:    message,
		StackTrace: getStackTrace(),
		Context:    make(map[string]interface{}),
	}
}

// NewAgentErrorWithCause creates a new agent error with a cause.
func NewAgentErrorWithCause(code ErrorCode, message string, cause error) *AgentError {
	return &AgentError{
		Code:       code,
		Message:    message,
		Cause:      cause,
		StackTrace: getStackTrace(),
		Context:    make(map[string]interface{}),
	}
}

// Error implements the error interface.
func (e *AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code.String(), e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code.String(), e.Message)
}

// WithContext adds context information to the error.
func (e *AgentError) WithContext(key string, value interface{}) *AgentError {
	e.Context[key] = value
	return e
}

// GetContext retrieves context information from the error.
func (e *AgentError) GetContext(key string) (interface{}, bool) {
	value, exists := e.Context[key]
	return value, exists
}

// Unwrap returns the underlying cause error.
func (e *AgentError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches a target error.
func (e *AgentError) Is(target error) bool {
	if targetAgentError, ok := target.(*AgentError); ok {
		return e.Code == targetAgentError.Code
	}
	return false
}

// getStackTrace captures the current stack trace.
func getStackTrace() string {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	// Clean up the stack trace to remove internal frames
	lines := strings.Split(string(buf), "\n")
	var cleaned []string
	skip := 0

	for i, line := range lines {
		if strings.Contains(line, "github.com/agentarium/core/agent.getStackTrace") ||
			strings.Contains(line, "github.com/agentarium/core/agent.NewAgentError") {
			skip = i + 2 // Skip the function and its file line
			continue
		}
		if i >= skip {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}

// ErrorHandler provides centralized error handling capabilities.
type ErrorHandler struct {
	handlers map[ErrorCode][]ErrorHandlerFunc
	logger   Logger
}

// ErrorHandlerFunc represents a function that handles specific error types.
type ErrorHandlerFunc func(err *AgentError)

// NewErrorHandler creates a new error handler.
func NewErrorHandler(logger Logger) *ErrorHandler {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &ErrorHandler{
		handlers: make(map[ErrorCode][]ErrorHandlerFunc),
		logger:   logger,
	}
}

// RegisterHandler registers a handler for a specific error code.
func (eh *ErrorHandler) RegisterHandler(code ErrorCode, handler ErrorHandlerFunc) {
	if eh.handlers[code] == nil {
		eh.handlers[code] = make([]ErrorHandlerFunc, 0)
	}
	eh.handlers[code] = append(eh.handlers[code], handler)
}

// Handle processes an error by calling all registered handlers.
func (eh *ErrorHandler) Handle(err error) {
	agentErr, ok := err.(*AgentError)
	if !ok {
		// Convert regular error to AgentError
		agentErr = NewAgentErrorWithCause(ErrUnknown, "unknown error", err)
	}

	// Log the error
	eh.logger.Error("Error handled",
		Field{Key: "error_code", Value: agentErr.Code.String()},
		Field{Key: "message", Value: agentErr.Message},
		Field{Key: "stack_trace", Value: agentErr.StackTrace},
	)

	// Call registered handlers
	if handlers, exists := eh.handlers[agentErr.Code]; exists {
		for _, handler := range handlers {
			go func(h ErrorHandlerFunc) {
				defer func() {
					if r := recover(); r != nil {
						eh.logger.Error("Error handler panicked", Field{Key: "panic", Value: r})
					}
				}()
				h(agentErr)
			}(handler)
		}
	}

	// Call generic handlers
	if handlers, exists := eh.handlers[ErrUnknown]; exists {
		for _, handler := range handlers {
			go func(h ErrorHandlerFunc) {
				defer func() {
					if r := recover(); r != nil {
						eh.logger.Error("Error handler panicked", Field{Key: "panic", Value: r})
					}
				}()
				h(agentErr)
			}(handler)
		}
	}
}

// Retry provides retry logic with exponential backoff.
type Retry struct {
	maxAttempts int
	backoff     BackoffStrategy
	logger      Logger
}

// BackoffStrategy defines how to calculate retry delays.
type BackoffStrategy interface {
	NextDelay(attempt int) time.Duration
	Reset()
}

// ExponentialBackoff implements exponential backoff strategy.
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
}

// NewExponentialBackoff creates a new exponential backoff strategy.
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		initialDelay: initial,
		maxDelay:     max,
		multiplier:   multiplier,
	}
}

// NextDelay calculates the next delay for the given attempt.
func (eb *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	delay := time.Duration(float64(eb.initialDelay) * math.Pow(eb.multiplier, float64(attempt)))
	if delay > eb.maxDelay {
		delay = eb.maxDelay
	}
	return delay
}

// Reset resets the backoff strategy.
func (eb *ExponentialBackoff) Reset() {
	// Nothing to reset for exponential backoff
}

// NewRetry creates a new retry instance.
func NewRetry(maxAttempts int, backoff BackoffStrategy, logger Logger) *Retry {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &Retry{
		maxAttempts: maxAttempts,
		backoff:     backoff,
		logger:      logger,
	}
}

// Do executes a function with retry logic.
func (r *Retry) Do(fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			if attempt > 0 {
				r.logger.Info("Operation succeeded after retry",
					Field{Key: "attempts", Value: attempt + 1},
				)
			}
			return nil
		}

		lastErr = err

		if attempt < r.maxAttempts-1 {
			delay := r.backoff.NextDelay(attempt)
			r.logger.Warn("Operation failed, retrying",
				Field{Key: "attempt", Value: attempt + 1},
				Field{Key: "max_attempts", Value: r.maxAttempts},
				Field{Key: "delay", Value: delay},
				Field{Key: "error", Value: err},
			)
			time.Sleep(delay)
		}
	}

	r.logger.Error("Operation failed after all retries",
		Field{Key: "attempts", Value: r.maxAttempts},
		Field{Key: "final_error", Value: lastErr},
	)

	return NewAgentErrorWithCause(ErrResourceExhausted,
		fmt.Sprintf("operation failed after %d attempts", r.maxAttempts), lastErr)
}
