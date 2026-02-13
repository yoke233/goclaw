package extensions

import "path/filepath"

// AgentsDir returns the .agents directory under the given root.
func AgentsDir(root string) string {
	return filepath.Join(root, ".agents")
}

// AgentsConfigPath returns the .agents/config.toml path under the given root.
func AgentsConfigPath(root string) string {
	return filepath.Join(root, ".agents", "config.toml")
}

// AgentsSkillsDir returns the .agents/skills directory under the given root.
func AgentsSkillsDir(root string) string {
	return filepath.Join(root, ".agents", "skills")
}
