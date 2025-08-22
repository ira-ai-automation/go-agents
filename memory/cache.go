package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemoryCache implements CacheMemory with LRU eviction and TTL support.
type InMemoryCache struct {
	data        map[string]*cacheEntry
	accessOrder []string // LRU tracking
	maxSize     int
	defaultTTL  time.Duration
	stats       CacheStats
	mu          sync.RWMutex
}

type cacheEntry struct {
	data       interface{}
	createdAt  time.Time
	expiresAt  *time.Time
	lastAccess time.Time
}

// NewInMemoryCache creates a new in-memory cache.
func NewInMemoryCache(maxSize int, defaultTTL time.Duration) CacheMemory {
	return &InMemoryCache{
		data:        make(map[string]*cacheEntry),
		accessOrder: make([]string, 0),
		maxSize:     maxSize,
		defaultTTL:  defaultTTL,
		stats: CacheStats{
			MaxSize: maxSize,
		},
	}
}

// Store saves data with a key.
func (c *InMemoryCache) Store(ctx context.Context, key string, data interface{}) error {
	return c.StoreWithTTL(ctx, key, data, c.defaultTTL)
}

// StoreWithTTL stores data with a specific time-to-live.
func (c *InMemoryCache) StoreWithTTL(ctx context.Context, key string, data interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiresAt *time.Time
	if ttl > 0 {
		expiry := now.Add(ttl)
		expiresAt = &expiry
	}

	entry := &cacheEntry{
		data:       data,
		createdAt:  now,
		expiresAt:  expiresAt,
		lastAccess: now,
	}

	// Check if key already exists
	if _, exists := c.data[key]; !exists {
		// New entry - check if we need to evict
		if len(c.data) >= c.maxSize {
			c.evictLRU()
		}
		c.accessOrder = append(c.accessOrder, key)
	} else {
		// Update access order
		c.updateAccessOrder(key)
	}

	c.data[key] = entry
	c.stats.Size = len(c.data)

	return nil
}

// Retrieve gets data by key.
func (c *InMemoryCache) Retrieve(ctx context.Context, key string) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.data[key]
	if !exists {
		c.stats.Misses++
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check if expired
	if c.isExpired(entry) {
		c.deleteEntry(key)
		c.stats.Misses++
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access info
	entry.lastAccess = time.Now()
	c.updateAccessOrder(key)
	c.stats.Hits++
	c.stats.LastAccess = time.Now()
	c.stats.HitRate = float64(c.stats.Hits) / float64(c.stats.Hits+c.stats.Misses)

	return entry.data, nil
}

// Delete removes data by key.
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteEntry(key)
	return nil
}

// Exists checks if a key exists and is not expired.
func (c *InMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return false, nil
	}

	return !c.isExpired(entry), nil
}

// Keys returns all non-expired keys with optional prefix filter.
func (c *InMemoryCache) Keys(ctx context.Context, prefix string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string
	for key, entry := range c.data {
		if c.isExpired(entry) {
			continue
		}
		if prefix == "" || hasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// Clear removes all data.
func (c *InMemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*cacheEntry)
	c.accessOrder = make([]string, 0)
	c.stats.Size = 0

	return nil
}

// Close closes the cache (no-op for in-memory).
func (c *InMemoryCache) Close() error {
	return nil
}

// GetTTL returns the remaining time-to-live for a key.
func (c *InMemoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return 0, fmt.Errorf("key not found: %s", key)
	}

	if entry.expiresAt == nil {
		return -1, nil // No expiration
	}

	ttl := time.Until(*entry.expiresAt)
	if ttl <= 0 {
		return 0, nil // Expired
	}

	return ttl, nil
}

// Refresh extends the TTL for a key.
func (c *InMemoryCache) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.data[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	if ttl > 0 {
		expiry := time.Now().Add(ttl)
		entry.expiresAt = &expiry
	} else {
		entry.expiresAt = nil // No expiration
	}

	return nil
}

// Stats returns cache statistics.
func (c *InMemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Calculate memory usage estimate
	memoryUsage := int64(len(c.data) * 100) // Rough estimate

	stats := c.stats
	stats.MemoryUsage = memoryUsage

	return stats
}

// SetMaxSize sets the maximum cache size.
func (c *InMemoryCache) SetMaxSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxSize = size
	c.stats.MaxSize = size

	// Evict entries if current size exceeds new max
	for len(c.data) > c.maxSize {
		c.evictLRU()
	}
}

