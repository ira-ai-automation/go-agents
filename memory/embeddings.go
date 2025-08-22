package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// OpenAIEmbeddingProvider implements EmbeddingProvider using OpenAI's embedding API.
type OpenAIEmbeddingProvider struct {
	apiKey     string
	model      string
	baseURL    string
	dimensions int
	httpClient *http.Client
}

// OpenAI embedding API structures
type openAIEmbeddingRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

type openAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []openAIEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  openAIEmbeddingUsage  `json:"usage"`
	Error  *openAIEmbeddingError `json:"error,omitempty"`
}

type openAIEmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openAIEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type openAIEmbeddingError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider.
func NewOpenAIEmbeddingProvider(apiKey, model string) EmbeddingProvider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = "text-embedding-3-small" // Default to newer model
	}

	// Set dimensions based on model
	dimensions := 1536 // Default for most OpenAI models
	switch model {
	case "text-embedding-3-small":
		dimensions = 1536
	case "text-embedding-3-large":
		dimensions = 3072
	case "text-embedding-ada-002":
		dimensions = 1536
	}

	return &OpenAIEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		baseURL:    "https://api.openai.com/v1",
		dimensions: dimensions,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GenerateEmbedding creates a vector embedding from text.
func (p *OpenAIEmbeddingProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI embedding provider not configured")
	}

	embeddings, err := p.GenerateBatchEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

// GenerateBatchEmbeddings creates embeddings for multiple texts.
func (p *OpenAIEmbeddingProvider) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI embedding provider not configured")
	}

	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Prepare request
	request := openAIEmbeddingRequest{
		Input:          texts,
		Model:          p.model,
		EncodingFormat: "float",
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Make request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var embeddingResponse openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if embeddingResponse.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", embeddingResponse.Error.Message)
	}

	// Extract embeddings
	embeddings := make([][]float32, len(embeddingResponse.Data))
	for i, data := range embeddingResponse.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}

// GetDimensions returns the embedding vector dimensions.
func (p *OpenAIEmbeddingProvider) GetDimensions() int {
	return p.dimensions
}

// GetModel returns the embedding model name.
func (p *OpenAIEmbeddingProvider) GetModel() string {
	return p.model
}

// IsConfigured returns true if the provider is properly configured.
func (p *OpenAIEmbeddingProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// LocalEmbeddingProvider implements EmbeddingProvider using local/simple embedding methods.
// This is useful for testing or when you don't want to use external APIs.
type LocalEmbeddingProvider struct {
	model      string
	dimensions int
}

// NewLocalEmbeddingProvider creates a new local embedding provider.
func NewLocalEmbeddingProvider(dimensions int) EmbeddingProvider {
	if dimensions <= 0 {
		dimensions = 384 // Default dimension
	}

	return &LocalEmbeddingProvider{
		model:      "local-simple",
		dimensions: dimensions,
	}
}

// GenerateEmbedding creates a simple vector embedding from text using local methods.
func (p *LocalEmbeddingProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := p.GenerateBatchEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings generated")
	}

	return embeddings[0], nil
}

// GenerateBatchEmbeddings creates simple embeddings for multiple texts.
func (p *LocalEmbeddingProvider) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		embedding := p.generateSimpleEmbedding(text)
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// generateSimpleEmbedding creates a simple embedding using basic text features.
// This is for demonstration/testing purposes - not suitable for production.
func (p *LocalEmbeddingProvider) generateSimpleEmbedding(text string) []float32 {
	text = strings.ToLower(strings.TrimSpace(text))
	embedding := make([]float32, p.dimensions)

	// Simple hash-based embedding (not semantically meaningful)
	for i := 0; i < p.dimensions; i++ {
		value := 0.0
		for j, char := range text {
			value += float64(char) * math.Sin(float64(i+j+1))
		}
		embedding[i] = float32(math.Tanh(value / 1000.0)) // Normalize to [-1, 1]
	}

	// Normalize to unit vector
	return normalizeVector(embedding)
}

// GetDimensions returns the embedding vector dimensions.
func (p *LocalEmbeddingProvider) GetDimensions() int {
	return p.dimensions
}

// GetModel returns the embedding model name.
func (p *LocalEmbeddingProvider) GetModel() string {
	return p.model
}

// IsConfigured returns true (local provider is always configured).
func (p *LocalEmbeddingProvider) IsConfigured() bool {
	return true
}

// MockEmbeddingProvider provides a mock embedding provider for testing.
type MockEmbeddingProvider struct {
	model      string
	dimensions int
	responses  map[string][]float32 // Pre-defined responses
}

// NewMockEmbeddingProvider creates a new mock embedding provider.
func NewMockEmbeddingProvider(dimensions int) EmbeddingProvider {
	return &MockEmbeddingProvider{
		model:      "mock",
		dimensions: dimensions,
		responses:  make(map[string][]float32),
	}
}

// SetResponse sets a pre-defined response for a specific text.
func (p *MockEmbeddingProvider) SetResponse(text string, embedding []float32) {
	p.responses[text] = embedding
}

// GenerateEmbedding returns a mock embedding.
func (p *MockEmbeddingProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Check for pre-defined response
	if embedding, exists := p.responses[text]; exists {
		return embedding, nil
	}

	// Generate deterministic mock embedding
	embedding := make([]float32, p.dimensions)
	hash := simpleHash(text)

	for i := 0; i < p.dimensions; i++ {
		embedding[i] = float32(math.Sin(float64(hash+i))) * 0.5
	}

	return normalizeVector(embedding), nil
}

// GenerateBatchEmbeddings creates mock embeddings for multiple texts.
func (p *MockEmbeddingProvider) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		embedding, err := p.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// GetDimensions returns the embedding vector dimensions.
func (p *MockEmbeddingProvider) GetDimensions() int {
	return p.dimensions
}

