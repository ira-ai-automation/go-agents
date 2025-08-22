package agent

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BaseMessage provides a default implementation of the Message interface.
type BaseMessage struct {
	id        string
	msgType   string
	sender    string
	recipient string
	payload   interface{}
	timestamp time.Time
	priority  int
}

// MessageBuilder helps construct messages with a fluent API.
type MessageBuilder struct {
	msg *BaseMessage
}

// NewMessage creates a new message builder.
func NewMessage() *MessageBuilder {
	return &MessageBuilder{
		msg: &BaseMessage{
			id:        uuid.New().String(),
			timestamp: time.Now(),
			priority:  0,
		},
	}
}

// NewMessageFrom creates a new message builder with an existing message as a template.
func NewMessageFrom(template Message) *MessageBuilder {
	return &MessageBuilder{
		msg: &BaseMessage{
			id:        uuid.New().String(),
			msgType:   template.Type(),
			sender:    template.Sender(),
			recipient: template.Recipient(),
			payload:   template.Payload(),
			timestamp: time.Now(),
			priority:  template.Priority(),
		},
	}
}

// Type sets the message type.
func (b *MessageBuilder) Type(msgType string) *MessageBuilder {
	b.msg.msgType = msgType
	return b
}

// From sets the sender of the message.
func (b *MessageBuilder) From(sender string) *MessageBuilder {
	b.msg.sender = sender
	return b
}

// To sets the recipient of the message.
func (b *MessageBuilder) To(recipient string) *MessageBuilder {
	b.msg.recipient = recipient
	return b
}

// WithPayload sets the message payload.
func (b *MessageBuilder) WithPayload(payload interface{}) *MessageBuilder {
	b.msg.payload = payload
	return b
}

// WithPriority sets the message priority.
func (b *MessageBuilder) WithPriority(priority int) *MessageBuilder {
	b.msg.priority = priority
	return b
}

// Build creates the final message.
func (b *MessageBuilder) Build() Message {
	return b.msg
}

// ID returns the unique identifier for this message.
func (m *BaseMessage) ID() string {
	return m.id
}

// Type returns the message type/category.
func (m *BaseMessage) Type() string {
	return m.msgType
}

// Sender returns the ID of the agent that sent this message.
func (m *BaseMessage) Sender() string {
	return m.sender
}

// Recipient returns the ID of the intended recipient agent.
func (m *BaseMessage) Recipient() string {
	return m.recipient
}

// Payload returns the message data.
func (m *BaseMessage) Payload() interface{} {
	return m.payload
}

// Timestamp returns when the message was created.
func (m *BaseMessage) Timestamp() time.Time {
	return m.timestamp
}

// Priority returns the message priority (higher numbers = higher priority).
func (m *BaseMessage) Priority() int {
	return m.priority
}

// String returns a string representation of the message.
func (m *BaseMessage) String() string {
	return m.msgType + ":" + m.id
}

// MarshalJSON implements json.Marshaler for serialization.
func (m *BaseMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"id":        m.id,
		"type":      m.msgType,
		"sender":    m.sender,
		"recipient": m.recipient,
		"payload":   m.payload,
		"timestamp": m.timestamp,
		"priority":  m.priority,
	})
}

// UnmarshalJSON implements json.Unmarshaler for deserialization.
func (m *BaseMessage) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if id, ok := raw["id"].(string); ok {
		m.id = id
	}
	if msgType, ok := raw["type"].(string); ok {
		m.msgType = msgType
	}
	if sender, ok := raw["sender"].(string); ok {
		m.sender = sender
	}
	if recipient, ok := raw["recipient"].(string); ok {
		m.recipient = recipient
	}
	if payload, ok := raw["payload"]; ok {
		m.payload = payload
	}
	if timestampStr, ok := raw["timestamp"].(string); ok {
		if ts, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			m.timestamp = ts
		}
	}
	if priority, ok := raw["priority"].(float64); ok {
		m.priority = int(priority)
	}

	return nil
}

