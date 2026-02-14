package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smallnest/goclaw/extensions"
)

func TestBuildSubagentSkillRegistrationsRepoDisabledShouldOverrideBaseSkill(t *testing.T) {
	baseRoot := t.TempDir()
	repoRoot := t.TempDir()

	if err := writeSkill(baseRoot, "demo", "---\nname: demo\ndescription: base skill\n---\nbase body\n", false); err != nil {
		t.Fatalf("write base skill: %v", err)
	}
	if err := writeSkill(repoRoot, "demo", "---\nname: demo\ndescription: repo skill\n---\nrepo body\n", true); err != nil {
		t.Fatalf("write repo disabled skill: %v", err)
	}

	regs, _, _ := buildSubagentSkillRegistrations(SubagentRunRequest{
		GoClawDir: baseRoot,
		RepoDir:   repoRoot,
	}, extensions.ClaudePluginResult{})

	if len(regs) != 0 {
		t.Fatalf("repo disabled skill should suppress inherited base skill, got %d registrations", len(regs))
	}
}

func writeSkill(root, name, content string, disabled bool) error {
	skillDir := filepath.Join(extensions.AgentsSkillsDir(root), name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return err
	}
	if disabled {
		if err := os.WriteFile(filepath.Join(skillDir, ".disabled"), []byte("disabled\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
