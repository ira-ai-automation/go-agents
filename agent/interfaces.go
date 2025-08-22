// Package agent provides the core interfaces and types for building agent systems.
package agent

import (
	"context"
	"time"

	"github.com/agentarium/core/llm"
)

// Agent represents the core interface that all agents must implement.
// It provides lifecycle management and message handling capabilities.
type Agent interface {
	// ID returns the unique identifier for this agent
	ID() string

	// Name returns a human-readable name for this agent
	Name() string

	// Start begins the agent's execution in its own goroutine
	Start(ctx context.Context) error

	// Stop gracefully shuts down the agent
	Stop() error

	// IsRunning returns true if the agent is currently running
	IsRunning() bool

	// SendMessage sends a message to this agent
	SendMessage(msg Message) error

	// GetMailbox returns the agent's message channel for receiving messages
	GetMailbox() <-chan Message
}

// Message represents a message that can be sent between agents.
type Message interface {
	// ID returns the unique identifier for this message
	ID() string

	// Type returns the message type/category
	Type() string

	// Sender returns the ID of the agent that sent this message
	Sender() string

	// Recipient returns the ID of the intended recipient agent
	Recipient() string

	// Payload returns the message data
	Payload() interface{}

	// Timestamp returns when the message was created
	Timestamp() time.Time

	// Priority returns the message priority (higher numbers = higher priority)
	Priority() int
}

// Context provides execution context and shared state for agents.
type Context interface {
	// Context returns the underlying context.Context for cancellation and timeouts
	Context() context.Context

	// Get retrieves a value from the shared context
	Get(key string) (interface{}, bool)

	// Set stores a value in the shared context
	Set(key string, value interface{})

	// Delete removes a value from the shared context
	Delete(key string)

	// Logger returns the logger instance for this context
	Logger() Logger
}

// Registry manages agent registration and discovery.
type Registry interface {
	// Register adds an agent to the registry
	Register(agent Agent) error

	// Unregister removes an agent from the registry
	Unregister(agentID string) error

	// Get retrieves an agent by ID
	Get(agentID string) (Agent, bool)

	// List returns all registered agents
	List() []Agent

	// Find searches for agents by name pattern
	Find(namePattern string) []Agent
}

// Supervisor manages the lifecycle of multiple agents.
type Supervisor interface {
	// Start starts the supervisor
	Start() error

	// Spawn creates and starts a new agent
	Spawn(factory AgentFactory, config Config) (Agent, error)

	// Terminate stops and removes an agent
	Terminate(agentID string) error

	// Restart stops and restarts an agent
	Restart(agentID string) error

	// Monitor returns a channel that receives agent status updates
	Monitor() <-chan AgentStatus

	// Shutdown gracefully stops all managed agents
	Shutdown(ctx context.Context) error

	// RegisterFactory registers an agent factory
	RegisterFactory(factory AgentFactory)

	// UnregisterFactory unregisters an agent factory
	UnregisterFactory(factoryType string)

	// GetRouter returns the message router
	GetRouter() *MessageRouter

	// GetRegistry returns the agent registry
	GetRegistry() Registry

	// GetLLMManager returns the global LLM manager
	GetLLMManager() llm.Manager

	// SetLLMManager sets the global LLM manager
	SetLLMManager(manager llm.Manager)
}

// AgentFactory creates new agent instances.
type AgentFactory interface {
	// Create creates a new agent instance with the given configuration
	Create(config Config) (Agent, error)

	// Type returns the type of agent this factory creates
	Type() string
}

// Config holds configuration parameters for agents.
type Config interface {
	// Get retrieves a configuration value
	Get(key string) interface{}

	// GetString retrieves a string configuration value
	GetString(key string) string

	// GetInt retrieves an integer configuration value
	GetInt(key string) int

	// GetBool retrieves a boolean configuration value
	GetBool(key string) bool

	// GetDuration retrieves a duration configuration value
	GetDuration(key string) time.Duration

	// Set stores a configuration value
	Set(key string, value interface{})

	// Has checks if a configuration key exists
	Has(key string) bool
}

// Logger provides structured logging capabilities.
type Logger interface {
	// Debug logs a debug message
	Debug(msg string, fields ...Field)

	// Info logs an info message
	Info(msg string, fields ...Field)

	// Warn logs a warning message
	Warn(msg string, fields ...Field)

	// Error logs an error message
	Error(msg string, fields ...Field)

	// Fatal logs a fatal message and exits
	Fatal(msg string, fields ...Field)

	// With returns a new logger with additional fields
	With(fields ...Field) Logger
}

// Field represents a structured log field.
type Field struct {
	Key   string
	Value interface{}
}

// AgentStatus represents the current status of an agent.
type AgentStatus struct {
	AgentID   string
	Name      string
	Status    Status
	Error     error
	Timestamp time.Time
}

// Status represents the execution status of an agent.
type Status int

const (
	// StatusStopped indicates the agent is not running
	StatusStopped Status = iota

	// StatusStarting indicates the agent is in the process of starting
	StatusStarting

	// StatusRunning indicates the agent is running normally
	StatusRunning

	// StatusStopping indicates the agent is in the process of stopping
	StatusStopping

	// StatusError indicates the agent encountered an error
	StatusError
)

// String returns a string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}
