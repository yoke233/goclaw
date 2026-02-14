package qmd

import (
	"path/filepath"
	"testing"

	"github.com/smallnest/goclaw/config"
)

func TestTruncateSnippetSmallMaxLenShouldNotPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("truncateSnippet should handle small maxLen, got panic: %v", rec)
		}
	}()

	got := truncateSnippet("abcdef", 2)
	if got == "" {
		t.Fatalf("expected non-empty truncated snippet")
	}
}

func TestExpandHomeDirSupportsWindowsStylePrefix(t *testing.T) {
	home, err := config.ResolveUserHomeDir()
	if err != nil {
		t.Fatalf("failed to resolve home dir: %v", err)
	}

	got := expandHomeDir("~\\docs")
	want := filepath.Join(home, "docs")
	if got != want {
		t.Fatalf("expected expanded path %q, got %q", want, got)
	}
}
