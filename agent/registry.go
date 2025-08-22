package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// AgentRegistry provides a thread-safe registry for agent management and discovery.
type AgentRegistry struct {
	agents   map[string]Agent
	metadata map[string]*AgentMetadata
	mu       sync.RWMutex
	logger   Logger
	hooks    []RegistryHook
}

// AgentMetadata holds additional information about registered agents.
type AgentMetadata struct {
	RegisteredAt time.Time
	Tags         []string
	Properties   map[string]interface{}
}

// RegistryHook allows external code to react to registry events.
type RegistryHook interface {
	OnAgentRegistered(agent Agent)
	OnAgentUnregistered(agentID string)
}

// NewAgentRegistry creates a new agent registry.
func NewAgentRegistry(logger Logger) Registry {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &AgentRegistry{
		agents:   make(map[string]Agent),
		metadata: make(map[string]*AgentMetadata),
		logger:   logger,
		hooks:    make([]RegistryHook, 0),
	}
}

// Register adds an agent to the registry.
func (r *AgentRegistry) Register(agent Agent) error {
	if agent == nil {
		return NewAgentError(ErrInvalidAgent, "agent cannot be nil")
	}

	agentID := agent.ID()
	if agentID == "" {
		return NewAgentError(ErrInvalidAgent, "agent ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if agent is already registered
	if _, exists := r.agents[agentID]; exists {
		return NewAgentError(ErrAgentExists, fmt.Sprintf("agent with ID %s already exists", agentID))
	}

	// Register the agent
	r.agents[agentID] = agent
	r.metadata[agentID] = &AgentMetadata{
		RegisteredAt: time.Now(),
		Tags:         make([]string, 0),
		Properties:   make(map[string]interface{}),
	}

	r.logger.Info("Agent registered",
		Field{Key: "agent_id", Value: agentID},
		Field{Key: "agent_name", Value: agent.Name()},
	)

	// Notify hooks
	r.notifyRegistered(agent)

	return nil
}

// Unregister removes an agent from the registry.
func (r *AgentRegistry) Unregister(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agentID]; !exists {
		return NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	// Remove the agent
	delete(r.agents, agentID)
	delete(r.metadata, agentID)

	r.logger.Info("Agent unregistered", Field{Key: "agent_id", Value: agentID})

	// Notify hooks
	r.notifyUnregistered(agentID)

	return nil
}

// Get retrieves an agent by ID.
func (r *AgentRegistry) Get(agentID string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	return agent, exists
}

// List returns all registered agents.
func (r *AgentRegistry) List() []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}

	return agents
}

// Find searches for agents by name pattern.
func (r *AgentRegistry) Find(namePattern string) []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Compile regex pattern
	regex, err := regexp.Compile(namePattern)
	if err != nil {
		r.logger.Warn("Invalid regex pattern", Field{Key: "pattern", Value: namePattern}, Field{Key: "error", Value: err})
		return []Agent{}
	}

	matches := make([]Agent, 0)
	for _, agent := range r.agents {
		if regex.MatchString(agent.Name()) {
			matches = append(matches, agent)
		}
	}

	return matches
}

// FindByTag searches for agents by tag.
func (r *AgentRegistry) FindByTag(tag string) []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches := make([]Agent, 0)
	for agentID, metadata := range r.metadata {
		for _, agentTag := range metadata.Tags {
			if agentTag == tag {
				if agent, exists := r.agents[agentID]; exists {
					matches = append(matches, agent)
				}
				break
			}
		}
	}

	return matches
}

