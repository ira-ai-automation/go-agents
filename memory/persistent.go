package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileMemory implements PersistentMemory using file-based storage.
type FileMemory struct {
	filePath string
	data     map[string]*MemoryEntry
	mu       sync.RWMutex
	dirty    bool // Track if data needs to be saved
}

// NewFileMemory creates a new file-based memory store.
func NewFileMemory(filePath string) PersistentMemory {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// Log error but continue - will fail on actual operations if needed
		fmt.Printf("Warning: Could not create directory %s: %v\n", dir, err)
	}

	return &FileMemory{
		filePath: filePath,
		data:     make(map[string]*MemoryEntry),
	}
}

// Store saves data with a key.
func (f *FileMemory) Store(ctx context.Context, key string, data interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	entry := &MemoryEntry{
		Key:        key,
		Data:       data,
		CreatedAt:  now,
		UpdatedAt:  now,
		AccessedAt: now,
	}

	// Update existing entry's creation time if it exists
	if existing, exists := f.data[key]; exists {
		entry.CreatedAt = existing.CreatedAt
	}

	f.data[key] = entry
	f.dirty = true

	return nil
}

// Retrieve gets data by key.
func (f *FileMemory) Retrieve(ctx context.Context, key string) (interface{}, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	entry, exists := f.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Update access time
	entry.AccessedAt = time.Now()

	return entry.Data, nil
}

// Delete removes data by key.
func (f *FileMemory) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.data[key]; exists {
		delete(f.data, key)
		f.dirty = true
	}

	return nil
}

// Exists checks if a key exists.
func (f *FileMemory) Exists(ctx context.Context, key string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, exists := f.data[key]
	return exists, nil
}

