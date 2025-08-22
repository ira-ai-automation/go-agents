package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultMemoryManager implements MemoryManager interface.
type DefaultMemoryManager struct {
	memories                map[string]CompositeMemory
	globalEmbeddingProvider EmbeddingProvider
	mu                      sync.RWMutex
	shutdown                chan struct{}
	wg                      sync.WaitGroup
}

// NewMemoryManager creates a new memory manager.
func NewMemoryManager() MemoryManager {
	return &DefaultMemoryManager{
		memories: make(map[string]CompositeMemory),
		shutdown: make(chan struct{}),
	}
}

// CreateMemory creates a new memory store for an agent.
func (m *DefaultMemoryManager) CreateMemory(agentID string, config MemoryConfig) (CompositeMemory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if memory already exists
	if _, exists := m.memories[agentID]; exists {
		return nil, fmt.Errorf("memory store for agent %s already exists", agentID)
	}

	// Use global embedding provider if none specified
	if config.EmbeddingProvider == nil && m.globalEmbeddingProvider != nil {
		config.EmbeddingProvider = m.globalEmbeddingProvider
	}

	// Create composite memory
	memory, err := NewCompositeMemory(agentID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory for agent %s: %w", agentID, err)
	}

	// Store the memory
	m.memories[agentID] = memory

	// Start auto-persistence if enabled
	if config.AutoPersist && config.PersistInterval > 0 {
		m.startAutoPersist(agentID, memory, config.PersistInterval)
	}

	return memory, nil
}

// GetMemory retrieves memory store for an agent.
func (m *DefaultMemoryManager) GetMemory(agentID string) (CompositeMemory, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memory, exists := m.memories[agentID]
	return memory, exists
}

// RemoveMemory removes memory store for an agent.
func (m *DefaultMemoryManager) RemoveMemory(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	memory, exists := m.memories[agentID]
	if !exists {
		return fmt.Errorf("memory store for agent %s not found", agentID)
	}

	// Close the memory store
	if err := memory.Close(); err != nil {
		return fmt.Errorf("failed to close memory for agent %s: %w", agentID, err)
	}

	// Remove from map
	delete(m.memories, agentID)

	return nil
}

// ListAgents returns all agents with memory stores.
func (m *DefaultMemoryManager) ListAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]string, 0, len(m.memories))
	for agentID := range m.memories {
		agents = append(agents, agentID)
	}

	return agents
}

// SetGlobalEmbeddingProvider sets the default embedding provider.
func (m *DefaultMemoryManager) SetGlobalEmbeddingProvider(provider EmbeddingProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.globalEmbeddingProvider = provider

	// Update existing memories that don't have a provider
	for _, memory := range m.memories {
		if vectorMem := memory.GetVectorMemory(); vectorMem != nil {
			vectorMem.SetEmbeddingProvider(provider)
		}
	}
}

// Shutdown gracefully shuts down all memory stores.
func (m *DefaultMemoryManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Signal shutdown
	close(m.shutdown)

	// Wait for auto-persist goroutines
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished
	case <-ctx.Done():
		// Timeout occurred
		return ctx.Err()
	}

	// Close all memory stores
	var errors []error
	for agentID, memory := range m.memories {
		if err := memory.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close memory for agent %s: %w", agentID, err))
		}
	}

	// Clear the map
	m.memories = make(map[string]CompositeMemory)

	// Return any errors
	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// startAutoPersist starts a goroutine for automatic persistence.
func (m *DefaultMemoryManager) startAutoPersist(agentID string, memory CompositeMemory, interval time.Duration) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-m.shutdown:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := memory.Persist(ctx); err != nil {
					// Log error but continue
					// Note: In a real implementation, you'd use a proper logger
					fmt.Printf("Auto-persist failed for agent %s: %v\n", agentID, err)
				}
				cancel()
			}
		}
	}()
}

// DefaultCompositeMemory implements CompositeMemory interface.
type DefaultCompositeMemory struct {
	agentID          string
	cacheMemory      CacheMemory
	persistentMemory PersistentMemory
	vectorMemory     VectorMemory
	config           MemoryConfig
	mu               sync.RWMutex
}

