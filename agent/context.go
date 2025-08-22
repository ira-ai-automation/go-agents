package agent

import (
	"context"
	"sync"
	"time"
)

// AgentContext provides execution context and shared state for agents.
type AgentContext struct {
	ctx    context.Context
	data   map[string]interface{}
	mu     sync.RWMutex
	logger Logger
}

// NewContext creates a new agent context.
func NewContext(ctx context.Context, logger Logger) Context {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &AgentContext{
		ctx:    ctx,
		data:   make(map[string]interface{}),
		logger: logger,
	}
}

// NewContextWithTimeout creates a new agent context with a timeout.
func NewContextWithTimeout(parent context.Context, timeout time.Duration, logger Logger) (Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return NewContext(ctx, logger), cancel
}

// NewContextWithCancel creates a new agent context with cancellation.
func NewContextWithCancel(parent context.Context, logger Logger) (Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	return NewContext(ctx, logger), cancel
}

// Context returns the underlying context.Context for cancellation and timeouts.
func (c *AgentContext) Context() context.Context {
	return c.ctx
}

// Get retrieves a value from the shared context.
func (c *AgentContext) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, exists := c.data[key]
	return value, exists
}

// Set stores a value in the shared context.
func (c *AgentContext) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

// Delete removes a value from the shared context.
func (c *AgentContext) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
}

// Logger returns the logger instance for this context.
func (c *AgentContext) Logger() Logger {
	return c.logger
}

// Keys returns all keys in the context.
func (c *AgentContext) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.data))
	for key := range c.data {
		keys = append(keys, key)
	}
	return keys
}

// Clone creates a copy of the context with the same data but a new underlying context.
func (c *AgentContext) Clone(ctx context.Context) Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	newContext := &AgentContext{
		ctx:    ctx,
		data:   make(map[string]interface{}),
		logger: c.logger,
	}

	// Copy all data
	for key, value := range c.data {
		newContext.data[key] = value
	}

	return newContext
}

// MapConfig provides a map-based configuration implementation.
type MapConfig struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

// NewMapConfig creates a new map-based configuration.
func NewMapConfig() Config {
	return &MapConfig{
		data: make(map[string]interface{}),
	}
}

// NewMapConfigFrom creates a new map-based configuration from existing data.
func NewMapConfigFrom(data map[string]interface{}) Config {
	config := &MapConfig{
		data: make(map[string]interface{}),
	}

	for key, value := range data {
		config.data[key] = value
	}

	return config
}

// Get retrieves a configuration value.
func (c *MapConfig) Get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.data[key]
}

// GetString retrieves a string configuration value.
func (c *MapConfig) GetString(key string) string {
	if value := c.Get(key); value != nil {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt retrieves an integer configuration value.
func (c *MapConfig) GetInt(key string) int {
	if value := c.Get(key); value != nil {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// GetBool retrieves a boolean configuration value.
func (c *MapConfig) GetBool(key string) bool {
	if value := c.Get(key); value != nil {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}

// GetDuration retrieves a duration configuration value.
func (c *MapConfig) GetDuration(key string) time.Duration {
	if value := c.Get(key); value != nil {
		switch v := value.(type) {
		case time.Duration:
			return v
		case string:
			if d, err := time.ParseDuration(v); err == nil {
				return d
			}
		case int64:
			return time.Duration(v)
		case float64:
			return time.Duration(int64(v))
		}
	}
	return 0
}

// Set stores a configuration value.
func (c *MapConfig) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

// Has checks if a configuration key exists.
func (c *MapConfig) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.data[key]
	return exists
}

// Keys returns all configuration keys.
func (c *MapConfig) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.data))
	for key := range c.data {
		keys = append(keys, key)
	}
	return keys
}

// Merge merges another configuration into this one.
func (c *MapConfig) Merge(other Config) {
	if otherMap, ok := other.(*MapConfig); ok {
		otherMap.mu.RLock()
		defer otherMap.mu.RUnlock()

		c.mu.Lock()
		defer c.mu.Unlock()

		for key, value := range otherMap.data {
			c.data[key] = value
		}
	}
}

// Clone creates a copy of the configuration.
func (c *MapConfig) Clone() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	newConfig := &MapConfig{
		data: make(map[string]interface{}),
	}

	for key, value := range c.data {
		newConfig.data[key] = value
	}

	return newConfig
}

// ContextManager manages contexts for multiple agents and provides
// hierarchical context management with inheritance.
type ContextManager struct {
	rootCtx   context.Context
	cancel    context.CancelFunc
	contexts  map[string]Context
	hierarchy map[string]string // child -> parent mapping
	mu        sync.RWMutex
	logger    Logger
}

// NewContextManager creates a new context manager.
func NewContextManager(logger Logger) *ContextManager {
	ctx, cancel := context.WithCancel(context.Background())

	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &ContextManager{
		rootCtx:   ctx,
		cancel:    cancel,
		contexts:  make(map[string]Context),
		hierarchy: make(map[string]string),
		logger:    logger,
	}
}

// CreateContext creates a new context for an agent.
func (cm *ContextManager) CreateContext(agentID string, parent string) Context {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var parentCtx context.Context = cm.rootCtx

	// If parent is specified, use its context
	if parent != "" {
		if parentContext, exists := cm.contexts[parent]; exists {
			parentCtx = parentContext.Context()
			cm.hierarchy[agentID] = parent
		}
	}

	// Create new context
	ctx := NewContext(parentCtx, cm.logger.With(Field{Key: "agent_id", Value: agentID}))
	cm.contexts[agentID] = ctx

	return ctx
}

// GetContext retrieves a context by agent ID.
func (cm *ContextManager) GetContext(agentID string) (Context, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ctx, exists := cm.contexts[agentID]
	return ctx, exists
}

// RemoveContext removes a context and all its children.
func (cm *ContextManager) RemoveContext(agentID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Remove all children first
	cm.removeChildren(agentID)

	// Remove from hierarchy
	delete(cm.hierarchy, agentID)

	// Remove the context
	delete(cm.contexts, agentID)
}

// removeChildren recursively removes all child contexts.
func (cm *ContextManager) removeChildren(parentID string) {
	for childID, parent := range cm.hierarchy {
		if parent == parentID {
			cm.removeChildren(childID)
			delete(cm.hierarchy, childID)
			delete(cm.contexts, childID)
		}
	}
}

// Shutdown gracefully shuts down the context manager and cancels all contexts.
func (cm *ContextManager) Shutdown() {
	cm.cancel()
}
