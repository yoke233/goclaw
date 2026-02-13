package runtime

import (
	"strings"

	"github.com/smallnest/goclaw/extensions"
)

// resolveSubagentBaseRoot chooses the base layer root directory.
//
// Rule:
// - If RoleDir is valid (contains .agents/config.toml or .agents/skills), use it.
// - Otherwise fall back to GoClawDir.
func resolveSubagentBaseRoot(req SubagentRunRequest) string {
	roleDir := strings.TrimSpace(req.RoleDir)
	if isAgentsRootValid(roleDir) {
		return roleDir
	}
	return strings.TrimSpace(req.GoClawDir)
}

func isAgentsRootValid(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	if fileExists(extensions.AgentsConfigPath(root)) {
		return true
	}
	if dirExists(extensions.AgentsSkillsDir(root)) {
		return true
	}
	return false
}
