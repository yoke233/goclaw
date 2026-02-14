package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerSaveAndLoadRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	manager, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	key := "user/1:alpha"
	session, err := manager.GetOrCreate(key)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	session.Metadata["type"] = "dm"
	session.AddMessage(Message{
		Role:      "assistant",
		Content:   "hello",
		Timestamp: time.Now(),
		Media: []Media{
			{
				Type:     "image",
				URL:      "https://example.com/a.png",
				MimeType: "image/png",
			},
		},
		ToolCalls: []ToolCall{
			{
				ID:   "tool-1",
				Name: "search",
				Params: map[string]interface{}{
					"q": "abc",
				},
			},
		},
		Metadata: map[string]interface{}{
			"source": "test",
		},
	})

	if err := manager.Save(session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	savedPath := manager.SessionPath(key)
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("expected saved file to exist: %v", err)
	}
	if strings.Contains(filepath.Base(savedPath), "/") || strings.Contains(filepath.Base(savedPath), ":") {
		t.Fatalf("session path should sanitize special characters: %s", savedPath)
	}

	reloadedManager, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create reloaded manager: %v", err)
	}

	loaded, err := reloadedManager.GetOrCreate(key)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if loaded.Key != key {
		t.Fatalf("expected key %q, got %q", key, loaded.Key)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Fatalf("expected content hello, got %q", loaded.Messages[0].Content)
	}
	if loaded.Metadata["type"] != "dm" {
		t.Fatalf("expected metadata type=dm, got %v", loaded.Metadata["type"])
	}
}

func TestManagerListOnlyReturnsJSONL(t *testing.T) {
	baseDir := t.TempDir()
	manager, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if err := os.WriteFile(filepath.Join(baseDir, "a.jsonl"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to create jsonl file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "b.txt"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to create txt file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(baseDir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	keys, err := manager.List()
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(keys) != 1 || keys[0] != "a" {
		t.Fatalf("expected only [a], got %v", keys)
	}
}

func TestManagerListShouldReturnOriginalKeyForSpecialCharacters(t *testing.T) {
	baseDir := t.TempDir()
	manager, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	originalKey := "user/1:alpha"
	s, err := manager.GetOrCreate(originalKey)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	s.AddMessage(Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: time.Now(),
	})
	if err := manager.Save(s); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	reloaded, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create reloaded manager: %v", err)
	}

	keys, err := reloaded.List()
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected one session key, got %v", keys)
	}
	if keys[0] != originalKey {
		t.Fatalf("expected listed key %q, got %q", originalKey, keys[0])
	}
}

func TestManagerSaveShouldNotCollideWhenKeysSanitizeToSameFilename(t *testing.T) {
	baseDir := t.TempDir()
	manager, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	keyA := "team/a:b"
	sessionA, err := manager.GetOrCreate(keyA)
	if err != nil {
		t.Fatalf("failed to create session A: %v", err)
	}
	sessionA.AddMessage(Message{
		Role:      "user",
		Content:   "message-from-A",
		Timestamp: time.Now(),
	})
	if err := manager.Save(sessionA); err != nil {
		t.Fatalf("failed to save session A: %v", err)
	}

	keyB := "team_a/b"
	sessionB, err := manager.GetOrCreate(keyB)
	if err != nil {
		t.Fatalf("failed to create session B: %v", err)
	}
	sessionB.AddMessage(Message{
		Role:      "user",
		Content:   "message-from-B",
		Timestamp: time.Now(),
	})
	if err := manager.Save(sessionB); err != nil {
		t.Fatalf("failed to save session B: %v", err)
	}

	reloaded, err := NewManager(baseDir)
	if err != nil {
		t.Fatalf("failed to create reloaded manager: %v", err)
	}
	loadedA, err := reloaded.GetOrCreate(keyA)
	if err != nil {
		t.Fatalf("failed to load session A: %v", err)
	}

	if len(loadedA.Messages) != 1 {
		t.Fatalf("expected session A to keep isolated history with 1 message, got %d", len(loadedA.Messages))
	}
	if loadedA.Messages[0].Content != "message-from-A" {
		t.Fatalf("expected session A content %q, got %q", "message-from-A", loadedA.Messages[0].Content)
	}

	loadedB, err := reloaded.GetOrCreate(keyB)
	if err != nil {
		t.Fatalf("failed to load session B: %v", err)
	}
	if len(loadedB.Messages) != 1 {
		t.Fatalf("expected session B to keep isolated history with 1 message, got %d", len(loadedB.Messages))
	}
	if loadedB.Messages[0].Content != "message-from-B" {
		t.Fatalf("expected session B content %q, got %q", "message-from-B", loadedB.Messages[0].Content)
	}
}
