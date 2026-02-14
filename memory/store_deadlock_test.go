package memory

import (
	"testing"
	"time"
)

func TestSQLiteStoreAddShouldNotBlockWhenVectorAndFTSDisabled(t *testing.T) {
	store, err := NewSQLiteStore(StoreConfig{
		DBPath:             ":memory:",
		Provider:           &mockEmbeddingProvider{},
		EnableVectorSearch: false,
		EnableFTS:          false,
	})
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- store.Add(&VectorEmbedding{
			ID:        "m-deadlock",
			Text:      "hello",
			Source:    MemorySourceSession,
			Type:      MemoryTypeFact,
			Vector:    []float32{1, 2, 3},
			Dimension: 3,
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Add() returned error: %v", err)
		}
		_ = store.Close()
	case <-time.After(2 * time.Second):
		t.Fatalf("Add() blocked unexpectedly; likely deadlock between transaction and metadata probe")
	}
}
