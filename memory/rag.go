package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RAGSystem implements Retrieval-Augmented Generation for enhancing LLM responses with relevant context.
type RAGSystem struct {
	vectorMemory      VectorMemory
	embeddingProvider EmbeddingProvider
	config            RAGConfig
}

// RAGConfig holds configuration for the RAG system.
type RAGConfig struct {
	MaxContextLength     int     `json:"max_context_length"`     // Maximum context length in characters
	SimilarityThreshold  float32 `json:"similarity_threshold"`   // Minimum similarity score
	MaxRetrievedChunks   int     `json:"max_retrieved_chunks"`   // Maximum number of chunks to retrieve
	ChunkOverlap         int     `json:"chunk_overlap"`          // Overlap between chunks in characters
	ChunkSize            int     `json:"chunk_size"`             // Size of each chunk in characters
	EnableReranking      bool    `json:"enable_reranking"`       // Enable result reranking
	RerankingModel       string  `json:"reranking_model"`        // Model to use for reranking
	ContextTemplate      string  `json:"context_template"`       // Template for formatting context
	EnableMetadataFilter bool    `json:"enable_metadata_filter"` // Enable metadata-based filtering
	TimestampWeighting   float32 `json:"timestamp_weighting"`    // Weight factor for recent content
	DiversityThreshold   float32 `json:"diversity_threshold"`    // Minimum diversity between retrieved chunks
}

// RAGResult represents a single result from RAG retrieval.
type RAGResult struct {
	Content    string                 `json:"content"`
	Source     string                 `json:"source"`
	Score      float32                `json:"score"`
	Metadata   map[string]interface{} `json:"metadata"`
	Timestamp  time.Time              `json:"timestamp"`
	ChunkIndex int                    `json:"chunk_index"`
}

// RAGResponse contains the retrieved context and metadata for LLM augmentation.
type RAGResponse struct {
	Context        string        `json:"context"`
	Results        []RAGResult   `json:"results"`
	Query          string        `json:"query"`
	TotalChunks    int           `json:"total_chunks"`
	ProcessingTime time.Duration `json:"processing_time"`
}

// NewRAGSystem creates a new RAG system.
func NewRAGSystem(vectorMemory VectorMemory, embeddingProvider EmbeddingProvider, config RAGConfig) *RAGSystem {
	// Set defaults
	if config.MaxContextLength <= 0 {
		config.MaxContextLength = 4000
	}
	if config.SimilarityThreshold <= 0 {
		config.SimilarityThreshold = 0.3
	}
	if config.MaxRetrievedChunks <= 0 {
		config.MaxRetrievedChunks = 10
	}
	if config.ChunkSize <= 0 {
		config.ChunkSize = 500
	}
	if config.ChunkOverlap <= 0 {
		config.ChunkOverlap = 50
	}
	if config.ContextTemplate == "" {
		config.ContextTemplate = "Relevant context:\n{{.Context}}"
	}
	if config.TimestampWeighting <= 0 {
		config.TimestampWeighting = 0.1
	}
	if config.DiversityThreshold <= 0 {
		config.DiversityThreshold = 0.8
	}

	return &RAGSystem{
		vectorMemory:      vectorMemory,
		embeddingProvider: embeddingProvider,
		config:            config,
	}
}

// Retrieve performs RAG retrieval for the given query.
func (r *RAGSystem) Retrieve(ctx context.Context, query string) (*RAGResponse, error) {
	startTime := time.Now()

	// Perform similarity search
	similarityResults, err := r.vectorMemory.SimilaritySearch(ctx, query, r.config.MaxRetrievedChunks*2) // Get more than needed for filtering
	if err != nil {
		return nil, fmt.Errorf("similarity search failed: %w", err)
	}

	// Convert to RAG results and apply filtering
	ragResults := r.convertToRAGResults(similarityResults)
	ragResults = r.filterResults(ragResults)
	ragResults = r.applyDiversityFiltering(ragResults)

	// Apply reranking if enabled
	if r.config.EnableReranking {
		ragResults = r.rerankResults(ctx, query, ragResults)
	}

	// Limit to max chunks
	if len(ragResults) > r.config.MaxRetrievedChunks {
		ragResults = ragResults[:r.config.MaxRetrievedChunks]
	}

	// Build context
	context := r.buildContext(ragResults)

	response := &RAGResponse{
		Context:        context,
		Results:        ragResults,
		Query:          query,
		TotalChunks:    len(ragResults),
		ProcessingTime: time.Since(startTime),
	}

	return response, nil
}

