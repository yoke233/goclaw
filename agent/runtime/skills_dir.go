package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveSubagentSkillsDir(req SubagentRunRequest) string {
	// If the subagent has a self-contained skills directory under its own WorkDir,
	// prefer it to allow per-subagent customization without extra wiring.
	//
	// Convention: <workdir>/.goclaw/skills/<skill>/SKILL.md
	workDir := strings.TrimSpace(req.WorkDir)
	if workDir != "" {
		local := filepath.Join(workDir, ".goclaw", "skills")
		if dirExists(local) {
			return local
		}
	}
	return strings.TrimSpace(req.SkillsDir)
}

func dirExists(path string) bool {
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat != nil && stat.IsDir()
}
