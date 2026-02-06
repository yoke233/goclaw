package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}
}

func TestGetOrCreate(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sessionID := "test-session-1"

	// Get or create session
	sess, err := mgr.GetOrCreate(sessionID)
	if err != nil {
		t.Fatalf("Failed to get or create session: %v", err)
	}

	if sess == nil {
		t.Fatal("Expected non-nil session")
	}

	if sess.Key != sessionID {
		t.Errorf("Expected ID %s, got %s", sessionID, sess.Key)
	}

	// Get the same session again
	sess2, err := mgr.GetOrCreate(sessionID)
	if err != nil {
		t.Fatalf("Failed to get existing session: %v", err)
	}

	if sess2.Key != sess.Key {
		t.Error("Expected same session ID")
	}
}

func TestAddMessage(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add messages
	sess.AddMessage(Message{
		Role:    "user",
		Content: "Hello",
	})

	sess.AddMessage(Message{
		Role:    "assistant",
		Content: "Hi there!",
	})

	history := sess.GetHistory(10)
	if len(history) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(history))
	}

	if history[0].Role != "user" {
		t.Errorf("Expected role 'user', got %s", history[0].Role)
	}
}

func TestGetHistory(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add 5 messages
	for i := 0; i < 5; i++ {
		sess.AddMessage(Message{
			Role:    "user",
			Content: string(rune('a' + i)),
		})
	}

	// Get last 3
	history := sess.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(history))
	}

	// Get all
	history = sess.GetHistory(100)
	if len(history) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(history))
	}
}

func TestClear(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add messages
	sess.AddMessage(Message{Role: "user", Content: "Test"})
	sess.AddMessage(Message{Role: "assistant", Content: "Response"})

	if len(sess.GetHistory(10)) != 2 {
		t.Fatal("Expected 2 messages before clear")
	}

	// Clear
	sess.Clear()

	if len(sess.GetHistory(10)) != 0 {
		t.Error("Expected 0 messages after clear")
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add messages
	sess.AddMessage(Message{Role: "user", Content: "Question"})
	sess.AddMessage(Message{Role: "assistant", Content: "Answer"})

	// Save
	if err := mgr.Save(sess); err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Check file exists
	filePath := filepath.Join(tmpDir, sess.Key+".jsonl")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Session file was not created")
	}

	// Load session
	sess2, err := mgr.GetOrCreate(sess.Key)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	history := sess2.GetHistory(10)
	if len(history) != 2 {
		t.Errorf("Expected 2 messages after load, got %d", len(history))
	}

	if history[0].Content != "Question" {
		t.Errorf("Expected content 'Question', got %s", history[0].Content)
	}
}

func TestUpdatedAt(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Session should be updated when created
	if sess.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}

	originalTime := sess.UpdatedAt

	// Wait a bit and add message
	time.Sleep(10 * time.Millisecond)
	sess.AddMessage(Message{Role: "user", Content: "Test"})

	if !sess.UpdatedAt.After(originalTime) {
		t.Error("Expected UpdatedAt to be updated after adding message")
	}
}

func TestDelete(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	sess, err := mgr.GetOrCreate("test-session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Save session
	if err := mgr.Save(sess); err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Check file exists
	filePath := filepath.Join(tmpDir, sess.Key+".jsonl")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Session file was not created")
	}

	// Delete session
	if err := mgr.Delete(sess.Key); err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Check file is deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("Session file was not deleted")
	}
}

func TestListSessions(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create multiple sessions
	sessionIDs := []string{"session-1", "session-2", "session-3"}
	for _, id := range sessionIDs {
		sess, err := mgr.GetOrCreate(id)
		if err != nil {
			t.Fatalf("Failed to create session %s: %v", id, err)
		}
		sess.AddMessage(Message{Role: "user", Content: "Test"})
		if err := mgr.Save(sess); err != nil {
			t.Fatalf("Failed to save session %s: %v", id, err)
		}
	}

	// List sessions
	sessions, err := mgr.List()
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != len(sessionIDs) {
		t.Errorf("Expected %d sessions, got %d", len(sessionIDs), len(sessions))
	}
}