// Keys returns all keys with optional prefix filter.
func (f *FileMemory) Keys(ctx context.Context, prefix string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var keys []string
	for key := range f.data {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// Clear removes all data.
func (f *FileMemory) Clear(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.data = make(map[string]*MemoryEntry)
	f.dirty = true

	return nil
}

// Close closes the memory store and persists data.
func (f *FileMemory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.dirty {
		return f.persistLocked()
	}

	return nil
}

// Persist forces a save to persistent storage.
func (f *FileMemory) Persist(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.persistLocked()
}

// persistLocked saves data to file (must be called with lock held).
func (f *FileMemory) persistLocked() error {
	// Create temporary file for atomic write
	tempFile := f.filePath + ".tmp"

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	// Encode data to JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(f.data); err != nil {
		os.Remove(tempFile) // Clean up temp file
		return fmt.Errorf("failed to encode data: %w", err)
	}

	// Sync to disk
	if err := file.Sync(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	file.Close()

	// Atomic rename
	if err := os.Rename(tempFile, f.filePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	f.dirty = false
	return nil
}

// Load restores data from persistent storage.
func (f *FileMemory) Load(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(f.filePath); os.IsNotExist(err) {
		// File doesn't exist - start with empty data
		f.data = make(map[string]*MemoryEntry)
		return nil
	}

	file, err := os.Open(f.filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Decode JSON data
	decoder := json.NewDecoder(file)

	var data map[string]*MemoryEntry
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	f.data = data
	f.dirty = false

	return nil
}

// GetStoragePath returns the storage file path.
func (f *FileMemory) GetStoragePath() string {
	return f.filePath
}

// Backup creates a backup of the memory store.
func (f *FileMemory) Backup(ctx context.Context, backupPath string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Ensure backup directory exists
	dir := filepath.Dir(backupPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Open source file
	src, err := os.Open(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Source doesn't exist - create empty backup
			return f.createEmptyBackup(backupPath)
		}
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Create backup file
	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	// Copy data
	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Sync to disk
	return dst.Sync()
}

// createEmptyBackup creates an empty backup file.
func (f *FileMemory) createEmptyBackup(backupPath string) error {
	file, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create empty backup: %w", err)
	}
	defer file.Close()

	// Write empty JSON object
	encoder := json.NewEncoder(file)
	return encoder.Encode(make(map[string]*MemoryEntry))
}

// Restore restores from a backup.
func (f *FileMemory) Restore(ctx context.Context, backupPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Open backup file
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	// Decode backup data
	decoder := json.NewDecoder(file)

	var data map[string]*MemoryEntry
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode backup data: %w", err)
	}

	// Replace current data
	f.data = data
	f.dirty = true

	// Persist the restored data
	return f.persistLocked()
}

// SQLiteMemory implements PersistentMemory using SQLite database.
// This is a basic implementation - in production you'd want proper schema migration, etc.
type SQLiteMemory struct {
	connectionString string
	mu               sync.RWMutex
	// Note: In a real implementation, you'd embed a *sql.DB here
}

// NewSQLiteMemory creates a new SQLite-based memory store.
func NewSQLiteMemory(connectionString string) PersistentMemory {
	return &SQLiteMemory{
		connectionString: connectionString,
	}
}

// Note: The SQLite implementation is stubbed out as it would require
// importing database/sql and a SQLite driver. In a real implementation,
// you would:
// 1. Import "database/sql" and _ "github.com/mattn/go-sqlite3"
// 2. Create tables with proper schema
// 3. Implement CRUD operations with SQL queries
// 4. Handle transactions and proper error handling

// Store saves data with a key.
func (s *SQLiteMemory) Store(ctx context.Context, key string, data interface{}) error {
	return fmt.Errorf("SQLite memory not fully implemented - use FileMemory for now")
}

// Retrieve gets data by key.
func (s *SQLiteMemory) Retrieve(ctx context.Context, key string) (interface{}, error) {
	return nil, fmt.Errorf("SQLite memory not fully implemented")
}

// Delete removes data by key.
func (s *SQLiteMemory) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// Exists checks if a key exists.
func (s *SQLiteMemory) Exists(ctx context.Context, key string) (bool, error) {
	return false, fmt.Errorf("SQLite memory not fully implemented")
}

// Keys returns all keys with optional prefix filter.
func (s *SQLiteMemory) Keys(ctx context.Context, prefix string) ([]string, error) {
	return nil, fmt.Errorf("SQLite memory not fully implemented")
}

// Clear removes all data.
func (s *SQLiteMemory) Clear(ctx context.Context) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// Close closes the memory store.
func (s *SQLiteMemory) Close() error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// Persist forces a save to persistent storage.
func (s *SQLiteMemory) Persist(ctx context.Context) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// Load restores data from persistent storage.
func (s *SQLiteMemory) Load(ctx context.Context) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// GetStoragePath returns the storage path/connection string.
func (s *SQLiteMemory) GetStoragePath() string {
	return s.connectionString
}

// Backup creates a backup of the memory store.
func (s *SQLiteMemory) Backup(ctx context.Context, path string) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// Restore restores from a backup.
func (s *SQLiteMemory) Restore(ctx context.Context, path string) error {
	return fmt.Errorf("SQLite memory not fully implemented")
}

// BatchWriter provides efficient batch writing for persistent memory.
type BatchWriter struct {
	memory     PersistentMemory
	batchSize  int
	batch      map[string]interface{}
	mu         sync.Mutex
	autoCommit bool
	commitChan chan struct{}
	stopped    bool
}

// NewBatchWriter creates a new batch writer.
func NewBatchWriter(memory PersistentMemory, batchSize int, autoCommitInterval time.Duration) *BatchWriter {
	bw := &BatchWriter{
		memory:     memory,
		batchSize:  batchSize,
		batch:      make(map[string]interface{}),
		commitChan: make(chan struct{}, 1),
	}

	// Start auto-commit if interval is specified
	if autoCommitInterval > 0 {
		bw.autoCommit = true
		go bw.autoCommitLoop(autoCommitInterval)
	}

	return bw
}

// Add adds an item to the batch.
func (bw *BatchWriter) Add(key string, data interface{}) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.stopped {
		return fmt.Errorf("batch writer is stopped")
	}

	bw.batch[key] = data

	// Auto-commit if batch is full
	if len(bw.batch) >= bw.batchSize {
		return bw.commitLocked()
	}

	return nil
}

// Commit flushes the current batch to persistent storage.
func (bw *BatchWriter) Commit() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	return bw.commitLocked()
}

// commitLocked commits the batch (must be called with lock held).
func (bw *BatchWriter) commitLocked() error {
	if len(bw.batch) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Write all items in batch
	for key, data := range bw.batch {
		if err := bw.memory.Store(ctx, key, data); err != nil {
			return fmt.Errorf("failed to store %s: %w", key, err)
		}
	}

	// Persist to storage
	if err := bw.memory.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist batch: %w", err)
	}

	// Clear batch
	bw.batch = make(map[string]interface{})

	return nil
}

// Stop stops the batch writer and commits any pending items.
func (bw *BatchWriter) Stop() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.stopped {
		return nil
	}

	bw.stopped = true

	// Signal auto-commit to stop
	if bw.autoCommit {
		select {
		case bw.commitChan <- struct{}{}:
		default:
		}
	}

	// Commit any remaining items
	return bw.commitLocked()
}

// autoCommitLoop runs periodic commits in the background.
func (bw *BatchWriter) autoCommitLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bw.Commit() // Ignore errors in auto-commit
		case <-bw.commitChan:
			return // Stop signal received
		}
	}
}

// Size returns the current batch size.
func (bw *BatchWriter) Size() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	return len(bw.batch)
}