// convertToRAGResults converts similarity results to RAG results.
func (r *RAGSystem) convertToRAGResults(similarityResults []SimilarityResult) []RAGResult {
	var ragResults []RAGResult

	for _, result := range similarityResults {
		ragResult := RAGResult{
			Score:    result.Score,
			Metadata: result.Metadata,
		}

		// Extract content and source based on data type
		switch data := result.Data.(type) {
		case KnowledgeEntry:
			ragResult.Content = data.Content
			ragResult.Source = data.Source
			ragResult.Timestamp = data.CreatedAt
			if ragResult.Metadata == nil {
				ragResult.Metadata = make(map[string]interface{})
			}
			ragResult.Metadata["category"] = data.Category
			ragResult.Metadata["title"] = data.Title
			ragResult.Metadata["tags"] = data.Tags

		case Message:
			ragResult.Content = data.Content
			ragResult.Source = fmt.Sprintf("conversation_%s", data.ID)
			ragResult.Timestamp = data.Timestamp
			if ragResult.Metadata == nil {
				ragResult.Metadata = make(map[string]interface{})
			}
			ragResult.Metadata["role"] = data.Role
			ragResult.Metadata["message_id"] = data.ID

		case ConversationMemory:
			ragResult.Content = r.extractConversationText(data)
			ragResult.Source = fmt.Sprintf("conversation_%s_%s", data.AgentID, data.SessionID)
			ragResult.Timestamp = data.UpdatedAt
			if ragResult.Metadata == nil {
				ragResult.Metadata = make(map[string]interface{})
			}
			ragResult.Metadata["agent_id"] = data.AgentID
			ragResult.Metadata["session_id"] = data.SessionID
			ragResult.Metadata["message_count"] = data.MessageCount

		case string:
			ragResult.Content = data
			ragResult.Source = result.Key
			ragResult.Timestamp = time.Now() // Default timestamp

		default:
			ragResult.Content = fmt.Sprintf("%v", data)
			ragResult.Source = result.Key
			ragResult.Timestamp = time.Now()
		}

		ragResults = append(ragResults, ragResult)
	}

	return ragResults
}

// filterResults applies various filters to the results.
func (r *RAGSystem) filterResults(results []RAGResult) []RAGResult {
	var filtered []RAGResult

	for _, result := range results {
		// Apply similarity threshold
		if result.Score < r.config.SimilarityThreshold {
			continue
		}

		// Apply timestamp weighting
		if r.config.TimestampWeighting > 0 {
			age := time.Since(result.Timestamp)
			// Boost recent content
			ageFactor := 1.0 + r.config.TimestampWeighting*float32(1.0/(1.0+age.Hours()/24))
			result.Score *= ageFactor
		}

		// Apply metadata filtering if enabled
		if r.config.EnableMetadataFilter {
			if !r.passesMetadataFilter(result) {
				continue
			}
		}

		filtered = append(filtered, result)
	}

	// Sort by score (highest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	return filtered
}

// passesMetadataFilter checks if a result passes metadata filtering.
func (r *RAGSystem) passesMetadataFilter(result RAGResult) bool {
	// Implement custom metadata filtering logic here
	// For now, return true (no filtering)
	return true
}

// applyDiversityFiltering ensures diversity in retrieved results.
func (r *RAGSystem) applyDiversityFiltering(results []RAGResult) []RAGResult {
	if len(results) <= 1 {
		return results
	}

	var diverseResults []RAGResult
	diverseResults = append(diverseResults, results[0]) // Always include the best result

	for _, candidate := range results[1:] {
		isDiverse := true

		// Check similarity with already selected results
		for _, selected := range diverseResults {
			similarity := r.calculateContentSimilarity(candidate.Content, selected.Content)
			if similarity > r.config.DiversityThreshold {
				isDiverse = false
				break
			}
		}

		if isDiverse {
			diverseResults = append(diverseResults, candidate)
		}
	}

	return diverseResults
}

// calculateContentSimilarity calculates similarity between two text contents.
func (r *RAGSystem) calculateContentSimilarity(content1, content2 string) float32 {
	// Simple similarity based on common words (can be improved with embeddings)
	words1 := strings.Fields(strings.ToLower(content1))
	words2 := strings.Fields(strings.ToLower(content2))

	if len(words1) == 0 || len(words2) == 0 {
		return 0
	}

	// Count common words
	wordMap := make(map[string]bool)
	for _, word := range words1 {
		wordMap[word] = true
	}

	commonWords := 0
	for _, word := range words2 {
		if wordMap[word] {
			commonWords++
		}
	}

	// Calculate Jaccard similarity
	totalWords := len(words1) + len(words2) - commonWords
	if totalWords == 0 {
		return 1.0
	}

	return float32(commonWords) / float32(totalWords)
}

// rerankResults reranks results using a more sophisticated model.
func (r *RAGSystem) rerankResults(ctx context.Context, query string, results []RAGResult) []RAGResult {
	// This is a placeholder for more sophisticated reranking
	// In a full implementation, you might use a cross-encoder model
	// For now, we'll just return the results as-is
	return results
}

// buildContext builds the final context string from RAG results.
func (r *RAGSystem) buildContext(results []RAGResult) string {
	if len(results) == 0 {
		return ""
	}

	var contextParts []string
	totalLength := 0

	for i, result := range results {
		// Format the content
		content := result.Content

		// Add source information if available
		if result.Source != "" {
			content = fmt.Sprintf("[Source: %s] %s", result.Source, content)
		}

		// Check length limits
		if totalLength+len(content) > r.config.MaxContextLength {
			break
		}

		contextParts = append(contextParts, fmt.Sprintf("%d. %s", i+1, content))
		totalLength += len(content)
	}

	return strings.Join(contextParts, "\n\n")
}

