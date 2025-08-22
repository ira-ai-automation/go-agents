// Package memory provides interfaces and implementations for agent memory systems.
package memory

import (
	"context"
	"time"
)

// Memory represents the core memory interface that all memory implementations must satisfy.
type Memory interface {
	// Store saves data with a key
	Store(ctx context.Context, key string, data interface{}) error

	// Retrieve gets data by key
	Retrieve(ctx context.Context, key string) (interface{}, error)

	// Delete removes data by key
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists
	Exists(ctx context.Context, key string) (bool, error)

	// Keys returns all keys (with optional prefix filter)
	Keys(ctx context.Context, prefix string) ([]string, error)

	// Clear removes all data
	Clear(ctx context.Context) error

	// Close closes the memory store
	Close() error
}

// PersistentMemory extends Memory with persistence capabilities.
type PersistentMemory interface {
	Memory

	// Persist forces a save to persistent storage
	Persist(ctx context.Context) error

	// Load restores data from persistent storage
	Load(ctx context.Context) error

	// GetStoragePath returns the storage path/connection string
	GetStoragePath() string

	// Backup creates a backup of the memory store
	Backup(ctx context.Context, path string) error

	// Restore restores from a backup
	Restore(ctx context.Context, path string) error
}

// CacheMemory extends Memory with caching capabilities.
type CacheMemory interface {
	Memory

	// StoreWithTTL stores data with a time-to-live
	StoreWithTTL(ctx context.Context, key string, data interface{}, ttl time.Duration) error

	// GetTTL returns the remaining time-to-live for a key
	GetTTL(ctx context.Context, key string) (time.Duration, error)

	// Refresh extends the TTL for a key
	Refresh(ctx context.Context, key string, ttl time.Duration) error

	// Stats returns cache statistics
	Stats() CacheStats

	// SetMaxSize sets the maximum cache size
	SetMaxSize(size int)

	// Evict removes expired entries
	Evict(ctx context.Context) error
}

// VectorMemory extends Memory with vector database capabilities for semantic search.
type VectorMemory interface {
	Memory

	// StoreVector stores data with its vector embedding
	StoreVector(ctx context.Context, key string, data interface{}, embedding []float32) error

	// StoreWithEmbedding stores data and generates embedding automatically
	StoreWithEmbedding(ctx context.Context, key string, data interface{}) error

	// SimilaritySearch finds similar items based on vector similarity
	SimilaritySearch(ctx context.Context, query string, limit int) ([]SimilarityResult, error)

	// SimilaritySearchByVector finds similar items using a vector
	SimilaritySearchByVector(ctx context.Context, embedding []float32, limit int) ([]SimilarityResult, error)

	// GetEmbedding returns the embedding for a key
	GetEmbedding(ctx context.Context, key string) ([]float32, error)

	// SetEmbeddingProvider sets the embedding generation provider
	SetEmbeddingProvider(provider EmbeddingProvider)

	// GetDimensions returns the vector dimensions
	GetDimensions() int
}

// CompositeMemory combines multiple memory types for comprehensive memory management.
type CompositeMemory interface {
	Memory
	PersistentMemory
	CacheMemory
	VectorMemory

	// GetCacheMemory returns the cache memory component
	GetCacheMemory() CacheMemory

	// GetPersistentMemory returns the persistent memory component
	GetPersistentMemory() PersistentMemory

	// GetVectorMemory returns the vector memory component
	GetVectorMemory() VectorMemory

	// Sync synchronizes cache with persistent storage
	Sync(ctx context.Context) error
}

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	// GenerateEmbedding creates a vector embedding from text
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)

	// GenerateBatchEmbeddings creates embeddings for multiple texts
	GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error)

	// GetDimensions returns the embedding vector dimensions
	GetDimensions() int

	// GetModel returns the embedding model name
	GetModel() string

	// IsConfigured returns true if the provider is properly configured
	IsConfigured() bool
}

// MemoryManager manages multiple memory stores for different agents.
type MemoryManager interface {
	// CreateMemory creates a new memory store for an agent
	CreateMemory(agentID string, config MemoryConfig) (CompositeMemory, error)

	// GetMemory retrieves memory store for an agent
	GetMemory(agentID string) (CompositeMemory, bool)

	// RemoveMemory removes memory store for an agent
	RemoveMemory(agentID string) error

	// ListAgents returns all agents with memory stores
	ListAgents() []string

	// SetGlobalEmbeddingProvider sets the default embedding provider
	SetGlobalEmbeddingProvider(provider EmbeddingProvider)

	// Shutdown gracefully shuts down all memory stores
	Shutdown(ctx context.Context) error
}

