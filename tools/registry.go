package tools

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// BaseToolRegistry provides a basic implementation of ToolRegistry.
type BaseToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewBaseToolRegistry creates a new BaseToolRegistry.
func NewBaseToolRegistry() *BaseToolRegistry {
	return &BaseToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *BaseToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return NewToolError(ErrInvalidParameters, "tool cannot be nil", "")
	}

	name := tool.Name()
	if name == "" {
		return NewToolError(ErrInvalidParameters, "tool name cannot be empty", "")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return NewToolError(ErrInvalidParameters, fmt.Sprintf("tool with name '%s' already exists", name), name)
	}

	r.tools[name] = tool
	return nil
}

// Unregister removes a tool from the registry.
func (r *BaseToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return NewToolError(ErrToolNotFound, fmt.Sprintf("tool '%s' not found", name), name)
	}

	delete(r.tools, name)
	return nil
}

// Get retrieves a tool by name.
func (r *BaseToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tools.
func (r *BaseToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ListNames returns the names of all registered tools.
func (r *BaseToolRegistry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Find searches for tools by name pattern or description.
func (r *BaseToolRegistry) Find(query string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []Tool
	queryLower := strings.ToLower(query)

	// Try exact name match first
	if tool, exists := r.tools[query]; exists {
		matches = append(matches, tool)
		return matches
	}

	// Try pattern matching
	for _, tool := range r.tools {
		name := strings.ToLower(tool.Name())
		description := strings.ToLower(tool.Description())

		// Check if query matches name or description
		if strings.Contains(name, queryLower) || strings.Contains(description, queryLower) {
			matches = append(matches, tool)
			continue
		}

		// Try regex matching for advanced queries
		if matched, _ := regexp.MatchString(queryLower, name); matched {
			matches = append(matches, tool)
			continue
		}
		if matched, _ := regexp.MatchString(queryLower, description); matched {
			matches = append(matches, tool)
		}
	}

	return matches
}

// Clear removes all tools from the registry.
func (r *BaseToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = make(map[string]Tool)
}

// Count returns the number of registered tools.
func (r *BaseToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// Has checks if a tool with the given name exists.
func (r *BaseToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tools[name]
	return exists
}