// extractConversationText extracts meaningful text from conversation memory.
func (r *RAGSystem) extractConversationText(conv ConversationMemory) string {
	var parts []string

	// Add summary if available
	if conv.Summary != "" {
		parts = append(parts, fmt.Sprintf("Summary: %s", conv.Summary))
	}

	// Add recent messages (limit to avoid too much context)
	maxMessages := 5
	messageStart := 0
	if len(conv.Messages) > maxMessages {
		messageStart = len(conv.Messages) - maxMessages
	}

	for _, msg := range conv.Messages[messageStart:] {
		if msg.Content != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
		}
	}

	return strings.Join(parts, "\n")
}

// AddDocument adds a document to the RAG system by chunking and storing it.
func (r *RAGSystem) AddDocument(ctx context.Context, document Document) error {
	chunks := r.chunkDocument(document)

	for i, chunk := range chunks {
		chunkKey := fmt.Sprintf("%s_chunk_%d", document.ID, i)

		chunkData := KnowledgeEntry{
			ID:        chunkKey,
			Title:     document.Title,
			Content:   chunk,
			Category:  document.Category,
			Tags:      document.Tags,
			Source:    document.Source,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"document_id":  document.ID,
				"chunk_index":  i,
				"total_chunks": len(chunks),
			},
		}

		if err := r.vectorMemory.StoreWithEmbedding(ctx, chunkKey, chunkData); err != nil {
			return fmt.Errorf("failed to store chunk %d: %w", i, err)
		}
	}

	return nil
}

// chunkDocument splits a document into smaller chunks for processing.
func (r *RAGSystem) chunkDocument(document Document) []string {
	content := document.Content
	if len(content) <= r.config.ChunkSize {
		return []string{content}
	}

	var chunks []string
	start := 0

	for start < len(content) {
		end := start + r.config.ChunkSize
		if end > len(content) {
			end = len(content)
		}

		// Try to break at a sentence or word boundary
		if end < len(content) {
			// Look for sentence boundary
			for i := end; i > start+r.config.ChunkSize/2; i-- {
				if content[i] == '.' || content[i] == '!' || content[i] == '?' {
					end = i + 1
					break
				}
			}

			// If no sentence boundary found, look for word boundary
			if end == start+r.config.ChunkSize {
				for i := end; i > start+r.config.ChunkSize/2; i-- {
					if content[i] == ' ' || content[i] == '\n' {
						end = i
						break
					}
				}
			}
		}

		chunk := content[start:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		// Move start position with overlap
		start = end - r.config.ChunkOverlap
		if start < 0 {
			start = 0
		}
	}

	return chunks
}

// Document represents a document to be added to the RAG system.
type Document struct {
	ID       string                 `json:"id"`
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`
	Category string                 `json:"category"`
	Tags     []string               `json:"tags"`
	Source   string                 `json:"source"`
	Metadata map[string]interface{} `json:"metadata"`
}

// RAGStats provides statistics about the RAG system.
type RAGStats struct {
	TotalDocuments   int           `json:"total_documents"`
	TotalChunks      int           `json:"total_chunks"`
	AverageChunkSize int           `json:"average_chunk_size"`
	LastUpdated      time.Time     `json:"last_updated"`
	MemoryUsage      int64         `json:"memory_usage"`
	QueryLatency     time.Duration `json:"query_latency"`
}

// GetStats returns statistics about the RAG system.
func (r *RAGSystem) GetStats(ctx context.Context) (*RAGStats, error) {
	// Get all keys to calculate statistics
	keys, err := r.vectorMemory.Keys(ctx, "")
	if err != nil {
		return nil, err
	}

	documentCount := make(map[string]bool)
	totalChunks := 0
	totalContent := 0

	for _, key := range keys {
		if strings.Contains(key, "_chunk_") {
			totalChunks++

			// Extract document ID
			parts := strings.Split(key, "_chunk_")
			if len(parts) > 0 {
				documentCount[parts[0]] = true
			}

			// Get content to calculate average size
			data, err := r.vectorMemory.Retrieve(ctx, key)
			if err == nil {
				if entry, ok := data.(KnowledgeEntry); ok {
					totalContent += len(entry.Content)
				}
			}
		}
	}

	averageChunkSize := 0
	if totalChunks > 0 {
		averageChunkSize = totalContent / totalChunks
	}

	return &RAGStats{
		TotalDocuments:   len(documentCount),
		TotalChunks:      totalChunks,
		AverageChunkSize: averageChunkSize,
		LastUpdated:      time.Now(),
	}, nil
}

// DefaultRAGConfig returns a default RAG configuration.
func DefaultRAGConfig() RAGConfig {
	return RAGConfig{
		MaxContextLength:     4000,
		SimilarityThreshold:  0.3,
		MaxRetrievedChunks:   10,
		ChunkOverlap:         50,
		ChunkSize:            500,
		EnableReranking:      false,
		ContextTemplate:      "Relevant context:\n{{.Context}}",
		EnableMetadataFilter: false,
		TimestampWeighting:   0.1,
		DiversityThreshold:   0.8,
	}
}
