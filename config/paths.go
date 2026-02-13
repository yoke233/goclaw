package config

import (
	"path/filepath"
	"strings"
)

// ExpandUserPath expands a leading "~" to the resolved user home directory.
// If expansion fails, the original path is returned.
func ExpandUserPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return path
	}
	if p == "~" {
		if home, err := ResolveUserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home, err := ResolveUserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return path
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(p, "~/"), "~\\")
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return path
}