// SetTags sets tags for an agent.
func (r *AgentRegistry) SetTags(agentID string, tags []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[agentID]
	if !exists {
		return NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	metadata.Tags = make([]string, len(tags))
	copy(metadata.Tags, tags)

	return nil
}

// GetTags returns tags for an agent.
func (r *AgentRegistry) GetTags(agentID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.metadata[agentID]
	if !exists {
		return nil, NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	tags := make([]string, len(metadata.Tags))
	copy(tags, metadata.Tags)

	return tags, nil
}

// SetProperty sets a property for an agent.
func (r *AgentRegistry) SetProperty(agentID string, key string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[agentID]
	if !exists {
		return NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	metadata.Properties[key] = value
	return nil
}

// GetProperty gets a property for an agent.
func (r *AgentRegistry) GetProperty(agentID string, key string) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.metadata[agentID]
	if !exists {
		return nil, NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	value, exists := metadata.Properties[key]
	if !exists {
		return nil, NewAgentError(ErrPropertyNotFound, fmt.Sprintf("property %s not found for agent %s", key, agentID))
	}

	return value, nil
}

// GetMetadata returns metadata for an agent.
func (r *AgentRegistry) GetMetadata(agentID string) (*AgentMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.metadata[agentID]
	if !exists {
		return nil, NewAgentError(ErrAgentNotFound, fmt.Sprintf("agent with ID %s not found", agentID))
	}

	// Return a copy to prevent external modification
	return &AgentMetadata{
		RegisteredAt: metadata.RegisteredAt,
		Tags:         append([]string(nil), metadata.Tags...),
		Properties:   copyMap(metadata.Properties),
	}, nil
}

// AddHook adds a registry hook.
func (r *AgentRegistry) AddHook(hook RegistryHook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hooks = append(r.hooks, hook)
}

// RemoveHook removes a registry hook.
func (r *AgentRegistry) RemoveHook(hook RegistryHook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, h := range r.hooks {
		if h == hook {
			r.hooks = append(r.hooks[:i], r.hooks[i+1:]...)
			break
		}
	}
}

// Stats returns registry statistics.
func (r *AgentRegistry) Stats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RegistryStats{
		TotalAgents: len(r.agents),
		TagStats:    make(map[string]int),
	}

	// Count agents by status
	for _, agent := range r.agents {
		if baseAgent, ok := agent.(*BaseAgent); ok {
			switch baseAgent.GetStatus() {
			case StatusRunning:
				stats.RunningAgents++
			case StatusStopped:
				stats.StoppedAgents++
			case StatusError:
				stats.ErrorAgents++
			}
		}
	}

	// Count tags
	for _, metadata := range r.metadata {
		for _, tag := range metadata.Tags {
			stats.TagStats[tag]++
		}
	}

	return stats
}

// notifyRegistered notifies all hooks about agent registration.
func (r *AgentRegistry) notifyRegistered(agent Agent) {
	for _, hook := range r.hooks {
		go func(h RegistryHook) {
			defer func() {
				if rec := recover(); rec != nil {
					r.logger.Error("Registry hook panicked on registration", Field{Key: "panic", Value: rec})
				}
			}()
			h.OnAgentRegistered(agent)
		}(hook)
	}
}

// notifyUnregistered notifies all hooks about agent unregistration.
func (r *AgentRegistry) notifyUnregistered(agentID string) {
	for _, hook := range r.hooks {
		go func(h RegistryHook, id string) {
			defer func() {
				if rec := recover(); rec != nil {
					r.logger.Error("Registry hook panicked on unregistration", Field{Key: "panic", Value: rec})
				}
			}()
			h.OnAgentUnregistered(id)
		}(hook, agentID)
	}
}

// RegistryStats holds statistics about the registry.
type RegistryStats struct {
	TotalAgents   int
	RunningAgents int
	StoppedAgents int
	ErrorAgents   int
	TagStats      map[string]int
}

// Discovery provides agent discovery capabilities across multiple registries.
type Discovery struct {
	registries []Registry
	mu         sync.RWMutex
	logger     Logger
}

// NewDiscovery creates a new discovery service.
func NewDiscovery(logger Logger) *Discovery {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &Discovery{
		registries: make([]Registry, 0),
		logger:     logger,
	}
}

// AddRegistry adds a registry to the discovery service.
func (d *Discovery) AddRegistry(registry Registry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.registries = append(d.registries, registry)
}

// RemoveRegistry removes a registry from the discovery service.
func (d *Discovery) RemoveRegistry(registry Registry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, r := range d.registries {
		if r == registry {
			d.registries = append(d.registries[:i], d.registries[i+1:]...)
			break
		}
	}
}

// FindAgents searches for agents across all registries.
func (d *Discovery) FindAgents(criteria SearchCriteria) []Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()

	allAgents := make([]Agent, 0)
	seen := make(map[string]bool)

	for _, registry := range d.registries {
		var agents []Agent

		// Apply search criteria
		if criteria.NamePattern != "" {
			agents = registry.Find(criteria.NamePattern)
		} else {
			agents = registry.List()
		}

		// Filter by additional criteria
		for _, agent := range agents {
			if seen[agent.ID()] {
				continue
			}

			if d.matchesCriteria(agent, registry, criteria) {
				allAgents = append(allAgents, agent)
				seen[agent.ID()] = true
			}
		}
	}

	// Sort results
	d.sortAgents(allAgents, criteria.SortBy)

	// Apply limit
	if criteria.Limit > 0 && len(allAgents) > criteria.Limit {
		allAgents = allAgents[:criteria.Limit]
	}

	return allAgents
}

// matchesCriteria checks if an agent matches the search criteria.
func (d *Discovery) matchesCriteria(agent Agent, registry Registry, criteria SearchCriteria) bool {
	// Check status filter
	if criteria.Status != nil {
		if baseAgent, ok := agent.(*BaseAgent); ok {
			if baseAgent.GetStatus() != *criteria.Status {
				return false
			}
		}
	}

	// Check tags filter
	if len(criteria.Tags) > 0 {
		if extRegistry, ok := registry.(*AgentRegistry); ok {
			agentTags, err := extRegistry.GetTags(agent.ID())
			if err != nil {
				return false
			}

			for _, requiredTag := range criteria.Tags {
				found := false
				for _, agentTag := range agentTags {
					if agentTag == requiredTag {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		}
	}

	return true
}

// sortAgents sorts agents based on the sort criteria.
func (d *Discovery) sortAgents(agents []Agent, sortBy string) {
	switch strings.ToLower(sortBy) {
	case "name":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Name() < agents[j].Name()
		})
	case "id":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].ID() < agents[j].ID()
		})
		// Default: no sorting
	}
}

// SearchCriteria defines criteria for agent discovery.
type SearchCriteria struct {
	NamePattern string
	Tags        []string
	Status      *Status
	SortBy      string
	Limit       int
}

// copyMap creates a deep copy of a map.
func copyMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for key, value := range original {
		copy[key] = value
	}
	return copy
}
