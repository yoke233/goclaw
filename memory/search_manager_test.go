package memory

import (
	"context"
	"testing"
)

type mockEmbeddingProvider struct {
	batch [][]float32
}

func (m *mockEmbeddingProvider) Embed(text string) ([]float32, error) {
	return []float32{1, 2, 3}, nil
}

func (m *mockEmbeddingProvider) EmbedBatch(texts []string) ([][]float32, error) {
	return m.batch, nil
}

func (m *mockEmbeddingProvider) Dimension() int {
	return 3
}

func (m *mockEmbeddingProvider) MaxBatchSize() int {
	return 100
}

type mockStore struct{}

func (m *mockStore) Add(embedding *VectorEmbedding) error { return nil }

func (m *mockStore) AddBatch(embeddings []*VectorEmbedding) error { return nil }

func (m *mockStore) Search(query []float32, opts SearchOptions) ([]*SearchResult, error) {
	return nil, nil
}

func (m *mockStore) Get(id string) (*VectorEmbedding, error) {
	return &VectorEmbedding{ID: id, Text: "x"}, nil
}

func (m *mockStore) Delete(id string) error { return nil }

func (m *mockStore) Update(embedding *VectorEmbedding) error { return nil }

func (m *mockStore) List(filter func(*VectorEmbedding) bool) ([]*VectorEmbedding, error) {
	return nil, nil
}

func (m *mockStore) Close() error { return nil }

func TestAddMemoryBatchEmbeddingCountMismatchShouldReturnError(t *testing.T) {
	manager, err := NewMemoryManager(ManagerConfig{
		Store: &mockStore{},
		Provider: &mockEmbeddingProvider{
			// Intentionally return fewer embeddings than inputs.
			batch: [][]float32{{1, 2, 3}},
		},
		CacheMaxSize: 10,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	items := []MemoryItem{
		{Text: "a", Source: MemorySourceSession, Type: MemoryTypeFact},
		{Text: "b", Source: MemorySourceSession, Type: MemoryTypeFact},
	}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("embedding count mismatch should return error, got panic: %v", rec)
		}
	}()

	if err := manager.AddMemoryBatch(context.Background(), items); err == nil {
		t.Fatalf("expected error when embedding count mismatches input items")
	}
}
