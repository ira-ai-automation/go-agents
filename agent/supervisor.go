package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ira-ai-automation/go-agents/llm"
	"golang.org/x/sync/errgroup"
)

// AgentSupervisor manages the lifecycle of multiple agents with supervision and restart capabilities.
type AgentSupervisor struct {
	registry       Registry
	contextManager *ContextManager
	router         *MessageRouter
	factories      map[string]AgentFactory
	statusChan     chan AgentStatus
	logger         Logger
	llmManager     llm.Manager // Global LLM manager
	mu             sync.RWMutex
	running        bool
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// SupervisorConfig holds configuration for creating a supervisor.
type SupervisorConfig struct {
	Registry       Registry
	ContextManager *ContextManager
	Logger         Logger
	LLMManager     llm.Manager // Global LLM manager
	StatusChanSize int
}

// NewSupervisor creates a new agent supervisor.
func NewSupervisor(config SupervisorConfig) Supervisor {
	if config.Registry == nil {
		config.Registry = NewAgentRegistry(config.Logger)
	}
	if config.ContextManager == nil {
		config.ContextManager = NewContextManager(config.Logger)
	}
	if config.Logger == nil {
		config.Logger = NewDefaultLogger()
	}
	if config.StatusChanSize <= 0 {
		config.StatusChanSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	supervisor := &AgentSupervisor{
		registry:       config.Registry,
		contextManager: config.ContextManager,
		router:         NewMessageRouter(config.Registry, config.Logger),
		factories:      make(map[string]AgentFactory),
		statusChan:     make(chan AgentStatus, config.StatusChanSize),
		logger:         config.Logger,
		llmManager:     config.LLMManager,
		ctx:            ctx,
		cancel:         cancel,
	}

	return supervisor
}

// RegisterFactory registers an agent factory with the supervisor.
func (s *AgentSupervisor) RegisterFactory(factory AgentFactory) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.factories[factory.Type()] = factory
	s.logger.Info("Agent factory registered", Field{Key: "type", Value: factory.Type()})
}

// UnregisterFactory unregisters an agent factory.
func (s *AgentSupervisor) UnregisterFactory(factoryType string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.factories, factoryType)
	s.logger.Info("Agent factory unregistered", Field{Key: "type", Value: factoryType})
}

// Start starts the supervisor.
func (s *AgentSupervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return NewAgentError(ErrAgentRunning, "supervisor is already running")
	}

	s.running = true

	// Start monitoring goroutine
	s.wg.Add(1)
	go s.monitor()

	s.logger.Info("Supervisor started")
	return nil
}

// Spawn creates and starts a new agent.
func (s *AgentSupervisor) Spawn(factory AgentFactory, config Config) (Agent, error) {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		return nil, NewAgentError(ErrAgentStopped, "supervisor is not running")
	}

	// Create the agent
	agent, err := factory.Create(config)
	if err != nil {
		s.logger.Error("Failed to create agent",
			Field{Key: "factory_type", Value: factory.Type()},
			Field{Key: "error", Value: err},
		)
		return nil, NewAgentErrorWithCause(ErrInvalidAgent, "failed to create agent", err)
	}

	// Register the agent
	if err := s.registry.Register(agent); err != nil {
		s.logger.Error("Failed to register agent",
			Field{Key: "agent_id", Value: agent.ID()},
			Field{Key: "error", Value: err},
		)
		return nil, err
	}

	// Create context for the agent
	agentCtx := s.contextManager.CreateContext(agent.ID(), "")

	// Start the agent
	if err := agent.Start(agentCtx.Context()); err != nil {
		s.registry.Unregister(agent.ID())
		s.contextManager.RemoveContext(agent.ID())
		s.logger.Error("Failed to start agent",
			Field{Key: "agent_id", Value: agent.ID()},
			Field{Key: "error", Value: err},
		)
		return nil, err
	}

	// Send status update
	s.sendStatusUpdate(AgentStatus{
		AgentID:   agent.ID(),
		Name:      agent.Name(),
		Status:    StatusRunning,
		Timestamp: time.Now(),
	})

	s.logger.Info("Agent spawned and started",
		Field{Key: "agent_id", Value: agent.ID()},
		Field{Key: "agent_name", Value: agent.Name()},
		Field{Key: "factory_type", Value: factory.Type()},
	)

	return agent, nil
}

// Terminate stops and removes an agent.
func (s *AgentSupervisor) Terminate(agentID string) error {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent %s not found", agentID))
	}

	// Stop the agent
	if err := agent.Stop(); err != nil {
		s.logger.Warn("Error stopping agent",
			Field{Key: "agent_id", Value: agentID},
			Field{Key: "error", Value: err},
		)
	}

	// Unregister the agent
	if err := s.registry.Unregister(agentID); err != nil {
		s.logger.Warn("Error unregistering agent",
			Field{Key: "agent_id", Value: agentID},
			Field{Key: "error", Value: err},
		)
	}

	// Remove context
	s.contextManager.RemoveContext(agentID)

	// Send status update
	s.sendStatusUpdate(AgentStatus{
		AgentID:   agentID,
		Name:      agent.Name(),
		Status:    StatusStopped,
		Timestamp: time.Now(),
	})

	s.logger.Info("Agent terminated", Field{Key: "agent_id", Value: agentID})
	return nil
}

