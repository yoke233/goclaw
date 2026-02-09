package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVectorOperations(t *testing.T) {
	t.Run("CosineSimilarity", func(t *testing.T) {
		a := []float32{1.0, 2.0, 3.0}
		b := []float32{1.0, 2.0, 3.0}

		sim, err := CosineSimilarity(a, b)
		if err != nil {
			t.Fatalf("CosineSimilarity failed: %v", err)
		}

		if sim < 0.999 || sim > 1.001 {
			t.Errorf("Expected similarity ~1.0, got %f", sim)
		}
	})

	t.Run("EuclideanDistance", func(t *testing.T) {
		a := []float32{0.0, 0.0}
		b := []float32{3.0, 4.0}

		dist, err := EuclideanDistance(a, b)
		if err != nil {
			t.Fatalf("EuclideanDistance failed: %v", err)
		}

		expected := 5.0
		if dist < expected-0.001 || dist > expected+0.001 {
			t.Errorf("Expected distance %f, got %f", expected, dist)
		}
	})

	t.Run("Normalize", func(t *testing.T) {
		vec := []float32{3.0, 4.0}

		normalized, err := Normalize(vec)
		if err != nil {
			t.Fatalf("Normalize failed: %v", err)
		}

		mag, _ := Magnitude(normalized)
		if mag < 0.999 || mag > 1.001 {
			t.Errorf("Normalized vector magnitude should be 1.0, got %f", mag)
		}
	})
}

func TestMemoryStore(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "memory-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create mock provider
	provider := &mockEmbeddingProvider{
		dimension: 128,
	}

	// Create store
	config := DefaultStoreConfig(dbPath, provider)
	config.EnableVectorSearch = false // Disable for testing without extension
	config.EnableFTS = false

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("AddAndGet", func(t *testing.T) {
		ve := &VectorEmbedding{
			Vector:    make([]float32, 128),
			Dimension: 128,
			Text:      "test memory",
			Source:    MemorySourceLongTerm,
			Type:      MemoryTypeFact,
			Metadata: MemoryMetadata{
				Tags:       []string{"test", "unit"},
				Importance: 0.8,
			},
		}

		err := store.Add(ve)
		if err != nil {
			t.Fatalf("Failed to add memory: %v", err)
		}

		retrieved, err := store.Get(ve.ID)
		if err != nil {
			t.Fatalf("Failed to get memory: %v", err)
		}

		if retrieved.Text != ve.Text {
			t.Errorf("Expected text %s, got %s", ve.Text, retrieved.Text)
		}

		if len(retrieved.Metadata.Tags) != 2 {
			t.Errorf("Expected 2 tags, got %d", len(retrieved.Metadata.Tags))
		}
	})

	t.Run("AddBatch", func(t *testing.T) {
		embeddings := make([]*VectorEmbedding, 5)
		for i := 0; i < 5; i++ {
			embeddings[i] = &VectorEmbedding{
				Vector:    make([]float32, 128),
				Dimension: 128,
				Text:      "test memory",
				Source:    MemorySourceDaily,
				Type:      MemoryTypeContext,
			}
		}

		err := store.AddBatch(embeddings)
		if err != nil {
			t.Fatalf("Failed to add batch: %v", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		all, err := store.List(nil)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}

		if len(all) < 6 {
			t.Errorf("Expected at least 6 memories, got %d", len(all))
		}

		// Test filtering
		longTerm, err := store.List(func(ve *VectorEmbedding) bool {
			return ve.Source == MemorySourceLongTerm
		})
		if err != nil {
			t.Fatalf("Failed to filter list: %v", err)
		}

		if len(longTerm) != 1 {
			t.Errorf("Expected 1 long-term memory, got %d", len(longTerm))
		}
	})

	t.Run("Update", func(t *testing.T) {
		all, err := store.List(nil)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}

		if len(all) == 0 {
			t.Skip("No memories to update")
		}

		ve := all[0]
		originalText := ve.Text
		ve.Text = "updated text"

		err = store.Update(ve)
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		retrieved, err := store.Get(ve.ID)
		if err != nil {
			t.Fatalf("Failed to get updated: %v", err)
		}

		if retrieved.Text == originalText {
			t.Error("Memory was not updated")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		all, err := store.List(nil)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}

		if len(all) == 0 {
			t.Skip("No memories to delete")
		}

		ve := all[0]
		err = store.Delete(ve.ID)
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		_, err = store.Get(ve.ID)
		if err == nil {
			t.Error("Expected error when getting deleted memory")
		}
	})
}

