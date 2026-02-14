package agentsdkcompat

import (
	"path/filepath"
	"strings"
)

// NormalizeSkillDirs trims, cleans, and deduplicates skill directories while
// preserving order.
func NormalizeSkillDirs(dirs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(dirs))
	for _, raw := range dirs {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			continue
		}
		clean := filepath.Clean(dir)
		if clean == "." || clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out
}
