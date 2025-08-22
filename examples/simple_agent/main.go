package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/agentarium/core/agent"
)

// SimpleTaskAgent demonstrates a basic agent that processes tasks.
type SimpleTaskAgent struct {
	*agent.BaseAgent
	taskCount int
}

// NewSimpleTaskAgent creates a new simple task agent.
func NewSimpleTaskAgent(name string) *SimpleTaskAgent {
	taskAgent := &SimpleTaskAgent{
		taskCount: 0,
	}

	config := agent.BaseAgentConfig{
		Name:        name,
		MailboxSize: 50,
		Logger:      agent.NewDefaultLogger(),
		Config:      agent.NewMapConfig(),
		OnStart:     taskAgent.onStart,
		OnStop:      taskAgent.onStop,
		OnMessage:   taskAgent.onMessage,
	}

	taskAgent.BaseAgent = agent.NewBaseAgent(config)
	return taskAgent
}

func (a *SimpleTaskAgent) onStart(ctx context.Context) error {
	a.Logger().Info("Task agent starting up",
		agent.Field{Key: "agent_id", Value: a.ID()},
		agent.Field{Key: "agent_name", Value: a.Name()},
	)
	return nil
}

func (a *SimpleTaskAgent) onStop() error {
	a.Logger().Info("Task agent shutting down",
		agent.Field{Key: "agent_id", Value: a.ID()},
		agent.Field{Key: "tasks_processed", Value: a.taskCount},
	)
	return nil
}

func (a *SimpleTaskAgent) onMessage(msg agent.Message) error {
	switch msg.Type() {
	case "task":
		return a.processTask(msg)
	case "status":
		return a.handleStatusRequest(msg)
	default:
		a.Logger().Warn("Unknown message type",
			agent.Field{Key: "message_type", Value: msg.Type()},
			agent.Field{Key: "message_id", Value: msg.ID()},
		)
	}
	return nil
}

func (a *SimpleTaskAgent) processTask(msg agent.Message) error {
	task, ok := msg.Payload().(map[string]interface{})
	if !ok {
		return agent.NewAgentError(agent.ErrInvalidMessage, "invalid task payload")
	}

	taskName, exists := task["name"]
	if !exists {
		return agent.NewAgentError(agent.ErrInvalidMessage, "task name missing")
	}

	a.Logger().Info("Processing task",
		agent.Field{Key: "task_name", Value: taskName},
		agent.Field{Key: "task_count", Value: a.taskCount + 1},
	)

	// Simulate task processing
	time.Sleep(100 * time.Millisecond)

	a.taskCount++

	a.Logger().Info("Task completed",
		agent.Field{Key: "task_name", Value: taskName},
		agent.Field{Key: "total_tasks", Value: a.taskCount},
	)

	return nil
}

func (a *SimpleTaskAgent) handleStatusRequest(msg agent.Message) error {
	// For this example, we'll just log the status since we don't have a router
	a.Logger().Info("Status response prepared",
		agent.Field{Key: "response_to", Value: msg.Sender()},
		agent.Field{Key: "tasks_processed", Value: a.taskCount},
	)

	return nil
}

func main() {
	// Create a simple task agent
	taskAgent := NewSimpleTaskAgent("task-processor-1")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the agent
	if err := taskAgent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

	fmt.Println("Agent started successfully!")

	// Send some task messages
	for i := 0; i < 5; i++ {
		taskMsg := agent.NewMessage().
			Type("task").
			From("system").
			To(taskAgent.ID()).
			WithPayload(map[string]interface{}{
				"name":        fmt.Sprintf("task-%d", i+1),
				"description": fmt.Sprintf("Process item %d", i+1),
			}).
			Build()

		if err := taskAgent.SendMessage(taskMsg); err != nil {
			log.Printf("Failed to send task message: %v", err)
		}
	}

	// Send a status request
	statusMsg := agent.NewMessage().
		Type("status").
		From("system").
		To(taskAgent.ID()).
		Build()

	if err := taskAgent.SendMessage(statusMsg); err != nil {
		log.Printf("Failed to send status message: %v", err)
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Stop the agent
	if err := taskAgent.Stop(); err != nil {
		log.Printf("Error stopping agent: %v", err)
	}

	fmt.Println("Example completed successfully!")
}
