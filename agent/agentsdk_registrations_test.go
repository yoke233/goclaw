package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentSDKRegistrations_FiltersDisabledAndSkipsInvalid(t *testing.T) {
	skillsDir := t.TempDir()

	// Valid skill: should load.
	demoDir := filepath.Join(skillsDir, "demo")
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatalf("mkdir demo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(demoDir, "SKILL.md"), []byte("---\nname: demo\ndescription: test\n---\n# Demo\n"), 0o644); err != nil {
		t.Fatalf("write demo: %v", err)
	}

	// Disabled skill: should be filtered out (no registration).
	offDir := filepath.Join(skillsDir, "off")
	if err := os.MkdirAll(offDir, 0o755); err != nil {
		t.Fatalf("mkdir off: %v", err)
	}
	if err := os.WriteFile(filepath.Join(offDir, "SKILL.md"), []byte("---\nname: off\ndescription: test\n---\n# Off\n"), 0o644); err != nil {
		t.Fatalf("write off: %v", err)
	}
	if err := os.WriteFile(filepath.Join(offDir, ".disabled"), []byte("disabled\n"), 0o644); err != nil {
		t.Fatalf("write off .disabled: %v", err)
	}

	// Invalid skill: should be skipped and surfaced as a warning.
	badDir := filepath.Join(skillsDir, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("mkdir bad: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("# Bad\n(no frontmatter)\n"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}

	regs, _, warnings := loadAgentSDKRegistrations(skillsDir)

	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d (warnings=%v)", len(regs), warnings)
	}

	got := map[string]bool{}
	for _, reg := range regs {
		got[reg.Definition.Name] = true
	}
	if !got["demo"] {
		t.Fatalf("expected demo registration, got: %#v", got)
	}
	if got["off"] {
		t.Fatalf("did not expect disabled skill to be registered, got: %#v", got)
	}
	if got["bad"] {
		t.Fatalf("did not expect invalid skill to be registered, got: %#v", got)
	}

	if len(warnings) == 0 {
		t.Fatalf("expected warnings for invalid skill, got none")
	}
}