// Restart stops and restarts an agent.
func (s *AgentSupervisor) Restart(agentID string) error {
	agent, exists := s.registry.Get(agentID)
	if !exists {
		return NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent %s not found", agentID))
	}

	// Stop the agent
	if err := agent.Stop(); err != nil {
		s.logger.Warn("Error stopping agent for restart",
			Field{Key: "agent_id", Value: agentID},
			Field{Key: "error", Value: err},
		)
	}

	// Wait a bit before restarting
	time.Sleep(100 * time.Millisecond)

	// Get or create context for the agent
	agentCtx, exists := s.contextManager.GetContext(agentID)
	if !exists {
		agentCtx = s.contextManager.CreateContext(agentID, "")
	}

	// Start the agent again
	if err := agent.Start(agentCtx.Context()); err != nil {
		s.logger.Error("Failed to restart agent",
			Field{Key: "agent_id", Value: agentID},
			Field{Key: "error", Value: err},
		)

		// Send error status
		s.sendStatusUpdate(AgentStatus{
			AgentID:   agentID,
			Name:      agent.Name(),
			Status:    StatusError,
			Error:     err,
			Timestamp: time.Now(),
		})

		return err
	}

	// Send status update
	s.sendStatusUpdate(AgentStatus{
		AgentID:   agentID,
		Name:      agent.Name(),
		Status:    StatusRunning,
		Timestamp: time.Now(),
	})

	s.logger.Info("Agent restarted", Field{Key: "agent_id", Value: agentID})
	return nil
}

// Monitor returns a channel that receives agent status updates.
func (s *AgentSupervisor) Monitor() <-chan AgentStatus {
	return s.statusChan
}

// Shutdown gracefully stops all managed agents.
func (s *AgentSupervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Supervisor shutting down")

	// Get all agents
	agents := s.registry.List()

	// Create error group for parallel shutdown
	g, gCtx := errgroup.WithContext(ctx)

	// Stop all agents in parallel
	for _, agent := range agents {
		agent := agent // Capture loop variable
		g.Go(func() error {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			default:
				if err := agent.Stop(); err != nil {
					s.logger.Warn("Error stopping agent during shutdown",
						Field{Key: "agent_id", Value: agent.ID()},
						Field{Key: "error", Value: err},
					)
				}

				// Unregister the agent
				s.registry.Unregister(agent.ID())

				// Send final status
				s.sendStatusUpdate(AgentStatus{
					AgentID:   agent.ID(),
					Name:      agent.Name(),
					Status:    StatusStopped,
					Timestamp: time.Now(),
				})

				return nil
			}
		})
	}

	// Wait for all agents to stop or timeout
	if err := g.Wait(); err != nil {
		s.logger.Error("Error during agent shutdown", Field{Key: "error", Value: err})
	}

	// Cancel supervisor context
	s.cancel()

	// Wait for monitoring goroutine to finish
	s.wg.Wait()

	// Close status channel
	close(s.statusChan)

	// Shutdown context manager
	s.contextManager.Shutdown()

	s.logger.Info("Supervisor shutdown complete")
	return nil
}

// GetRouter returns the message router.
func (s *AgentSupervisor) GetRouter() *MessageRouter {
	return s.router
}

// GetRegistry returns the agent registry.
func (s *AgentSupervisor) GetRegistry() Registry {
	return s.registry
}

// GetLLMManager returns the global LLM manager.
func (s *AgentSupervisor) GetLLMManager() llm.Manager {
	return s.llmManager
}

// SetLLMManager sets the global LLM manager.
func (s *AgentSupervisor) SetLLMManager(manager llm.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmManager = manager
}

// sendStatusUpdate sends an agent status update to the monitor channel.
func (s *AgentSupervisor) sendStatusUpdate(status AgentStatus) {
	select {
	case s.statusChan <- status:
	case <-s.ctx.Done():
		return
	default:
		// Channel is full, drop the update
		s.logger.Warn("Status channel full, dropping update",
			Field{Key: "agent_id", Value: status.AgentID},
		)
	}
}

// monitor runs in a goroutine to monitor agent health and handle restarts.
func (s *AgentSupervisor) monitor() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.checkAgentHealth()
		}
	}
}

// checkAgentHealth checks the health of all registered agents.
func (s *AgentSupervisor) checkAgentHealth() {
	agents := s.registry.List()

	for _, agent := range agents {
		// Check if agent is still running
		if baseAgent, ok := agent.(*BaseAgent); ok {
			status := baseAgent.GetStatus()

			// If agent has crashed, send error status
			if status == StatusError {
				s.sendStatusUpdate(AgentStatus{
					AgentID:   agent.ID(),
					Name:      agent.Name(),
					Status:    StatusError,
					Timestamp: time.Now(),
				})

				s.logger.Warn("Agent in error state detected",
					Field{Key: "agent_id", Value: agent.ID()},
					Field{Key: "agent_name", Value: agent.Name()},
				)
			}
		}
	}
}

// SimpleAgentFactory provides a basic implementation of AgentFactory.
type SimpleAgentFactory struct {
	factoryType string
	createFunc  func(config Config) (Agent, error)
}

// NewSimpleAgentFactory creates a new simple agent factory.
func NewSimpleAgentFactory(factoryType string, createFunc func(config Config) (Agent, error)) AgentFactory {
	return &SimpleAgentFactory{
		factoryType: factoryType,
		createFunc:  createFunc,
	}
}

// Create creates a new agent instance with the given configuration.
func (f *SimpleAgentFactory) Create(config Config) (Agent, error) {
	return f.createFunc(config)
}

// Type returns the type of agent this factory creates.
func (f *SimpleAgentFactory) Type() string {
	return f.factoryType
}
