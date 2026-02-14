package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMemoryFileRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("failed to create memory dir: %v", err)
	}

	secretPath := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("top-secret"), 0644); err != nil {
		t.Fatalf("failed to create secret file: %v", err)
	}

	mgr := NewManager(root)
	content, err := mgr.ReadMemoryFile("../secret.txt")
	if err != nil {
		t.Fatalf("expected traversal to be blocked without fs error, got: %v", err)
	}
	if content != "" {
		t.Fatalf("expected traversal read to be blocked, got content: %q", content)
	}
}

func TestListMemoryFilesWithoutEnsureReturnsEmpty(t *testing.T) {
	mgr := NewManager(t.TempDir())

	files, err := mgr.ListMemoryFiles()
	if err != nil {
		t.Fatalf("expected no error when memory dir not initialized, got: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected empty file list, got: %v", files)
	}
}

func TestReadBootstrapFileRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(secret, []byte("outside-data"), 0644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	mgr := NewManager(filepath.Join(root, "workspace"))
	if err := os.MkdirAll(mgr.GetWorkspaceDir(), 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	content, err := mgr.ReadBootstrapFile("../outside.txt")
	if err != nil {
		t.Fatalf("expected traversal to be blocked without fs error, got: %v", err)
	}
	if content != "" {
		t.Fatalf("expected traversal read to be blocked, got content: %q", content)
	}
}