// NewCompositeMemory creates a new composite memory instance.
func NewCompositeMemory(agentID string, config MemoryConfig) (CompositeMemory, error) {
	memory := &DefaultCompositeMemory{
		agentID: agentID,
		config:  config,
	}

	var err error

	// Create cache memory
	memory.cacheMemory, err = createCacheMemory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache memory: %w", err)
	}

	// Create persistent memory
	memory.persistentMemory, err = createPersistentMemory(agentID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent memory: %w", err)
	}

	// Create vector memory
	memory.vectorMemory, err = createVectorMemory(agentID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector memory: %w", err)
	}

	// Load data from persistent storage
	if err := memory.persistentMemory.Load(context.Background()); err != nil {
		// Log warning but don't fail - might be first time
		fmt.Printf("Warning: Could not load persistent memory for agent %s: %v\n", agentID, err)
	}

	return memory, nil
}

// Store saves data with a key (stores in both cache and schedules for persistence).
func (m *DefaultCompositeMemory) Store(ctx context.Context, key string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store in cache first for fast access
	if err := m.cacheMemory.Store(ctx, key, data); err != nil {
		return fmt.Errorf("cache store failed: %w", err)
	}

	// Store in persistent memory
	if err := m.persistentMemory.Store(ctx, key, data); err != nil {
		return fmt.Errorf("persistent store failed: %w", err)
	}

	return nil
}

// Retrieve gets data by key (tries cache first, then persistent).
func (m *DefaultCompositeMemory) Retrieve(ctx context.Context, key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try cache first
	data, err := m.cacheMemory.Retrieve(ctx, key)
	if err == nil {
		return data, nil
	}

	// Try persistent memory
	data, err = m.persistentMemory.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}

	// Store in cache for future access
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.cacheMemory.Store(ctx, key, data)
	}()

	return data, nil
}

// Delete removes data by key from all stores.
func (m *DefaultCompositeMemory) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	// Delete from cache
	if err := m.cacheMemory.Delete(ctx, key); err != nil {
		errors = append(errors, fmt.Errorf("cache delete failed: %w", err))
	}

	// Delete from persistent memory
	if err := m.persistentMemory.Delete(ctx, key); err != nil {
		errors = append(errors, fmt.Errorf("persistent delete failed: %w", err))
	}

	// Delete from vector memory
	if err := m.vectorMemory.Delete(ctx, key); err != nil {
		errors = append(errors, fmt.Errorf("vector delete failed: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("delete errors: %v", errors)
	}

	return nil
}

// Exists checks if a key exists in any store.
func (m *DefaultCompositeMemory) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check cache first
	exists, err := m.cacheMemory.Exists(ctx, key)
	if err == nil && exists {
		return true, nil
	}

	// Check persistent memory
	return m.persistentMemory.Exists(ctx, key)
}

// Keys returns all keys from persistent memory.
func (m *DefaultCompositeMemory) Keys(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.persistentMemory.Keys(ctx, prefix)
}

// Clear removes all data from all stores.
func (m *DefaultCompositeMemory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	if err := m.cacheMemory.Clear(ctx); err != nil {
		errors = append(errors, err)
	}

	if err := m.persistentMemory.Clear(ctx); err != nil {
		errors = append(errors, err)
	}

	if err := m.vectorMemory.Clear(ctx); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("clear errors: %v", errors)
	}

	return nil
}

// Close closes all memory stores.
func (m *DefaultCompositeMemory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	if err := m.cacheMemory.Close(); err != nil {
		errors = append(errors, err)
	}

	if err := m.persistentMemory.Close(); err != nil {
		errors = append(errors, err)
	}

	if err := m.vectorMemory.Close(); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("close errors: %v", errors)
	}

	return nil
}

// Implement CompositeMemory interface methods
func (m *DefaultCompositeMemory) GetCacheMemory() CacheMemory {
	return m.cacheMemory
}

func (m *DefaultCompositeMemory) GetPersistentMemory() PersistentMemory {
	return m.persistentMemory
}

func (m *DefaultCompositeMemory) GetVectorMemory() VectorMemory {
	return m.vectorMemory
}

// Implement PersistentMemory interface methods
func (m *DefaultCompositeMemory) Persist(ctx context.Context) error {
	return m.persistentMemory.Persist(ctx)
}

func (m *DefaultCompositeMemory) Load(ctx context.Context) error {
	return m.persistentMemory.Load(ctx)
}

func (m *DefaultCompositeMemory) GetStoragePath() string {
	return m.persistentMemory.GetStoragePath()
}