// MemoryConfig holds configuration for creating memory stores.
type MemoryConfig struct {
	// Persistent storage configuration
	PersistentType string                 // "file", "sqlite", "postgres", etc.
	PersistentPath string                 // File path or database connection string
	PersistentOpts map[string]interface{} // Provider-specific options

	// Cache configuration
	CacheType  string        // "memory", "redis", etc.
	CacheSize  int           // Maximum cache entries
	DefaultTTL time.Duration // Default time-to-live

	// Vector database configuration
	VectorType        string                 // "memory", "qdrant", "pinecone", "chroma", etc.
	VectorConfig      map[string]interface{} // Provider-specific config
	EmbeddingProvider EmbeddingProvider      // Custom embedding provider
	Dimensions        int                    // Vector dimensions

	// General options
	AutoPersist     bool          // Auto-save to persistent storage
	PersistInterval time.Duration // Auto-persist interval
	EnableSync      bool          // Enable cache-persistent sync
}

// SimilarityResult represents a result from similarity search.
type SimilarityResult struct {
	Key       string                 `json:"key"`
	Data      interface{}            `json:"data"`
	Score     float32                `json:"score"`    // Similarity score (0-1)
	Distance  float32                `json:"distance"` // Vector distance
	Embedding []float32              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CacheStats provides cache performance statistics.
type CacheStats struct {
	Hits         int64     `json:"hits"`
	Misses       int64     `json:"misses"`
	Evictions    int64     `json:"evictions"`
	Size         int       `json:"size"`
	MaxSize      int       `json:"max_size"`
	HitRate      float64   `json:"hit_rate"`
	MemoryUsage  int64     `json:"memory_usage"` // Bytes
	LastEviction time.Time `json:"last_eviction"`
	LastAccess   time.Time `json:"last_access"`
}

// MemoryEntry represents a stored memory item.
type MemoryEntry struct {
	Key        string                 `json:"key"`
	Data       interface{}            `json:"data"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	AccessedAt time.Time              `json:"accessed_at"`
	TTL        time.Duration          `json:"ttl,omitempty"`
	ExpiresAt  *time.Time             `json:"expires_at,omitempty"`
}

// ConversationMemory specialized memory for storing conversation history.
type ConversationMemory struct {
	AgentID      string    `json:"agent_id"`
	SessionID    string    `json:"session_id"`
	Messages     []Message `json:"messages"`
	Summary      string    `json:"summary,omitempty"`
	Topics       []string  `json:"topics,omitempty"`
	Sentiment    string    `json:"sentiment,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

// Message represents a conversation message.
type Message struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// KnowledgeEntry represents structured knowledge for agents.
type KnowledgeEntry struct {
	ID         string                 `json:"id"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	Category   string                 `json:"category"`
	Tags       []string               `json:"tags"`
	Source     string                 `json:"source,omitempty"`
	Confidence float32                `json:"confidence,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// MemorySearchFilter provides filtering options for memory searches.
type MemorySearchFilter struct {
	KeyPrefix     string                 `json:"key_prefix,omitempty"`
	Category      string                 `json:"category,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	CreatedAfter  *time.Time             `json:"created_after,omitempty"`
	CreatedBefore *time.Time             `json:"created_before,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
	Offset        int                    `json:"offset,omitempty"`
}

// Default memory types
const (
	// Persistent memory types
	PersistentTypeFile     = "file"
	PersistentTypeSQLite   = "sqlite"
	PersistentTypePostgres = "postgres"

	// Cache memory types
	CacheTypeMemory = "memory"
	CacheTypeRedis  = "redis"

	// Vector memory types
	VectorTypeMemory   = "memory"
	VectorTypeQdrant   = "qdrant"
	VectorTypePinecone = "pinecone"
	VectorTypeChroma   = "chroma"
	VectorTypeWeaviate = "weaviate"
)

// Helper functions for creating memory configurations
func NewMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		PersistentType:  PersistentTypeFile,
		PersistentOpts:  make(map[string]interface{}),
		CacheType:       CacheTypeMemory,
		CacheSize:       1000,
		DefaultTTL:      24 * time.Hour,
		VectorType:      VectorTypeMemory,
		VectorConfig:    make(map[string]interface{}),
		Dimensions:      384, // Default for many embedding models
		AutoPersist:     true,
		PersistInterval: 5 * time.Minute,
		EnableSync:      true,
	}
}

// WithPersistentStorage configures persistent storage
func (c *MemoryConfig) WithPersistentStorage(storageType, path string, opts map[string]interface{}) *MemoryConfig {
	c.PersistentType = storageType
	c.PersistentPath = path
	if opts != nil {
		c.PersistentOpts = opts
	}
	return c
}

// WithCache configures cache settings
func (c *MemoryConfig) WithCache(cacheType string, size int, ttl time.Duration) *MemoryConfig {
	c.CacheType = cacheType
	c.CacheSize = size
	c.DefaultTTL = ttl
	return c
}

// WithVectorDB configures vector database
func (c *MemoryConfig) WithVectorDB(vectorType string, dimensions int, config map[string]interface{}) *MemoryConfig {
	c.VectorType = vectorType
	c.Dimensions = dimensions
	if config != nil {
		c.VectorConfig = config
	}
	return c
}

// WithEmbeddingProvider sets the embedding provider
func (c *MemoryConfig) WithEmbeddingProvider(provider EmbeddingProvider) *MemoryConfig {
	c.EmbeddingProvider = provider
	return c
}
