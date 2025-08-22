package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// BaseAgent provides a default implementation of the Agent interface with
// goroutine-based lifecycle management and message handling.
type BaseAgent struct {
	id      string
	name    string
	mailbox chan Message
	status  int64 // atomic access - Status type
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
	logger  Logger
	config  Config

	// Handler functions
	onStart   func(ctx context.Context) error
	onStop    func() error
	onMessage func(msg Message) error
}

// BaseAgentConfig holds configuration for creating a BaseAgent.
type BaseAgentConfig struct {
	Name        string
	MailboxSize int
	Logger      Logger
	Config      Config
	OnStart     func(ctx context.Context) error
	OnStop      func() error
	OnMessage   func(msg Message) error
}

// NewBaseAgent creates a new BaseAgent with the given configuration.
func NewBaseAgent(config BaseAgentConfig) *BaseAgent {
	id := uuid.New().String()

	// Set defaults
	if config.MailboxSize <= 0 {
		config.MailboxSize = 100
	}
	if config.Logger == nil {
		config.Logger = NewDefaultLogger()
	}
	if config.Config == nil {
		config.Config = NewMapConfig()
	}

	agent := &BaseAgent{
		id:        id,
		name:      config.Name,
		mailbox:   make(chan Message, config.MailboxSize),
		logger:    config.Logger,
		config:    config.Config,
		onStart:   config.OnStart,
		onStop:    config.OnStop,
		onMessage: config.OnMessage,
	}

	atomic.StoreInt64(&agent.status, int64(StatusStopped))

	return agent
}

// ID returns the unique identifier for this agent.
func (a *BaseAgent) ID() string {
	return a.id
}

// Name returns a human-readable name for this agent.
func (a *BaseAgent) Name() string {
	return a.name
}

// Start begins the agent's execution in its own goroutine.
func (a *BaseAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.IsRunning() {
		return fmt.Errorf("agent %s is already running", a.id)
	}

	atomic.StoreInt64(&a.status, int64(StatusStarting))

	// Create cancellable context
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Start the main agent goroutine
	a.wg.Add(1)
	go a.run()

	a.logger.Info("Agent started", Field{Key: "agent_id", Value: a.id}, Field{Key: "name", Value: a.name})
	return nil
}

// Stop gracefully shuts down the agent.
func (a *BaseAgent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.IsRunning() {
		return nil
	}

	atomic.StoreInt64(&a.status, int64(StatusStopping))

	// Cancel the context to signal shutdown
	if a.cancel != nil {
		a.cancel()
	}

	// Wait for goroutines to finish
	a.wg.Wait()

	// Close mailbox
	close(a.mailbox)

	// Call custom stop handler
	if a.onStop != nil {
		if err := a.onStop(); err != nil {
			a.logger.Error("Error in onStop handler", Field{Key: "error", Value: err})
		}
	}

	atomic.StoreInt64(&a.status, int64(StatusStopped))
	a.logger.Info("Agent stopped", Field{Key: "agent_id", Value: a.id})

	return nil
}

// IsRunning returns true if the agent is currently running.
func (a *BaseAgent) IsRunning() bool {
	status := Status(atomic.LoadInt64(&a.status))
	return status == StatusRunning || status == StatusStarting
}

// GetStatus returns the current status of the agent.
func (a *BaseAgent) GetStatus() Status {
	return Status(atomic.LoadInt64(&a.status))
}

// SendMessage sends a message to this agent.
func (a *BaseAgent) SendMessage(msg Message) error {
	if !a.IsRunning() {
		return fmt.Errorf("agent %s is not running", a.id)
	}

	select {
	case a.mailbox <- msg:
		return nil
	case <-a.ctx.Done():
		return fmt.Errorf("agent %s is shutting down", a.id)
	default:
		return fmt.Errorf("agent %s mailbox is full", a.id)
	}
}

// GetMailbox returns the agent's message channel for receiving messages.
func (a *BaseAgent) GetMailbox() <-chan Message {
	return a.mailbox
}

// run is the main execution loop for the agent.
func (a *BaseAgent) run() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			atomic.StoreInt64(&a.status, int64(StatusError))
			a.logger.Error("Agent panicked", Field{Key: "panic", Value: r})
		}
	}()

	// Call custom start handler
	if a.onStart != nil {
		if err := a.onStart(a.ctx); err != nil {
			atomic.StoreInt64(&a.status, int64(StatusError))
			a.logger.Error("Error in onStart handler", Field{Key: "error", Value: err})
			return
		}
	}

	atomic.StoreInt64(&a.status, int64(StatusRunning))
	a.logger.Debug("Agent main loop started", Field{Key: "agent_id", Value: a.id})

	// Main message processing loop
	for {
		select {
		case <-a.ctx.Done():
			a.logger.Debug("Agent received shutdown signal", Field{Key: "agent_id", Value: a.id})
			return

		case msg, ok := <-a.mailbox:
			if !ok {
				a.logger.Debug("Agent mailbox closed", Field{Key: "agent_id", Value: a.id})
				return
			}

			a.handleMessage(msg)
		}
	}
}

// handleMessage processes a single message.
func (a *BaseAgent) handleMessage(msg Message) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("Message handler panicked",
				Field{Key: "panic", Value: r},
				Field{Key: "message_id", Value: msg.ID()},
				Field{Key: "message_type", Value: msg.Type()},
			)
		}
	}()

	a.logger.Debug("Processing message",
		Field{Key: "agent_id", Value: a.id},
		Field{Key: "message_id", Value: msg.ID()},
		Field{Key: "message_type", Value: msg.Type()},
		Field{Key: "sender", Value: msg.Sender()},
	)

	if a.onMessage != nil {
		if err := a.onMessage(msg); err != nil {
			a.logger.Error("Error processing message",
				Field{Key: "error", Value: err},
				Field{Key: "message_id", Value: msg.ID()},
			)
		}
	}
}

// Context returns the agent's execution context.
func (a *BaseAgent) Context() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctx
}

// Logger returns the agent's logger.
func (a *BaseAgent) Logger() Logger {
	return a.logger
}

// Config returns the agent's configuration.
func (a *BaseAgent) Config() Config {
	return a.config
}
