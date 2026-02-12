package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// LoadSkillsOptions configures skill loading (using types.go version)
// This uses the full LoadSkillsOptions from types.go with additional fields needed internally
type DiscoveryOptions struct {
	LoadSkillsOptions
	// Additional options specific to discovery
	WorkspaceDir string
	PluginDirs   []string
}

// loadSkillsFromDir loads skills from a directory
func loadSkillsFromDir(dir, source string) ([]*Skill, error) {
	var skills []*Skill
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for SKILL.md in subdirectories
		if d.IsDir() && d.Name() != dir {
			return nil
		}

		// Check for .md files
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			skill, err := loadSkillFromFile(path, source)
			if err != nil {
				logger.Warn("Failed to load skill", zap.String("path", path), zap.Error(err))
				return nil
			}
			if skill != nil {
				skills = append(skills, skill)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return skills, nil
}

// loadSkillFromFile loads a skill from a markdown file
func loadSkillFromFile(filePath, source string) (*Skill, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	// Parse frontmatter using existing frontmatter parser
	frontmatter := ParseFrontmatter(string(content))
	if len(frontmatter) == 0 {
		return nil, fmt.Errorf("no frontmatter found")
	}

	// Extract name and description
	name, ok := frontmatter["name"]
	if !ok {
		name = filepath.Base(filepath.Dir(filePath))
	}

	description := frontmatter["description"]
	if description == "" {
		return nil, fmt.Errorf("skill description is required")
	}

	return &Skill{
		Name:        name,
		Description: description,
		FilePath:    filePath,
		BaseDir:     filepath.Dir(filePath),
		Source:      source,
		Content:     StripFrontmatter(string(content)),
		Frontmatter: frontmatter,
	}, nil
}

// parseFrontmatter parses YAML frontmatter from markdown content
func parseFrontmatter(content string) map[string]string {
	if !strings.HasPrefix(content, "---") {
		return nil
	}

	endIdx := strings.Index(content[3:], "---")
	if endIdx == -1 {
		return nil
	}

	yamlContent := content[4 : endIdx+3]
	return parseYAML(yamlContent)
}

// parseYAML parses YAML content into a map
func parseYAML(yamlContent string) map[string]string {
	// Simple YAML parsing for now - will be replaced by proper YAML/JSON5 parser
	lines := strings.Split(yamlContent, "\n")
	result := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Remove quotes if present
		value = strings.Trim(value, "'\"")
		result[key] = value
	}

	return result
}

// extractContent extracts the markdown content after YAML frontmatter
func extractContent(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// Find the closing delimiter (searching after the opening "---")
	rest := content[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx == -1 {
		return content
	}

	// Extract content after the closing "---" and trim leading whitespace
	result := rest[endIdx+3:]
	return strings.TrimSpace(result)
}

// resolveBundledSkillsDir resolves the path to bundled skills directory
func resolveBundledSkillsDir(opts LoadSkillsOptions) (string, error) {
	// Check environment variable
	if dir := os.Getenv("GOCLAW_BUNDLED_SKILLS_DIR"); dir != "" {
		if looksLikeSkillsDir(dir) {
			return dir, nil
		}
	}

	// Check executable sibling directory
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		sibling := filepath.Join(exeDir, "skills")
		if looksLikeSkillsDir(sibling) {
			return sibling, nil
		}
	}

	// Check current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "skills")
		if looksLikeSkillsDir(candidate) {
			return candidate, nil
		}
	}

	return "", nil
}

// looksLikeSkillsDir checks if a directory contains skill files
func looksLikeSkillsDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, entry.Name(), "SKILL.md")); err == nil {
				return true
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			return true
		}
	}

	return false
}

// scanPluginSkillDirs scans for skill directories from plugins
func scanPluginSkillDirs(workspaceDir string, cfg map[string]interface{}) []string {
	// TODO: Implement plugin scanning
	return nil
}

