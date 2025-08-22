package llm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LLMManager implements the Manager interface for managing multiple LLM providers.
type LLMManager struct {
	providers       map[string]Provider
	defaultProvider string
	mu              sync.RWMutex
}

// NewManager creates a new LLM manager.
func NewManager() Manager {
	return &LLMManager{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider registers an LLM provider.
func (m *LLMManager) RegisterProvider(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.providers[name] = provider

	// Set as default if it's the first provider or if it's OpenAI
	if m.defaultProvider == "" || name == ProviderOpenAI {
		m.defaultProvider = name
	}

	return nil
}

// GetProvider returns a provider by name.
func (m *LLMManager) GetProvider(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", name)
	}

	return provider, nil
}

// GetDefaultProvider returns the default provider.
func (m *LLMManager) GetDefaultProvider() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.defaultProvider == "" {
		return nil
	}

	return m.providers[m.defaultProvider]
}

// SetDefaultProvider sets the default provider.
func (m *LLMManager) SetDefaultProvider(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; !exists {
		return fmt.Errorf("provider '%s' not found", name)
	}

	m.defaultProvider = name
	return nil
}

// ListProviders returns all registered provider names.
func (m *LLMManager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}

	return names
}

// GenerateResponse generates a response using the specified or default provider.
func (m *LLMManager) GenerateResponse(ctx context.Context, providerName string, request Request) (*Response, error) {
	var provider Provider
	var err error

	if providerName == "" {
		provider = m.GetDefaultProvider()
		if provider == nil {
			return nil, fmt.Errorf("no default provider available")
		}
	} else {
		provider, err = m.GetProvider(providerName)
		if err != nil {
			return nil, err
		}
	}

	if !provider.IsConfigured() {
		return nil, fmt.Errorf("provider '%s' is not properly configured", provider.Name())
	}

	return provider.GenerateResponse(ctx, request)
}

// ProviderConfig implements the Config interface with a map-based storage.
type ProviderConfig struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

// NewProviderConfig creates a new provider configuration.
func NewProviderConfig() Config {
	return &ProviderConfig{
		data: make(map[string]interface{}),
	}
}

// GetAPIKey returns the API key for the provider.
func (c *ProviderConfig) GetAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if key, ok := c.data["api_key"].(string); ok {
		return key
	}
	return ""
}

// GetBaseURL returns the base URL for API calls.
func (c *ProviderConfig) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if url, ok := c.data["base_url"].(string); ok {
		return url
	}
	return ""
}

// GetModel returns the default model to use.
func (c *ProviderConfig) GetModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if model, ok := c.data["model"].(string); ok {
		return model
	}
	return ""
}

// GetTimeout returns the request timeout.
func (c *ProviderConfig) GetTimeout() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if timeout, ok := c.data["timeout"].(time.Duration); ok {
		return timeout
	}
	return 30 * time.Second // default timeout
}

// GetRetryAttempts returns the number of retry attempts.
func (c *ProviderConfig) GetRetryAttempts() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if attempts, ok := c.data["retry_attempts"].(int); ok {
		return attempts
	}
	return 3 // default retry attempts
}

// GetMetadata returns additional configuration metadata.
func (c *ProviderConfig) GetMetadata() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metadata := make(map[string]interface{})
	for key, value := range c.data {
		if key != "api_key" && key != "base_url" && key != "model" && key != "timeout" && key != "retry_attempts" {
			metadata[key] = value
		}
	}

	return metadata
}

// Set sets a configuration value.
func (c *ProviderConfig) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

// Get gets a configuration value.
func (c *ProviderConfig) Get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.data[key]
}

// SetAPIKey sets the API key.
func (c *ProviderConfig) SetAPIKey(key string) {
	c.Set("api_key", key)
}

// SetBaseURL sets the base URL.
func (c *ProviderConfig) SetBaseURL(url string) {
	c.Set("base_url", url)
}

// SetModel sets the default model.
func (c *ProviderConfig) SetModel(model string) {
	c.Set("model", model)
}

// SetTimeout sets the request timeout.
func (c *ProviderConfig) SetTimeout(timeout time.Duration) {
	c.Set("timeout", timeout)
}

// SetRetryAttempts sets the number of retry attempts.
func (c *ProviderConfig) SetRetryAttempts(attempts int) {
	c.Set("retry_attempts", attempts)
}