func (m *DefaultCompositeMemory) Backup(ctx context.Context, path string) error {
	return m.persistentMemory.Backup(ctx, path)
}

func (m *DefaultCompositeMemory) Restore(ctx context.Context, path string) error {
	return m.persistentMemory.Restore(ctx, path)
}

// Implement CacheMemory interface methods
func (m *DefaultCompositeMemory) StoreWithTTL(ctx context.Context, key string, data interface{}, ttl time.Duration) error {
	return m.cacheMemory.StoreWithTTL(ctx, key, data, ttl)
}

func (m *DefaultCompositeMemory) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	return m.cacheMemory.GetTTL(ctx, key)
}

func (m *DefaultCompositeMemory) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	return m.cacheMemory.Refresh(ctx, key, ttl)
}

func (m *DefaultCompositeMemory) Stats() CacheStats {
	return m.cacheMemory.Stats()
}

func (m *DefaultCompositeMemory) SetMaxSize(size int) {
	m.cacheMemory.SetMaxSize(size)
}

func (m *DefaultCompositeMemory) Evict(ctx context.Context) error {
	return m.cacheMemory.Evict(ctx)
}

// Implement VectorMemory interface methods
func (m *DefaultCompositeMemory) StoreVector(ctx context.Context, key string, data interface{}, embedding []float32) error {
	// Store in vector memory first
	if err := m.vectorMemory.StoreVector(ctx, key, data, embedding); err != nil {
		return err
	}

	// Also store in regular memory for non-vector access
	return m.Store(ctx, key, data)
}

func (m *DefaultCompositeMemory) StoreWithEmbedding(ctx context.Context, key string, data interface{}) error {
	// Store in vector memory with auto-embedding
	if err := m.vectorMemory.StoreWithEmbedding(ctx, key, data); err != nil {
		return err
	}

	// Also store in regular memory
	return m.Store(ctx, key, data)
}

func (m *DefaultCompositeMemory) SimilaritySearch(ctx context.Context, query string, limit int) ([]SimilarityResult, error) {
	return m.vectorMemory.SimilaritySearch(ctx, query, limit)
}

func (m *DefaultCompositeMemory) SimilaritySearchByVector(ctx context.Context, embedding []float32, limit int) ([]SimilarityResult, error) {
	return m.vectorMemory.SimilaritySearchByVector(ctx, embedding, limit)
}

func (m *DefaultCompositeMemory) GetEmbedding(ctx context.Context, key string) ([]float32, error) {
	return m.vectorMemory.GetEmbedding(ctx, key)
}

func (m *DefaultCompositeMemory) SetEmbeddingProvider(provider EmbeddingProvider) {
	m.vectorMemory.SetEmbeddingProvider(provider)
}

func (m *DefaultCompositeMemory) GetDimensions() int {
	return m.vectorMemory.GetDimensions()
}

// Sync synchronizes cache with persistent storage.
func (m *DefaultCompositeMemory) Sync(ctx context.Context) error {
	// Implementation would sync cache with persistent storage
	// For now, just persist the cache data
	return m.Persist(ctx)
}

// Factory functions for creating different memory types
func createCacheMemory(config MemoryConfig) (CacheMemory, error) {
	switch config.CacheType {
	case CacheTypeMemory:
		return NewInMemoryCache(config.CacheSize, config.DefaultTTL), nil
	case CacheTypeRedis:
		// Would create Redis cache implementation
		return nil, fmt.Errorf("Redis cache not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported cache type: %s", config.CacheType)
	}
}

func createPersistentMemory(agentID string, config MemoryConfig) (PersistentMemory, error) {
	switch config.PersistentType {
	case PersistentTypeFile:
		path := config.PersistentPath
		if path == "" {
			path = fmt.Sprintf("./data/%s.json", agentID)
		}
		return NewFileMemory(path), nil
	case PersistentTypeSQLite:
		// Would create SQLite implementation
		return nil, fmt.Errorf("SQLite memory not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported persistent type: %s", config.PersistentType)
	}
}

func createVectorMemory(agentID string, config MemoryConfig) (VectorMemory, error) {
	switch config.VectorType {
	case VectorTypeMemory:
		return NewInMemoryVectorDB(config.Dimensions, config.EmbeddingProvider), nil
	case VectorTypeQdrant:
		// Would create Qdrant implementation
		return nil, fmt.Errorf("Qdrant vector memory not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported vector type: %s", config.VectorType)
	}
}
