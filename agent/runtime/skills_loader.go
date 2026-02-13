package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	corehooks "github.com/cexll/agentsdk-go/pkg/core/hooks"
	sdkskills "github.com/cexll/agentsdk-go/pkg/runtime/skills"
	"github.com/smallnest/goclaw/extensions"
	"gopkg.in/yaml.v3"
)

var agentsSkillNameRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// skillFrontMatter mirrors the YAML frontmatter fields inside SKILL.md.
type skillFrontMatter struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
	AllowedTools toolList          `yaml:"allowed-tools,omitempty"`
}

// toolList supports YAML string or sequence, normalizing to a de-duplicated list.
type toolList []string

func (t *toolList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Tag == "!!null" {
		*t = nil
		return nil
	}

	var tools []string
	switch value.Kind {
	case yaml.ScalarNode:
		for _, entry := range strings.Split(value.Value, ",") {
			tool := strings.TrimSpace(entry)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
	case yaml.SequenceNode:
		for i, entry := range value.Content {
			if entry.Kind != yaml.ScalarNode {
				return fmt.Errorf("allowed-tools[%d]: expected string", i)
			}
			tool := strings.TrimSpace(entry.Value)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
	default:
		return errors.New("allowed-tools: expected string or sequence")
	}

	seen := map[string]struct{}{}
	deduped := tools[:0]
	for _, tool := range tools {
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		deduped = append(deduped, tool)
	}

	if len(deduped) == 0 {
		*t = nil
		return nil
	}
	*t = toolList(deduped)
	return nil
}

func buildSubagentSkillRegistrations(req SubagentRunRequest) ([]sdkapi.SkillRegistration, []corehooks.ShellHook, []string) {
	baseRoot := resolveSubagentBaseRoot(req)
	repoRoot := strings.TrimSpace(req.RepoDir)

	baseSkillsDir := ""
	if strings.TrimSpace(baseRoot) != "" {
		baseSkillsDir = extensions.AgentsSkillsDir(baseRoot)
	}
	repoSkillsDir := ""
	if repoRoot != "" {
		repoSkillsDir = extensions.AgentsSkillsDir(repoRoot)
	}

	baseSkills, baseWarnings := loadSkillsFromDir(baseSkillsDir)
	repoSkills, repoWarnings := loadSkillsFromDir(repoSkillsDir)

	merged := make(map[string]sdkapi.SkillRegistration, len(baseSkills)+len(repoSkills))
	for name, reg := range baseSkills {
		merged[name] = reg
	}
	for name, reg := range repoSkills {
		merged[name] = reg // repo overrides base
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	regs := make([]sdkapi.SkillRegistration, 0, len(names))
	for _, name := range names {
		regs = append(regs, merged[name])
	}

	warnings := make([]string, 0, len(baseWarnings)+len(repoWarnings)+1)
	warnings = append(warnings, baseWarnings...)
	warnings = append(warnings, repoWarnings...)
	if len(regs) == 0 {
		warnings = append(warnings, "no skills loaded from layered .agents/skills directories")
	}

	// hooks are not part of the .agents/skills convention (yet)
	return regs, nil, warnings
}

func loadSkillsFromDir(skillsDir string) (map[string]sdkapi.SkillRegistration, []string) {
	skillsDir = strings.TrimSpace(skillsDir)
	if skillsDir == "" {
		return map[string]sdkapi.SkillRegistration{}, nil
	}

	stat, err := os.Stat(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]sdkapi.SkillRegistration{}, nil
		}
		return map[string]sdkapi.SkillRegistration{}, []string{fmt.Sprintf("stat skills dir %s: %v", skillsDir, err)}
	}
	if stat == nil || !stat.IsDir() {
		return map[string]sdkapi.SkillRegistration{}, []string{fmt.Sprintf("skills path is not a directory: %s", skillsDir)}
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return map[string]sdkapi.SkillRegistration{}, []string{fmt.Sprintf("read skills dir %s: %v", skillsDir, err)}
	}

	out := make(map[string]sdkapi.SkillRegistration)
	var warnings []string

	for _, entry := range entries {
		if entry == nil || !entry.IsDir() {
			continue
		}
		dirName := strings.TrimSpace(entry.Name())
		if dirName == "" || strings.HasPrefix(dirName, ".") {
			continue
		}

		if !agentsSkillNameRegexp.MatchString(dirName) {
			warnings = append(warnings, fmt.Sprintf("skip skill %s: invalid directory name", dirName))
			continue
		}

		skillDir := filepath.Join(skillsDir, dirName)
		if fileExists(filepath.Join(skillDir, ".disabled")) {
			continue
		}

		skillFile := resolveSkillFile(skillDir)
		if skillFile == "" {
			continue
		}

		data, err := os.ReadFile(skillFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read skill %s: %v", skillFile, err))
			continue
		}

		meta, body, err := parseSkillFrontMatter(string(data))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("parse skill %s: %v", skillFile, err))
			continue
		}

		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = dirName
		}
		if name != dirName {
			warnings = append(warnings, fmt.Sprintf("skip skill %s: frontmatter name %q does not match directory name %q", skillFile, name, dirName))
			continue
		}

		if _, exists := out[name]; exists {
			warnings = append(warnings, fmt.Sprintf("skip duplicate skill %q under %s", name, skillsDir))
			continue
		}

		defMeta := map[string]string{}
		for k, v := range meta.Metadata {
			kk := strings.TrimSpace(k)
			if kk == "" {
				continue
			}
			defMeta[kk] = strings.TrimSpace(v)
		}
		defMeta["source"] = skillFile

		def := sdkskills.Definition{
			Name:        name,
			Description: strings.TrimSpace(meta.Description),
			Metadata:    defMeta,
		}
		skillName := name
		sourcePath := skillFile
		bodyText := strings.TrimSpace(body)
		metaCopy := meta
		handler := sdkskills.HandlerFunc(func(_ context.Context, _ sdkskills.ActivationContext) (sdkskills.Result, error) {
			output := map[string]any{"body": bodyText}
			resultMeta := map[string]any{"source": sourcePath}
			if len(metaCopy.AllowedTools) > 0 {
				resultMeta["allowed-tools"] = []string(metaCopy.AllowedTools)
			}
			return sdkskills.Result{
				Skill:    skillName,
				Output:   output,
				Metadata: resultMeta,
			}, nil
		})

		out[name] = sdkapi.SkillRegistration{
			Definition: def,
			Handler:    handler,
		}
	}

	return out, warnings
}

func resolveSkillFile(skillDir string) string {
	if strings.TrimSpace(skillDir) == "" {
		return ""
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if fileExists(path) {
		return path
	}
	path = filepath.Join(skillDir, "skill.md")
	if fileExists(path) {
		return path
	}
	return ""
}

func parseSkillFrontMatter(content string) (skillFrontMatter, string, error) {
	trimmed := strings.TrimPrefix(content, "\uFEFF")
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skillFrontMatter{}, "", errors.New("missing YAML frontmatter")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return skillFrontMatter{}, "", errors.New("missing closing frontmatter separator")
	}

	metaText := strings.Join(lines[1:end], "\n")
	var meta skillFrontMatter
	if err := yaml.Unmarshal([]byte(metaText), &meta); err != nil {
		return skillFrontMatter{}, "", fmt.Errorf("decode YAML: %w", err)
	}

	body := strings.Join(lines[end+1:], "\n")
	return meta, body, nil
}

func dirExists(path string) bool {
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat != nil && stat.IsDir()
}
