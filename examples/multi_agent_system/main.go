package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/ira-ai-automation/go-agents/agent"
)

// ProducerAgent generates work items for consumer agents.
type ProducerAgent struct {
	*agent.BaseAgent
	supervisor agent.Supervisor
	router     *agent.MessageRouter
	itemCount  int
}

// ConsumerAgent processes work items from producer agents.
type ConsumerAgent struct {
	*agent.BaseAgent
	processedCount int
}

// NewProducerAgent creates a new producer agent.
func NewProducerAgent(name string, supervisor agent.Supervisor, router *agent.MessageRouter) *ProducerAgent {
	producer := &ProducerAgent{
		supervisor: supervisor,
		router:     router,
		itemCount:  0,
	}

	config := agent.BaseAgentConfig{
		Name:        name,
		MailboxSize: 100,
		Logger:      agent.NewDefaultLogger(),
		Config:      agent.NewMapConfig(),
		OnStart:     producer.onStart,
		OnStop:      producer.onStop,
		OnMessage:   producer.onMessage,
	}

	producer.BaseAgent = agent.NewBaseAgent(config)
	return producer
}

func (p *ProducerAgent) onStart(ctx context.Context) error {
	p.Logger().Info("Producer agent starting",
		agent.Field{Key: "agent_id", Value: p.ID()},
	)

	// Start generating work items
	go p.generateWork(ctx)

	return nil
}

func (p *ProducerAgent) onStop() error {
	p.Logger().Info("Producer agent stopping",
		agent.Field{Key: "items_produced", Value: p.itemCount},
	)
	return nil
}

func (p *ProducerAgent) onMessage(msg agent.Message) error {
	switch msg.Type() {
	case "status_request":
		return p.handleStatusRequest(msg)
	default:
		p.Logger().Warn("Unknown message type",
			agent.Field{Key: "type", Value: msg.Type()},
		)
	}
	return nil
}

func (p *ProducerAgent) generateWork(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.itemCount++
			workItem := agent.NewMessage().
				Type("work_item").
				From(p.ID()).
				To("broadcast").
				WithPayload(map[string]interface{}{
					"id":         p.itemCount,
					"data":       fmt.Sprintf("Work item %d from %s", p.itemCount, p.Name()),
					"priority":   rand.Intn(10),
					"created_at": time.Now(),
				}).
				Build()

			if err := p.router.Route(workItem); err != nil {
				p.Logger().Error("Failed to route work item",
					agent.Field{Key: "error", Value: err},
					agent.Field{Key: "item_id", Value: p.itemCount},
				)
			} else {
				p.Logger().Info("Work item generated",
					agent.Field{Key: "item_id", Value: p.itemCount},
				)
			}
		}
	}
}

func (p *ProducerAgent) handleStatusRequest(msg agent.Message) error {
	response := agent.NewMessage().
		Type("status_response").
		From(p.ID()).
		To(msg.Sender()).
		WithPayload(map[string]interface{}{
			"agent_type":     "producer",
			"items_produced": p.itemCount,
			"status":         "running",
		}).
		Build()

	return p.router.Route(response)
}

// NewConsumerAgent creates a new consumer agent.
func NewConsumerAgent(name string) *ConsumerAgent {
	consumer := &ConsumerAgent{
		processedCount: 0,
	}

	config := agent.BaseAgentConfig{
		Name:        name,
		MailboxSize: 100,
		Logger:      agent.NewDefaultLogger(),
		Config:      agent.NewMapConfig(),
		OnStart:     consumer.onStart,
		OnStop:      consumer.onStop,
		OnMessage:   consumer.onMessage,
	}

	consumer.BaseAgent = agent.NewBaseAgent(config)
	return consumer
}

func (c *ConsumerAgent) onStart(ctx context.Context) error {
	c.Logger().Info("Consumer agent starting",
		agent.Field{Key: "agent_id", Value: c.ID()},
	)
	return nil
}

func (c *ConsumerAgent) onStop() error {
	c.Logger().Info("Consumer agent stopping",
		agent.Field{Key: "items_processed", Value: c.processedCount},
	)
	return nil
}