// LoadSkillEntries loads skill entries from multiple directories with priority
func LoadSkillEntries(workspaceDir string, opts LoadSkillsOptions) ([]*SkillEntry, error) {
	// Resolve bundled skills directory
	bundledSkillsDir, err := resolveBundledSkillsDir(opts)
	if err != nil {
		logger.Warn("Failed to resolve bundled skills dir", zap.Error(err))
	}

	// Determine managed skills directory
	managedSkillsDir := opts.ManagedSkillsDir
	if managedSkillsDir == "" {
		home, err := config.ResolveUserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		managedSkillsDir = filepath.Join(home, ".goclaw", "skills")
	}

	workspaceSkillsDir := filepath.Join(workspaceDir, "skills")
	// Get extra directories from skills config
	var extraDirs []string
	if opts.SkillsConfig != nil && opts.SkillsConfig.Load.ExtraDirs != nil {
		extraDirs = opts.SkillsConfig.Load.ExtraDirs
	}

	// Get plugin skill directories
	pluginSkillDirs := scanPluginSkillDirs(workspaceDir, nil)

	// Load skills from all sources
	var allSkills []*Skill
	sources := []struct {
		dir    string
		source string
	}{
		// Extra directories (lowest priority)
	}

	// Add extra directories
	for _, dir := range extraDirs {
		if dir != "" {
			sources = append(sources, struct {
				dir    string
				source string
			}{dir: dir, source: "extra"})
		}
	}

	// Add plugin directories
	for _, dir := range pluginSkillDirs {
		if dir != "" {
			sources = append(sources, struct {
				dir    string
				source string
			}{dir: dir, source: "plugin"})
		}
	}

	// Add bundled directory
	if bundledSkillsDir != "" {
		sources = append(sources, struct {
			dir    string
			source string
		}{dir: bundledSkillsDir, source: "bundled"})
	}

	// Add managed directory
	if _, err := os.Stat(managedSkillsDir); err == nil {
		sources = append(sources, struct {
			dir    string
			source string
		}{dir: managedSkillsDir, source: "managed"})
	}

	// Add workspace directory
	if _, err := os.Stat(workspaceSkillsDir); err == nil {
		sources = append(sources, struct {
			dir    string
			source string
		}{dir: workspaceSkillsDir, source: "workspace"})
	}

	// Load skills from each source
	for _, source := range sources {
		skills, err := loadSkillsFromDir(source.dir, source.source)
		if err != nil {
			logger.Warn("Failed to load skills from directory",
				zap.String("dir", source.dir), zap.String("source", source.source), zap.Error(err))
			continue
		}
		allSkills = append(allSkills, skills...)
	}

	// Merge skills by name (later sources override earlier ones)
	merged := make(map[string]*Skill)
	for _, skill := range allSkills {
		merged[skill.Name] = skill
	}

	// Create skill entries
	var entries []*SkillEntry
	for _, skill := range merged {
		// Use already parsed frontmatter from the skill
		frontmatter := skill.Frontmatter
		if frontmatter == nil {
			// Try parsing from content as fallback (for skills loaded without frontmatter)
			frontmatter = parseFrontmatter(skill.Content)
			if frontmatter == nil {
				continue
			}
		}

		// Parse metadata
		metadata := resolveOpenClawMetadata(frontmatter)

		// Determine invocation policy
		invocation := resolveSkillInvocationPolicy(frontmatter)

		entries = append(entries, &SkillEntry{
			Skill:            skill,
			Frontmatter:      frontmatter,
			Metadata:         metadata,
			InvocationPolicy: invocation,
		})
	}

	return entries, nil
}

// resolveOpenClawMetadata parses OpenClaw-specific metadata from frontmatter
func resolveOpenClawMetadata(frontmatter map[string]string) *OpenClawSkillMetadata {
	// TODO: Implement JSON5 parsing of metadata field
	return nil
}

// resolveSkillInvocationPolicy determines skill invocation policy from frontmatter
func resolveSkillInvocationPolicy(frontmatter map[string]string) *SkillInvocationPolicy {
	policy := &SkillInvocationPolicy{
		UserInvocable:          true,
		DisableModelInvocation: false,
	}

	// Parse user-invocable
	if val, ok := frontmatter["user-invocable"]; ok {
		policy.UserInvocable = strings.ToLower(val) == "true" || val == "1"
	}

	// Parse disable-model-invocation
	if val, ok := frontmatter["disable-model-invocation"]; ok {
		policy.DisableModelInvocation = strings.ToLower(val) == "true" || val == "1"
	}

	return policy
}