// GetModel returns the embedding model name.
func (p *MockEmbeddingProvider) GetModel() string {
	return p.model
}

// IsConfigured returns true (mock provider is always configured).
func (p *MockEmbeddingProvider) IsConfigured() bool {
	return true
}

// EmbeddingCache wraps an embedding provider with caching.
type EmbeddingCache struct {
	provider EmbeddingProvider
	cache    map[string][]float32
	maxSize  int
	mu       sync.RWMutex
}

// NewEmbeddingCache creates a new embedding provider with caching.
func NewEmbeddingCache(provider EmbeddingProvider, maxSize int) EmbeddingProvider {
	return &EmbeddingCache{
		provider: provider,
		cache:    make(map[string][]float32),
		maxSize:  maxSize,
	}
}

// GenerateEmbedding generates embedding with caching.
func (c *EmbeddingCache) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Check cache first
	c.mu.RLock()
	if cached, exists := c.cache[text]; exists {
		c.mu.RUnlock()
		// Return a copy to prevent external modification
		result := make([]float32, len(cached))
		copy(result, cached)
		return result, nil
	}
	c.mu.RUnlock()

	// Generate embedding
	embedding, err := c.provider.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, err
	}

	// Cache the result
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entries if cache is full
	if len(c.cache) >= c.maxSize {
		// Simple eviction - remove random entry
		for key := range c.cache {
			delete(c.cache, key)
			break
		}
	}

	// Store in cache
	cached := make([]float32, len(embedding))
	copy(cached, embedding)
	c.cache[text] = cached

	return embedding, nil
}

// GenerateBatchEmbeddings generates batch embeddings with caching.
func (c *EmbeddingCache) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	var uncachedTexts []string
	var uncachedIndices []int
	results := make([][]float32, len(texts))

	// Check cache for each text
	c.mu.RLock()
	for i, text := range texts {
		if cached, exists := c.cache[text]; exists {
			// Use cached result
			result := make([]float32, len(cached))
			copy(result, cached)
			results[i] = result
		} else {
			// Need to generate
			uncachedTexts = append(uncachedTexts, text)
			uncachedIndices = append(uncachedIndices, i)
		}
	}
	c.mu.RUnlock()

	// Generate embeddings for uncached texts
	if len(uncachedTexts) > 0 {
		embeddings, err := c.provider.GenerateBatchEmbeddings(ctx, uncachedTexts)
		if err != nil {
			return nil, err
		}

		// Cache and store results
		c.mu.Lock()
		for i, embedding := range embeddings {
			textIndex := uncachedIndices[i]
			text := uncachedTexts[i]

			// Store in results
			results[textIndex] = embedding

			// Cache the result
			if len(c.cache) < c.maxSize {
				cached := make([]float32, len(embedding))
				copy(cached, embedding)
				c.cache[text] = cached
			}
		}
		c.mu.Unlock()
	}

	return results, nil
}

// GetDimensions returns the embedding vector dimensions.
func (c *EmbeddingCache) GetDimensions() int {
	return c.provider.GetDimensions()
}

// GetModel returns the embedding model name.
func (c *EmbeddingCache) GetModel() string {
	return c.provider.GetModel()
}

// IsConfigured returns true if the underlying provider is configured.
func (c *EmbeddingCache) IsConfigured() bool {
	return c.provider.IsConfigured()
}

// Helper functions

// normalizeVector normalizes a vector to unit length.
func normalizeVector(vector []float32) []float32 {
	var magnitude float64
	for _, v := range vector {
		magnitude += float64(v * v)
	}
	magnitude = math.Sqrt(magnitude)

	if magnitude == 0 {
		return vector
	}

	normalized := make([]float32, len(vector))
	for i, v := range vector {
		normalized[i] = float32(float64(v) / magnitude)
	}

	return normalized
}

// simpleHash creates a simple hash from a string.
func simpleHash(s string) int {
	hash := 0
	for _, char := range s {
		hash = hash*31 + int(char)
	}
	return hash
}

// EmbeddingConfig holds configuration for embedding providers.
type EmbeddingConfig struct {
	Provider   string                 `json:"provider"`   // "openai", "local", "mock"
	Model      string                 `json:"model"`      // Model name
	APIKey     string                 `json:"api_key"`    // API key for external providers
	BaseURL    string                 `json:"base_url"`   // Custom base URL
	Dimensions int                    `json:"dimensions"` // Vector dimensions
	CacheSize  int                    `json:"cache_size"` // Cache size (0 = no cache)
	Options    map[string]interface{} `json:"options"`    // Provider-specific options
}

// CreateEmbeddingProvider creates an embedding provider from configuration.
func CreateEmbeddingProvider(config EmbeddingConfig) (EmbeddingProvider, error) {
	var provider EmbeddingProvider

	switch strings.ToLower(config.Provider) {
	case "openai":
		provider = NewOpenAIEmbeddingProvider(config.APIKey, config.Model)

	case "local":
		dimensions := config.Dimensions
		if dimensions <= 0 {
			dimensions = 384
		}
		provider = NewLocalEmbeddingProvider(dimensions)

	case "mock":
		dimensions := config.Dimensions
		if dimensions <= 0 {
			dimensions = 384
		}
		provider = NewMockEmbeddingProvider(dimensions)

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", config.Provider)
	}

	// Add caching if requested
	if config.CacheSize > 0 {
		provider = NewEmbeddingCache(provider, config.CacheSize)
	}

	return provider, nil
}

// DefaultEmbeddingConfig returns a default embedding configuration.
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		Provider:   "local",
		Model:      "local-simple",
		Dimensions: 384,
		CacheSize:  1000,
		Options:    make(map[string]interface{}),
	}
}