func (c *ConsumerAgent) onMessage(msg agent.Message) error {
	switch msg.Type() {
	case "work_item":
		return c.processWorkItem(msg)
	case "status_request":
		return c.handleStatusRequest(msg)
	default:
		c.Logger().Warn("Unknown message type",
			agent.Field{Key: "type", Value: msg.Type()},
		)
	}
	return nil
}

func (c *ConsumerAgent) processWorkItem(msg agent.Message) error {
	workItem, ok := msg.Payload().(map[string]interface{})
	if !ok {
		return agent.NewAgentError(agent.ErrInvalidMessage, "invalid work item payload")
	}

	itemID := workItem["id"]
	data := workItem["data"]
	priority := workItem["priority"]

	c.Logger().Info("Processing work item",
		agent.Field{Key: "item_id", Value: itemID},
		agent.Field{Key: "priority", Value: priority},
		agent.Field{Key: "data", Value: data},
	)

	// Simulate processing time based on priority
	if p, ok := priority.(int); ok {
		processingTime := time.Duration(100+p*50) * time.Millisecond
		time.Sleep(processingTime)
	}

	c.processedCount++

	c.Logger().Info("Work item completed",
		agent.Field{Key: "item_id", Value: itemID},
		agent.Field{Key: "total_processed", Value: c.processedCount},
	)

	return nil
}

func (c *ConsumerAgent) handleStatusRequest(msg agent.Message) error {
	// For simplicity, just log the status (would normally route response)
	c.Logger().Info("Status requested",
		agent.Field{Key: "requestor", Value: msg.Sender()},
		agent.Field{Key: "items_processed", Value: c.processedCount},
	)
	return nil
}

func main() {
	fmt.Println("Starting multi-agent system example...")

	// Create supervisor
	supervisorConfig := agent.SupervisorConfig{
		Logger: agent.NewDefaultLogger(),
	}
	supervisor := agent.NewSupervisor(supervisorConfig)

	if err := supervisor.Start(); err != nil {
		log.Fatalf("Failed to start supervisor: %v", err)
	}

	// Get router and registry
	router := supervisor.GetRouter()
	registry := supervisor.GetRegistry()

	// Create agent factories
	producerFactory := agent.NewSimpleAgentFactory("producer", func(config agent.Config) (agent.Agent, error) {
		name := config.GetString("name")
		if name == "" {
			name = "producer"
		}
		return NewProducerAgent(name, supervisor, router), nil
	})

	consumerFactory := agent.NewSimpleAgentFactory("consumer", func(config agent.Config) (agent.Agent, error) {
		name := config.GetString("name")
		if name == "" {
			name = "consumer"
		}
		return NewConsumerAgent(name), nil
	})

	// Register factories
	supervisor.RegisterFactory(producerFactory)
	supervisor.RegisterFactory(consumerFactory)

	// Create and spawn producer agents
	producerConfig := agent.NewMapConfig()
	producerConfig.Set("name", "producer-1")

	producer, err := supervisor.Spawn(producerFactory, producerConfig)
	if err != nil {
		log.Fatalf("Failed to spawn producer: %v", err)
	}

	// Create and spawn consumer agents
	for i := 1; i <= 3; i++ {
		consumerConfig := agent.NewMapConfig()
		consumerConfig.Set("name", fmt.Sprintf("consumer-%d", i))

		consumer, err := supervisor.Spawn(consumerFactory, consumerConfig)
		if err != nil {
			log.Printf("Failed to spawn consumer %d: %v", i, err)
			continue
		}

		fmt.Printf("Spawned consumer: %s\n", consumer.Name())
	}

	fmt.Printf("Spawned producer: %s\n", producer.Name())
	fmt.Printf("Total agents: %d\n", len(registry.List()))

	// Monitor agent status
	statusChan := supervisor.Monitor()
	go func() {
		for status := range statusChan {
			fmt.Printf("Agent Status Update: %s (%s) -> %s\n",
				status.Name, status.AgentID, status.Status.String())

			if status.Error != nil {
				fmt.Printf("  Error: %v\n", status.Error)
			}
		}
	}()

	// Let the system run for a while
	fmt.Println("System running... Press Ctrl+C to stop or wait 30 seconds")
	time.Sleep(30 * time.Second)

	// Shutdown the system
	fmt.Println("Shutting down system...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := supervisor.Shutdown(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	fmt.Println("Multi-agent system example completed!")
}
