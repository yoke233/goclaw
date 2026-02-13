package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// agentsdk-go enforces this skill name format for built-in "skill" invocation.
// Keep goclaw's on-disk convention consistent so that a saved skill is actually loadable.
var agentsSkillNameRegexp = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

func isSafeSkillName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return agentsSkillNameRegexp.MatchString(name)
}

type skillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseSkillFrontMatter(md string) (skillFrontMatter, error) {
	trimmed := strings.TrimPrefix(md, "\uFEFF")
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skillFrontMatter{}, errors.New("missing YAML frontmatter (expected leading ---)")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return skillFrontMatter{}, errors.New("missing closing frontmatter separator (---)")
	}

	metaText := strings.Join(lines[1:end], "\n")
	var meta skillFrontMatter
	if err := yaml.Unmarshal([]byte(metaText), &meta); err != nil {
		return skillFrontMatter{}, fmt.Errorf("decode YAML frontmatter: %w", err)
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	return meta, nil
}
