package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryVectorDB implements VectorMemory using in-memory storage with vector operations.
type InMemoryVectorDB struct {
	data              map[string]*VectorEntry
	embeddingProvider EmbeddingProvider
	dimensions        int
	mu                sync.RWMutex
}

// VectorEntry represents a stored vector entry.
type VectorEntry struct {
	Key       string                 `json:"key"`
	Data      interface{}            `json:"data"`
	Embedding []float32              `json:"embedding"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt int64                  `json:"created_at"`
	UpdatedAt int64                  `json:"updated_at"`
}

// NewInMemoryVectorDB creates a new in-memory vector database.
func NewInMemoryVectorDB(dimensions int, embeddingProvider EmbeddingProvider) VectorMemory {
	return &InMemoryVectorDB{
		data:              make(map[string]*VectorEntry),
		embeddingProvider: embeddingProvider,
		dimensions:        dimensions,
	}
}

// Store saves data with a key (generates embedding automatically if provider is available).
func (v *InMemoryVectorDB) Store(ctx context.Context, key string, data interface{}) error {
	if v.embeddingProvider != nil && v.embeddingProvider.IsConfigured() {
		return v.StoreWithEmbedding(ctx, key, data)
	}

	// Store without embedding
	return v.StoreVector(ctx, key, data, nil)
}

// StoreVector stores data with its vector embedding.
func (v *InMemoryVectorDB) StoreVector(ctx context.Context, key string, data interface{}, embedding []float32) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Validate embedding dimensions if provided
	if embedding != nil && len(embedding) != v.dimensions {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d", v.dimensions, len(embedding))
	}

	now := getCurrentTimestamp()
	entry := &VectorEntry{
		Key:       key,
		Data:      data,
		Embedding: embedding,
		Metadata:  make(map[string]interface{}),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Preserve creation time if updating existing entry
	if existing, exists := v.data[key]; exists {
		entry.CreatedAt = existing.CreatedAt
		entry.Metadata = existing.Metadata // Preserve metadata
	}

	v.data[key] = entry
	return nil
}

// StoreWithEmbedding stores data and generates embedding automatically.
func (v *InMemoryVectorDB) StoreWithEmbedding(ctx context.Context, key string, data interface{}) error {
	if v.embeddingProvider == nil || !v.embeddingProvider.IsConfigured() {
		return fmt.Errorf("no embedding provider configured")
	}

	// Convert data to text for embedding
	text := convertToText(data)
	if text == "" {
		return fmt.Errorf("cannot convert data to text for embedding")
	}

	// Generate embedding
	embedding, err := v.embeddingProvider.GenerateEmbedding(ctx, text)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	return v.StoreVector(ctx, key, data, embedding)
}

// Retrieve gets data by key.
func (v *InMemoryVectorDB) Retrieve(ctx context.Context, key string) (interface{}, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entry, exists := v.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	return entry.Data, nil
}

// Delete removes data by key.
func (v *InMemoryVectorDB) Delete(ctx context.Context, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.data, key)
	return nil
}

// Exists checks if a key exists.
func (v *InMemoryVectorDB) Exists(ctx context.Context, key string) (bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	_, exists := v.data[key]
	return exists, nil
}

// Keys returns all keys with optional prefix filter.
func (v *InMemoryVectorDB) Keys(ctx context.Context, prefix string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var keys []string
	for key := range v.data {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// Clear removes all data.
func (v *InMemoryVectorDB) Clear(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.data = make(map[string]*VectorEntry)
	return nil
}

// Close closes the vector database.
func (v *InMemoryVectorDB) Close() error {
	return nil // No resources to close for in-memory implementation
}

// SimilaritySearch finds similar items based on vector similarity.
func (v *InMemoryVectorDB) SimilaritySearch(ctx context.Context, query string, limit int) ([]SimilarityResult, error) {
	if v.embeddingProvider == nil || !v.embeddingProvider.IsConfigured() {
		return nil, fmt.Errorf("no embedding provider configured for similarity search")
	}

	// Generate embedding for query
	queryEmbedding, err := v.embeddingProvider.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	return v.SimilaritySearchByVector(ctx, queryEmbedding, limit)
}

// SimilaritySearchByVector finds similar items using a vector.
func (v *InMemoryVectorDB) SimilaritySearchByVector(ctx context.Context, queryEmbedding []float32, limit int) ([]SimilarityResult, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(queryEmbedding) != v.dimensions {
		return nil, fmt.Errorf("query embedding dimension mismatch: expected %d, got %d", v.dimensions, len(queryEmbedding))
	}

	var results []SimilarityResult

	// Calculate similarity with all entries that have embeddings
	for key, entry := range v.data {
		if entry.Embedding == nil {
			continue // Skip entries without embeddings
		}

		similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
		distance := 1.0 - similarity // Convert similarity to distance

		results = append(results, SimilarityResult{
			Key:       key,
			Data:      entry.Data,
			Score:     similarity,
			Distance:  distance,
			Embedding: entry.Embedding,
			Metadata:  entry.Metadata,
		})
	}

	// Sort by similarity score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetEmbedding returns the embedding for a key.
func (v *InMemoryVectorDB) GetEmbedding(ctx context.Context, key string) ([]float32, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entry, exists := v.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	if entry.Embedding == nil {
		return nil, fmt.Errorf("no embedding found for key: %s", key)
	}

	// Return a copy to prevent external modification
	embedding := make([]float32, len(entry.Embedding))
	copy(embedding, entry.Embedding)

	return embedding, nil
}

// SetEmbeddingProvider sets the embedding generation provider.
func (v *InMemoryVectorDB) SetEmbeddingProvider(provider EmbeddingProvider) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.embeddingProvider = provider
}

// GetDimensions returns the vector dimensions.
func (v *InMemoryVectorDB) GetDimensions() int {
	return v.dimensions
}

// Vector similarity functions

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// euclideanDistance calculates the Euclidean distance between two vectors.
func euclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return float32(math.Inf(1))
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		diff := float64(a[i] - b[i])
		sum += diff * diff
	}

	return float32(math.Sqrt(sum))
}

// dotProduct calculates the dot product of two vectors.
func dotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var product float64
	for i := 0; i < len(a); i++ {
		product += float64(a[i]) * float64(b[i])
	}

	return float32(product)
}

// Helper functions

// convertToText converts various data types to text for embedding generation.
func convertToText(data interface{}) string {
	switch v := data.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	case ConversationMemory:
		return extractConversationText(v)
	case KnowledgeEntry:
		return fmt.Sprintf("%s\n%s", v.Title, v.Content)
	case MemoryEntry:
		return convertToText(v.Data)
	default:
		return fmt.Sprintf("%v", data)
	}
}

// extractConversationText extracts text from conversation for embedding.
func extractConversationText(conv ConversationMemory) string {
	var texts []string

	// Add summary if available
	if conv.Summary != "" {
		texts = append(texts, conv.Summary)
	}

	// Add recent messages
	for _, msg := range conv.Messages {
		if msg.Content != "" {
			texts = append(texts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
		}
	}

	return strings.Join(texts, "\n")
}

// getCurrentTimestamp returns current Unix timestamp in milliseconds.
func getCurrentTimestamp() int64 {
	return time.Now().UnixNano() / 1000000
}

// VectorSearchOptions provides advanced search options.
type VectorSearchOptions struct {
	Limit            int                `json:"limit,omitempty"`
	MinScore         float32            `json:"min_score,omitempty"`
	MaxDistance      float32            `json:"max_distance,omitempty"`
	Filter           MemorySearchFilter `json:"filter,omitempty"`
	IncludeEmbedding bool               `json:"include_embedding,omitempty"`
	SimilarityType   string             `json:"similarity_type,omitempty"` // "cosine", "euclidean", "dot"
}

// AdvancedSimilaritySearch performs similarity search with advanced options.
func (v *InMemoryVectorDB) AdvancedSimilaritySearch(ctx context.Context, queryEmbedding []float32, options VectorSearchOptions) ([]SimilarityResult, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(queryEmbedding) != v.dimensions {
		return nil, fmt.Errorf("query embedding dimension mismatch: expected %d, got %d", v.dimensions, len(queryEmbedding))
	}

	var results []SimilarityResult

	// Calculate similarity with all entries that have embeddings
	for key, entry := range v.data {
		if entry.Embedding == nil {
			continue
		}

		// Apply filters
		if !v.matchesFilter(entry, options.Filter) {
			continue
		}

		// Calculate similarity based on type
		var score float32
		var distance float32

		switch options.SimilarityType {
		case "euclidean":
			distance = euclideanDistance(queryEmbedding, entry.Embedding)
			score = 1.0 / (1.0 + distance) // Convert distance to similarity
		case "dot":
			score = dotProduct(queryEmbedding, entry.Embedding)
			distance = 1.0 - score
		default: // "cosine" or empty
			score = cosineSimilarity(queryEmbedding, entry.Embedding)
			distance = 1.0 - score
		}

		// Apply score/distance filters
		if options.MinScore > 0 && score < options.MinScore {
			continue
		}
		if options.MaxDistance > 0 && distance > options.MaxDistance {
			continue
		}

		result := SimilarityResult{
			Key:      key,
			Data:     entry.Data,
			Score:    score,
			Distance: distance,
			Metadata: entry.Metadata,
		}

		// Include embedding if requested
		if options.IncludeEmbedding {
			result.Embedding = make([]float32, len(entry.Embedding))
			copy(result.Embedding, entry.Embedding)
		}

		results = append(results, result)
	}

	// Sort by similarity score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit results
	limit := options.Limit
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// matchesFilter checks if an entry matches the search filter.
func (v *InMemoryVectorDB) matchesFilter(entry *VectorEntry, filter MemorySearchFilter) bool {
	// Key prefix filter
	if filter.KeyPrefix != "" && !strings.HasPrefix(entry.Key, filter.KeyPrefix) {
		return false
	}

	// Metadata filters would be implemented here
	// For now, we'll just return true

	return true
}

// BatchEmbedding provides efficient batch embedding operations.
type BatchEmbedding struct {
	vectorDB  VectorMemory
	provider  EmbeddingProvider
	batchSize int
}

// NewBatchEmbedding creates a new batch embedding processor.
func NewBatchEmbedding(vectorDB VectorMemory, provider EmbeddingProvider, batchSize int) *BatchEmbedding {
	return &BatchEmbedding{
		vectorDB:  vectorDB,
		provider:  provider,
		batchSize: batchSize,
	}
}

// ProcessBatch processes a batch of items for embedding and storage.
func (be *BatchEmbedding) ProcessBatch(ctx context.Context, items map[string]interface{}) error {
	if be.provider == nil || !be.provider.IsConfigured() {
		return fmt.Errorf("embedding provider not configured")
	}

	// Convert items to text
	var keys []string
	var texts []string

	for key, data := range items {
		text := convertToText(data)
		if text == "" {
			continue // Skip items that can't be converted to text
		}
		keys = append(keys, key)
		texts = append(texts, text)
	}

	if len(texts) == 0 {
		return fmt.Errorf("no valid text items to process")
	}

	// Generate embeddings in batches
	for i := 0; i < len(texts); i += be.batchSize {
		end := i + be.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batchTexts := texts[i:end]
		batchKeys := keys[i:end]

		// Generate batch embeddings
		embeddings, err := be.provider.GenerateBatchEmbeddings(ctx, batchTexts)
		if err != nil {
			return fmt.Errorf("failed to generate batch embeddings: %w", err)
		}

		// Store embeddings
		for j, embedding := range embeddings {
			key := batchKeys[j]
			data := items[key]

			if err := be.vectorDB.StoreVector(ctx, key, data, embedding); err != nil {
				return fmt.Errorf("failed to store vector for key %s: %w", key, err)
			}
		}
	}

	return nil
}
