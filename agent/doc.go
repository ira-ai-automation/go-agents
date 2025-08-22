/*
Package agent provides a powerful and flexible agent-based system library for Go
that leverages goroutines for high-performance concurrent agent execution.

# Overview

The agent package is designed around several key interfaces that provide clean
abstractions for building agent-based systems:

  - Agent: Core agent lifecycle and message handling
  - Message: Communication between agents
  - Registry: Agent registration and discovery
  - Supervisor: Agent lifecycle management
  - Context: Execution context and shared state
  - Logger: Structured logging

# Core Concepts

## Agents

Agents are autonomous entities that run in their own goroutines and communicate
through message passing. Each agent has:

  - Unique identifier
  - Mailbox for receiving messages
  - Lifecycle management (start/stop)
  - Message processing capabilities

## Messages

Messages are the primary communication mechanism between agents. They contain:

  - Unique identifier
  - Message type
  - Sender and recipient information
  - Payload data
  - Priority and timestamp

## Registry

The registry provides agent discovery and metadata management:

  - Agent registration and unregistration
  - Search by name patterns
  - Tag-based filtering
  - Metadata storage

## Supervisor

The supervisor manages agent lifecycles and provides:

  - Agent spawning and termination
  - Health monitoring
  - Automatic restart capabilities
  - Graceful shutdown

# Quick Start

Create a simple agent:

	config := BaseAgentConfig{
		Name:        "my-agent",
		MailboxSize: 50,
		Logger:      NewDefaultLogger(),
		OnMessage: func(msg Message) error {
			log.Printf("Received: %s", msg.Type())
			return nil
		},
	}

	agent := NewBaseAgent(config)
	ctx := context.Background()
	agent.Start(ctx)

Send a message:

	msg := NewMessage().
		Type("hello").
		From("sender").
		To(agent.ID()).
		WithPayload("Hello, World!").
		Build()

	agent.SendMessage(msg)

# Advanced Usage

## Multi-Agent Systems

Use a supervisor to manage multiple agents:

	supervisor := NewSupervisor(SupervisorConfig{
		Logger: NewDefaultLogger(),
	})

	supervisor.Start()

	factory := NewSimpleAgentFactory("worker", func(config Config) (Agent, error) {
		return NewBaseAgent(BaseAgentConfig{
			Name:   config.GetString("name"),
			Logger: NewDefaultLogger(),
		}), nil
	})

	supervisor.RegisterFactory(factory)

	config := NewMapConfig()
	config.Set("name", "worker-1")
	worker, err := supervisor.Spawn(factory, config)

## Message Routing

Use a message router for complex communication patterns:

	registry := NewAgentRegistry(logger)
	router := NewMessageRouter(registry, logger)

	// Register agents with registry
	registry.Register(agent1)
	registry.Register(agent2)

	// Route messages
	msg := NewMessage().
		Type("task").
		From(agent1.ID()).
		To(agent2.ID()).
		WithPayload(taskData).
		Build()

	router.Route(msg)

## Error Handling

Robust error handling with custom error types:

	err := NewAgentError(ErrAgentNotFound, "agent not found").
		WithContext("agent_id", agentID)

	// Retry with exponential backoff
	retry := NewRetry(
		3, // max attempts
		NewExponentialBackoff(100*time.Millisecond, 5*time.Second, 2.0),
		logger,
	)

	err := retry.Do(func() error {
		return someOperationThatMightFail()
	})

## Structured Logging

Flexible logging with multiple implementations:

	// Default logger
	logger := NewDefaultLogger()

	// Structured logger
	logger := NewStructuredLogger(LogLevelInfo, encoder, output)

	// Multi-logger
	logger := NewMultiLogger(consoleLogger, fileLogger)

	// Use with fields
	logger.Info("Processing message",
		Field{Key: "agent_id", Value: agentID},
		Field{Key: "message_type", Value: msgType},
	)

# Best Practices

## Agent Design

1. Keep agents focused on single responsibilities
2. Handle errors gracefully using the error utilities
3. Use structured logging with relevant context
4. Implement proper lifecycle management

## Message Handling

1. Define clear message type constants
2. Validate message payloads
3. Handle unknown message types gracefully
4. Use appropriate message priorities

## Performance

1. Configure appropriate mailbox sizes
2. Monitor agent health using supervisor
3. Use appropriate logging levels
4. Implement proper backpressure handling

# Examples

See the examples directory for complete working examples:

  - simple_agent/: Basic agent creation and message handling
  - multi_agent_system/: Producer-consumer system with supervision

# Thread Safety

All components in this package are designed to be thread-safe and can be used
safely from multiple goroutines. The package extensively uses channels and
mutexes to ensure proper synchronization.

# Performance Considerations

The package is optimized for high-throughput scenarios:

  - Minimal memory allocations in hot paths
  - Efficient message passing using channels
  - Lock-free operations where possible
  - Configurable buffer sizes for performance tuning
*/
package agent