// Evict removes expired entries.
func (c *InMemoryCache) Evict(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiredKeys []string

	// Find expired keys
	for key, entry := range c.data {
		if c.isExpired(entry) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Remove expired keys
	for _, key := range expiredKeys {
		c.deleteEntry(key)
		c.stats.Evictions++
	}

	if len(expiredKeys) > 0 {
		c.stats.LastEviction = now
	}

	return nil
}

// Helper methods

// isExpired checks if an entry has expired.
func (c *InMemoryCache) isExpired(entry *cacheEntry) bool {
	if entry.expiresAt == nil {
		return false
	}
	return time.Now().After(*entry.expiresAt)
}

// deleteEntry removes an entry and updates access order.
func (c *InMemoryCache) deleteEntry(key string) {
	if _, exists := c.data[key]; exists {
		delete(c.data, key)
		c.removeFromAccessOrder(key)
		c.stats.Size = len(c.data)
	}
}

// updateAccessOrder moves a key to the end (most recently used).
func (c *InMemoryCache) updateAccessOrder(key string) {
	// Remove from current position
	c.removeFromAccessOrder(key)
	// Add to end
	c.accessOrder = append(c.accessOrder, key)
}

// removeFromAccessOrder removes a key from the access order list.
func (c *InMemoryCache) removeFromAccessOrder(key string) {
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
}

// evictLRU removes the least recently used entry.
func (c *InMemoryCache) evictLRU() {
	if len(c.accessOrder) == 0 {
		return
	}

	// Remove oldest entry
	oldestKey := c.accessOrder[0]
	c.deleteEntry(oldestKey)
	c.stats.Evictions++
	c.stats.LastEviction = time.Now()
}

// hasPrefix checks if a string has a prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// AutoEvictingCache wraps a cache with automatic background eviction.
type AutoEvictingCache struct {
	cache    CacheMemory
	interval time.Duration
	stopChan chan struct{}
	stopped  bool
	mu       sync.Mutex
}

// NewAutoEvictingCache creates a cache with automatic eviction.
func NewAutoEvictingCache(cache CacheMemory, evictionInterval time.Duration) *AutoEvictingCache {
	aec := &AutoEvictingCache{
		cache:    cache,
		interval: evictionInterval,
		stopChan: make(chan struct{}),
	}

	// Start background eviction
	go aec.backgroundEviction()

	return aec
}

// backgroundEviction runs periodic eviction in the background.
func (aec *AutoEvictingCache) backgroundEviction() {
	ticker := time.NewTicker(aec.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			aec.cache.Evict(ctx)
			cancel()
		case <-aec.stopChan:
			return
		}
	}
}

// Stop stops the background eviction.
func (aec *AutoEvictingCache) Stop() {
	aec.mu.Lock()
	defer aec.mu.Unlock()

	if !aec.stopped {
		close(aec.stopChan)
		aec.stopped = true
	}
}

// Implement CacheMemory interface by delegating to wrapped cache
func (aec *AutoEvictingCache) Store(ctx context.Context, key string, data interface{}) error {
	return aec.cache.Store(ctx, key, data)
}

func (aec *AutoEvictingCache) StoreWithTTL(ctx context.Context, key string, data interface{}, ttl time.Duration) error {
	return aec.cache.StoreWithTTL(ctx, key, data, ttl)
}

func (aec *AutoEvictingCache) Retrieve(ctx context.Context, key string) (interface{}, error) {
	return aec.cache.Retrieve(ctx, key)
}

func (aec *AutoEvictingCache) Delete(ctx context.Context, key string) error {
	return aec.cache.Delete(ctx, key)
}

func (aec *AutoEvictingCache) Exists(ctx context.Context, key string) (bool, error) {
	return aec.cache.Exists(ctx, key)
}

func (aec *AutoEvictingCache) Keys(ctx context.Context, prefix string) ([]string, error) {
	return aec.cache.Keys(ctx, prefix)
}

func (aec *AutoEvictingCache) Clear(ctx context.Context) error {
	return aec.cache.Clear(ctx)
}

func (aec *AutoEvictingCache) Close() error {
	aec.Stop()
	return aec.cache.Close()
}

func (aec *AutoEvictingCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	return aec.cache.GetTTL(ctx, key)
}

func (aec *AutoEvictingCache) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	return aec.cache.Refresh(ctx, key, ttl)
}

func (aec *AutoEvictingCache) Stats() CacheStats {
	return aec.cache.Stats()
}

func (aec *AutoEvictingCache) SetMaxSize(size int) {
	aec.cache.SetMaxSize(size)
}

func (aec *AutoEvictingCache) Evict(ctx context.Context) error {
	return aec.cache.Evict(ctx)
}