func TestMemoryManager(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "manager-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create mock provider
	provider := &mockEmbeddingProvider{
		dimension: 128,
	}

	// Create store
	config := DefaultStoreConfig(dbPath, provider)
	config.EnableVectorSearch = false
	config.EnableFTS = false

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create manager
	manager, err := NewMemoryManager(DefaultManagerConfig(store, provider))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	t.Run("AddMemory", func(t *testing.T) {
		ve, err := manager.AddMemory(ctx, "test memory", MemorySourceLongTerm, MemoryTypeFact, MemoryMetadata{
			Tags: []string{"test"},
		})
		if err != nil {
			t.Fatalf("Failed to add memory: %v", err)
		}

		if ve.Text != "test memory" {
			t.Errorf("Expected text 'test memory', got %s", ve.Text)
		}
	})

	t.Run("AddMemoryBatch", func(t *testing.T) {
		items := []MemoryItem{
			{Text: "memory 1", Source: MemorySourceDaily, Type: MemoryTypeContext},
			{Text: "memory 2", Source: MemorySourceDaily, Type: MemoryTypeContext},
			{Text: "memory 3", Source: MemorySourceDaily, Type: MemoryTypeContext},
		}

		err := manager.AddMemoryBatch(ctx, items)
		if err != nil {
			t.Fatalf("Failed to add batch: %v", err)
		}
	})

	t.Run("GetStats", func(t *testing.T) {
		stats, err := manager.GetStats(ctx)
		if err != nil {
			t.Fatalf("Failed to get stats: %v", err)
		}

		if stats.TotalCount < 4 {
			t.Errorf("Expected at least 4 memories, got %d", stats.TotalCount)
		}
	})

	t.Run("SearchByTag", func(t *testing.T) {
		results, err := manager.SearchByTag(ctx, "test")
		if err != nil {
			t.Fatalf("Failed to search by tag: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected at least one result with 'test' tag")
		}
	})

	t.Run("SearchByText", func(t *testing.T) {
		results, err := manager.SearchByText(ctx, "memory")
		if err != nil {
			t.Fatalf("Failed to search by text: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected at least one result containing 'memory'")
		}
	})
}

// mockEmbeddingProvider is a mock implementation for testing
type mockEmbeddingProvider struct {
	dimension int
}

func (m *mockEmbeddingProvider) Embed(text string) ([]float32, error) {
	result := make([]float32, m.dimension)
	// Generate deterministic pseudo-random embeddings based on text
	for i := 0; i < m.dimension; i++ {
		result[i] = float32((i+len(text))%100) / 100.0
	}
	return result, nil
}

func (m *mockEmbeddingProvider) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := m.Embed(text)
		if err != nil {
			return nil, err
		}
		results[i] = emb
	}
	return results, nil
}

func (m *mockEmbeddingProvider) Dimension() int {
	return m.dimension
}

func (m *mockEmbeddingProvider) MaxBatchSize() int {
	return 100
}

func TestChunkText(t *testing.T) {
	t.Run("SmallText", func(t *testing.T) {
		text := "small text"
		chunks := ChunkText(text, 100)
		if len(chunks) != 1 {
			t.Errorf("Expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0] != text {
			t.Errorf("Chunk content mismatch")
		}
	})

	t.Run("LargeText", func(t *testing.T) {
		text := "This is a sentence. This is another sentence. And a third one. "
		text += text + text + text // Make it long enough

		chunks := ChunkText(text, 50)
		if len(chunks) < 2 {
			t.Errorf("Expected multiple chunks, got %d", len(chunks))
		}

		// Verify chunks are approximately the right size
		for _, chunk := range chunks {
			if len(chunk) > 250 { // 50 tokens * 5 chars/token * some buffer
				t.Errorf("Chunk too large: %d chars", len(chunk))
			}
		}
	})
}

func TestSearchOptions(t *testing.T) {
	opts := DefaultSearchOptions()

	if opts.Limit != 10 {
		t.Errorf("Expected default limit 10, got %d", opts.Limit)
	}

	if opts.MinScore != 0.7 {
		t.Errorf("Expected default min score 0.7, got %f", opts.MinScore)
	}

	if !opts.Hybrid {
		t.Error("Expected hybrid to be enabled by default")
	}
}
