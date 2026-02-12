package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	corehooks "github.com/cexll/agentsdk-go/pkg/core/hooks"
	sdkprompts "github.com/cexll/agentsdk-go/pkg/prompts"
)

// loadAgentSDKRegistrations parses SKILL.md (and optional hooks) from a directory and
// returns agentsdk-go registrations plus warnings.
//
// The directory is treated as the "skills root", meaning it should contain:
//   <skillsDir>/<skill_name>/SKILL.md
//
// This mirrors the subagent runtime loader, but is kept in the main agent package
// to avoid import cycles between agent and agent/runtime.
func loadAgentSDKRegistrations(skillsDir string) ([]sdkapi.SkillRegistration, []corehooks.ShellHook, []string) {
	if strings.TrimSpace(skillsDir) == "" {
		return nil, nil, []string{"skills directory is empty"}
	}

	stat, err := os.Stat(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, []string{fmt.Sprintf("skills directory missing: %s", skillsDir)}
	}
	if !stat.IsDir() {
		return nil, nil, []string{fmt.Sprintf("skills path is not a directory: %s", skillsDir)}
	}

	baseFS := fs.FS(os.DirFS(skillsDir))
	disabled := findDisabledSkillDirs(skillsDir)
	if len(disabled) > 0 {
		baseFS = &disabledDirFS{
			base:    baseFS,
			disabled: disabled,
		}
	}

	parsed := sdkprompts.ParseWithOptions(baseFS, sdkprompts.ParseOptions{
		SkillsDir:    ".",
		CommandsDir:  "__none__",
		SubagentsDir: "__none__",
		HooksDir:     "__none__",
	})

	warnings := make([]string, 0, len(parsed.Errors)+1)
	for _, parseErr := range parsed.Errors {
		if parseErr != nil {
			warnings = append(warnings, fmt.Sprintf("skills parse warning: %v", parseErr))
		}
	}
	if len(parsed.Skills) == 0 {
		warnings = append(warnings, fmt.Sprintf("no SKILL.md found under: %s", skillsDir))
	}

	regs := make([]sdkapi.SkillRegistration, 0, len(parsed.Skills))
	for _, entry := range parsed.Skills {
		regs = append(regs, sdkapi.SkillRegistration{
			Definition: entry.Definition,
			Handler:    entry.Handler,
		})
	}
	return regs, parsed.Hooks, warnings
}

func findDisabledSkillDirs(skillsDir string) map[string]struct{} {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	disabled := make(map[string]struct{})
	for _, entry := range entries {
		if entry == nil || !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsDir, name, ".disabled")); err == nil {
			disabled[name] = struct{}{}
		}
	}
	if len(disabled) == 0 {
		return nil
	}
	return disabled
}

// disabledDirFS filters out disabled top-level skill directories (those containing a .disabled file).
// It is intentionally minimal: only ReadDir needs filtering for WalkDir-style traversals.
type disabledDirFS struct {
	base     fs.FS
	disabled map[string]struct{}
}

func (f *disabledDirFS) Open(name string) (fs.File, error) {
	if f == nil {
		return nil, fs.ErrNotExist
	}
	clean := path.Clean(filepath.ToSlash(name))
	clean = strings.TrimPrefix(clean, "./")
	parts := strings.SplitN(clean, "/", 2)
	if len(parts) > 0 {
		if _, ok := f.disabled[parts[0]]; ok {
			return nil, fs.ErrNotExist
		}
	}
	return f.base.Open(name)
}

func (f *disabledDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f == nil {
		return nil, fs.ErrNotExist
	}
	entries, err := fs.ReadDir(f.base, name)
	if err != nil {
		return nil, err
	}
	clean := path.Clean(filepath.ToSlash(name))
	clean = strings.TrimPrefix(clean, "./")
	if clean != "." && clean != "" {
		// Only filter the top-level listing.
		return entries, nil
	}

	filtered := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.IsDir() {
			if _, ok := f.disabled[entry.Name()]; ok {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered, nil
}