// Common message types
const (
	// MessageTypeRequest represents a request message
	MessageTypeRequest = "request"

	// MessageTypeResponse represents a response message
	MessageTypeResponse = "response"

	// MessageTypeEvent represents an event notification
	MessageTypeEvent = "event"

	// MessageTypeCommand represents a command message
	MessageTypeCommand = "command"

	// MessageTypeHeartbeat represents a heartbeat/ping message
	MessageTypeHeartbeat = "heartbeat"

	// MessageTypeError represents an error message
	MessageTypeError = "error"

	// MessageTypeBroadcast represents a broadcast message to all agents
	MessageTypeBroadcast = "broadcast"
)

// MessageRouter handles routing messages between agents.
type MessageRouter struct {
	registry Registry
	logger   Logger
}

// NewMessageRouter creates a new message router.
func NewMessageRouter(registry Registry, logger Logger) *MessageRouter {
	return &MessageRouter{
		registry: registry,
		logger:   logger,
	}
}

// Route delivers a message to its intended recipient.
func (r *MessageRouter) Route(msg Message) error {
	// Handle broadcast messages
	if msg.Type() == MessageTypeBroadcast {
		return r.broadcast(msg)
	}

	// Find the recipient agent
	recipient, exists := r.registry.Get(msg.Recipient())
	if !exists {
		r.logger.Warn("Recipient agent not found",
			Field{Key: "recipient", Value: msg.Recipient()},
			Field{Key: "message_id", Value: msg.ID()},
		)
		return NewAgentError(ErrAgentNotFound, "recipient agent not found: "+msg.Recipient())
	}

	// Deliver the message
	if err := recipient.SendMessage(msg); err != nil {
		r.logger.Error("Failed to deliver message",
			Field{Key: "error", Value: err},
			Field{Key: "recipient", Value: msg.Recipient()},
			Field{Key: "message_id", Value: msg.ID()},
		)
		return err
	}

	r.logger.Debug("Message routed successfully",
		Field{Key: "recipient", Value: msg.Recipient()},
		Field{Key: "message_id", Value: msg.ID()},
		Field{Key: "message_type", Value: msg.Type()},
	)

	return nil
}

// broadcast sends a message to all registered agents.
func (r *MessageRouter) broadcast(msg Message) error {
	agents := r.registry.List()
	errors := make([]error, 0)

	for _, agent := range agents {
		// Don't send broadcast back to sender
		if agent.ID() == msg.Sender() {
			continue
		}

		// Create a copy of the message for each recipient
		broadcastMsg := NewMessageFrom(msg).To(agent.ID()).Build()

		if err := agent.SendMessage(broadcastMsg); err != nil {
			r.logger.Warn("Failed to broadcast to agent",
				Field{Key: "error", Value: err},
				Field{Key: "agent_id", Value: agent.ID()},
				Field{Key: "message_id", Value: msg.ID()},
			)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		r.logger.Warn("Some broadcast deliveries failed",
			Field{Key: "failed_count", Value: len(errors)},
			Field{Key: "total_count", Value: len(agents)},
		)
	}

	return nil
}

// MessageQueue provides a priority queue for messages.
type MessageQueue struct {
	messages []Message
	maxSize  int
}

// NewMessageQueue creates a new message queue with the specified maximum size.
func NewMessageQueue(maxSize int) *MessageQueue {
	return &MessageQueue{
		messages: make([]Message, 0),
		maxSize:  maxSize,
	}
}

// Enqueue adds a message to the queue.
func (q *MessageQueue) Enqueue(msg Message) bool {
	if len(q.messages) >= q.maxSize {
		return false
	}

	// Insert message in priority order (higher priority first)
	inserted := false
	for i, existing := range q.messages {
		if msg.Priority() > existing.Priority() {
			// Insert at position i
			q.messages = append(q.messages[:i], append([]Message{msg}, q.messages[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		q.messages = append(q.messages, msg)
	}

	return true
}

// Dequeue removes and returns the highest priority message.
func (q *MessageQueue) Dequeue() (Message, bool) {
	if len(q.messages) == 0 {
		return nil, false
	}

	msg := q.messages[0]
	q.messages = q.messages[1:]
	return msg, true
}

// Size returns the current number of messages in the queue.
func (q *MessageQueue) Size() int {
	return len(q.messages)
}

// IsFull returns true if the queue is at maximum capacity.
func (q *MessageQueue) IsFull() bool {
	return len(q.messages) >= q.maxSize
}

// IsEmpty returns true if the queue has no messages.
func (q *MessageQueue) IsEmpty() bool {
	return len(q.messages) == 0
}
