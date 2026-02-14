package extensions

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sdkapi "github.com/cexll/agentsdk-go/pkg/api"
	sdkconfig "github.com/cexll/agentsdk-go/pkg/config"
	coreevents "github.com/cexll/agentsdk-go/pkg/core/events"
	corehooks "github.com/cexll/agentsdk-go/pkg/core/hooks"
	sdkprompts "github.com/cexll/agentsdk-go/pkg/prompts"
	"github.com/smallnest/goclaw/config"
)

const (
	claudePluginManifestDir  = ".claude-plugin"
	claudePluginManifestFile = "plugin.json"
	claudePluginRootDir      = ".claude"
	claudePluginDirName      = "plugins"
	claudePluginMCPFile      = ".mcp.json"
)

// ClaudePluginInfo captures an installed Claude Code plugin root.
type ClaudePluginInfo struct {
	Name string
	Root string
}

// ClaudePluginResult aggregates loaded Claude Code plugin contributions.
type ClaudePluginResult struct {
	SkillDirs []string
	Commands  []sdkapi.CommandRegistration
	Subagents []sdkapi.SubagentRegistration
	Hooks     []corehooks.ShellHook
	MCP       *AgentsConfig
	Plugins   []ClaudePluginInfo
	Warnings  []string
}

// LoadClaudePlugins discovers and loads Claude Code plugins for a project root.
// Plugins are discovered from user + project scope and merged in that order.
func LoadClaudePlugins(projectRoot string) ClaudePluginResult {
	var result ClaudePluginResult
	baseDirs, warnings := discoverClaudePluginBases(projectRoot)
	result.Warnings = append(result.Warnings, warnings...)

	for _, base := range baseDirs {
		pluginRoots := scanClaudePluginRoots(base)
		for _, root := range pluginRoots {
			pluginResult := loadClaudePlugin(root, projectRoot)
			if len(pluginResult.Plugins) > 0 {
				result.Plugins = append(result.Plugins, pluginResult.Plugins...)
			}
			result.SkillDirs = append(result.SkillDirs, pluginResult.SkillDirs...)
			result.Commands = append(result.Commands, pluginResult.Commands...)
			result.Subagents = append(result.Subagents, pluginResult.Subagents...)
			result.Hooks = append(result.Hooks, pluginResult.Hooks...)
			if pluginResult.MCP != nil && len(pluginResult.MCP.MCPServers) > 0 {
				result.MCP = MergeAgentsConfig(result.MCP, pluginResult.MCP)
			}
			result.Warnings = append(result.Warnings, pluginResult.Warnings...)
		}
	}

	result.SkillDirs = uniqueStrings(result.SkillDirs)

	return result
}

type claudePluginManifest struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Components  claudePluginComponents `json:"components"`

	Commands   json.RawMessage `json:"commands"`
	Agents     json.RawMessage `json:"agents"`
	Skills     json.RawMessage `json:"skills"`
	Hooks      json.RawMessage `json:"hooks"`
	MCPServers json.RawMessage `json:"mcpServers"`
}

type claudePluginComponents struct {
	Commands   json.RawMessage `json:"commands"`
	Agents     json.RawMessage `json:"agents"`
	Skills     json.RawMessage `json:"skills"`
	Hooks      json.RawMessage `json:"hooks"`
	MCPServers json.RawMessage `json:"mcpServers"`
}

type pluginMCPServer struct {
	Type               string            `json:"type,omitempty"`
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	URL                string            `json:"url,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	TimeoutSeconds     int               `json:"timeoutSeconds,omitempty"`
	EnabledTools       []string          `json:"enabledTools,omitempty"`
	DisabledTools      []string          `json:"disabledTools,omitempty"`
	ToolTimeoutSeconds int               `json:"toolTimeoutSeconds,omitempty"`
}

func (m claudePluginManifest) componentRaw(field string) json.RawMessage {
	switch field {
	case "commands":
		if len(m.Components.Commands) > 0 {
			return m.Components.Commands
		}
		return m.Commands
	case "agents":
		if len(m.Components.Agents) > 0 {
			return m.Components.Agents
		}
		return m.Agents
	case "skills":
		if len(m.Components.Skills) > 0 {
			return m.Components.Skills
		}
		return m.Skills
	case "hooks":
		if len(m.Components.Hooks) > 0 {
			return m.Components.Hooks
		}
		return m.Hooks
	case "mcpServers":
		if len(m.Components.MCPServers) > 0 {
			return m.Components.MCPServers
		}
		return m.MCPServers
	default:
		return nil
	}
}

func discoverClaudePluginBases(projectRoot string) ([]string, []string) {
	var bases []string
	var warnings []string

	if home, err := config.ResolveUserHomeDir(); err == nil {
		if strings.TrimSpace(home) != "" {
			bases = append(bases, filepath.Join(home, claudePluginRootDir, claudePluginDirName))
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("resolve user home for claude plugins: %v", err))
	}

	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot != "" {
		bases = append(bases, filepath.Join(projectRoot, claudePluginRootDir, claudePluginDirName))
	}

	return bases, warnings
}

func scanClaudePluginRoots(baseDir string) []string {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}

	var roots []string
	for _, entry := range entries {
		if entry == nil || !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		root := filepath.Join(baseDir, name)
		if looksLikeClaudePluginRoot(root) {
			roots = append(roots, root)
		}
	}

	sort.Strings(roots)
	return roots
}

func findClaudePluginManifest(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}

	candidate := filepath.Join(root, claudePluginManifestDir, claudePluginManifestFile)
	if fileExists(candidate) {
		return candidate
	}
	candidate = filepath.Join(root, claudePluginManifestFile)
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

func looksLikeClaudePluginRoot(root string) bool {
	if findClaudePluginManifest(root) != "" {
		return true
	}
	if dirExists(filepath.Join(root, "skills")) {
		return true
	}
	if dirExists(filepath.Join(root, "commands")) {
		return true
	}
	if dirExists(filepath.Join(root, "agents")) {
		return true
	}
	if dirExists(filepath.Join(root, "hooks")) {
		return true
	}
	if fileExists(filepath.Join(root, claudePluginMCPFile)) {
		return true
	}
	if fileExists(filepath.Join(root, ".lsp.json")) {
		return true
	}
	return false
}

func loadClaudePlugin(root, projectRoot string) ClaudePluginResult {
	var result ClaudePluginResult

	manifestPath := findClaudePluginManifest(root)
	var manifest claudePluginManifest
	if manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("read claude plugin manifest %s: %v", manifestPath, err))
		} else if err := json.Unmarshal(data, &manifest); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("parse claude plugin manifest %s: %v", manifestPath, err))
		}
	}

	pluginName := strings.TrimSpace(manifest.Name)
	if pluginName == "" {
		pluginName = filepath.Base(root)
	}
	result.Plugins = append(result.Plugins, ClaudePluginInfo{Name: pluginName, Root: root})

	fsys := fs.FS(os.DirFS(root))

	skillPaths := buildComponentPaths(manifest.componentRaw("skills"), []string{"skills"})
	commandPaths := buildComponentPaths(manifest.componentRaw("commands"), []string{"commands"})
	agentPaths := buildComponentPaths(manifest.componentRaw("agents"), []string{"agents"})
	hookPaths, inlineHooks := buildComponentPathsWithInline(manifest.componentRaw("hooks"), []string{"hooks"})
	mcpPaths, inlineMCP := buildComponentPathsWithInline(manifest.componentRaw("mcpServers"), nil)

	// Always consider .mcp.json at plugin root if present.
	if fileExists(filepath.Join(root, claudePluginMCPFile)) {
		mcpPaths = append(mcpPaths, claudePluginMCPFile)
	}

	skillPaths = uniqueStrings(skillPaths)
	commandPaths = uniqueStrings(commandPaths)
	agentPaths = uniqueStrings(agentPaths)
	hookPaths = uniqueStrings(hookPaths)
	mcpPaths = uniqueStrings(mcpPaths)

	for _, rel := range skillPaths {
		abs, _, err := resolvePluginPath(root, rel)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plugin %s skills path %q: %v", pluginName, rel, err))
			continue
		}
		if !dirExists(abs) {
			continue
		}
		result.SkillDirs = append(result.SkillDirs, abs)
	}

	for _, rel := range commandPaths {
		abs, relPath, err := resolvePluginPath(root, rel)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plugin %s commands path %q: %v", pluginName, rel, err))
			continue
		}
		if !dirExists(abs) {
			if fileExists(abs) {
				// If a file was specified, fall back to its parent directory.
				relPath = filepath.ToSlash(filepath.Dir(relPath))
				if relPath == "." {
					relPath = ""
				}
			} else {
				continue
			}
		}
		regs, warnings := parsePluginCommands(fsys, relPath, pluginName)
		result.Commands = append(result.Commands, regs...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	for _, rel := range agentPaths {
		abs, relPath, err := resolvePluginPath(root, rel)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plugin %s agents path %q: %v", pluginName, rel, err))
			continue
		}
		if !dirExists(abs) {
			if fileExists(abs) {
				relPath = filepath.ToSlash(filepath.Dir(relPath))
				if relPath == "." {
					relPath = ""
				}
			} else {
				continue
			}
		}
		regs, warnings := parsePluginSubagents(fsys, relPath, pluginName)
		result.Subagents = append(result.Subagents, regs...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	for _, rel := range hookPaths {
		abs, relPath, err := resolvePluginPath(root, rel)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plugin %s hooks path %q: %v", pluginName, rel, err))
			continue
		}
		if fileExists(abs) {
			hooks, warnings := loadHooksFromFile(abs, pluginName, root, projectRoot)
			result.Hooks = append(result.Hooks, hooks...)
			result.Warnings = append(result.Warnings, warnings...)
			continue
		}
		if !dirExists(abs) {
			continue
		}
		hooks, warnings := parsePluginHooks(fsys, relPath, pluginName)
		hooks = injectPluginHookEnv(hooks, pluginName, root, projectRoot)
		result.Hooks = append(result.Hooks, hooks...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if len(inlineHooks) > 0 {
		hooks, warnings := loadHooksFromInline(inlineHooks, pluginName, root, projectRoot)
		result.Hooks = append(result.Hooks, hooks...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if len(inlineMCP) > 0 {
		cfg, warnings := parsePluginMCPInline(inlineMCP, pluginName, root, projectRoot)
		result.Warnings = append(result.Warnings, warnings...)
		if cfg != nil && len(cfg.MCPServers) > 0 {
			result.MCP = MergeAgentsConfig(result.MCP, cfg)
		}
	}

	for _, rel := range mcpPaths {
		abs, _, err := resolvePluginPath(root, rel)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plugin %s mcp path %q: %v", pluginName, rel, err))
			continue
		}
		if !fileExists(abs) {
			continue
		}
		cfg, warnings := parsePluginMCPFile(abs, pluginName, root, projectRoot)
		result.Warnings = append(result.Warnings, warnings...)
		if cfg != nil && len(cfg.MCPServers) > 0 {
			result.MCP = MergeAgentsConfig(result.MCP, cfg)
		}
	}

	return result
}

func buildComponentPaths(raw json.RawMessage, defaults []string) []string {
	paths := append([]string(nil), defaults...)
	list, ok, _ := parseStringList(raw)
	if ok {
		paths = append(paths, list...)
	}
	return paths
}

func buildComponentPathsWithInline(raw json.RawMessage, defaults []string) ([]string, json.RawMessage) {
	paths := append([]string(nil), defaults...)
	list, ok, _ := parseStringList(raw)
	if ok {
		paths = append(paths, list...)
		return paths, nil
	}
	if len(raw) == 0 {
		return paths, nil
	}
	return paths, raw
}

func parseStringList(raw json.RawMessage) ([]string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, true, nil
		}
		return []string{single}, true, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, true, nil
	}
	return nil, false, nil
}

func resolvePluginPath(root, rel string) (string, string, error) {
	root = strings.TrimSpace(root)
	rel = strings.TrimSpace(rel)
	if root == "" {
		return "", "", fmt.Errorf("plugin root is empty")
	}
	if rel == "" {
		return "", "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("absolute paths not allowed")
	}

	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") {
		return "", "", fmt.Errorf("path escapes plugin root")
	}
	abs := filepath.Join(root, clean)
	relPath, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", fmt.Errorf("resolve relative path: %w", err)
	}
	relPath = filepath.ToSlash(relPath)
	return abs, relPath, nil
}

func parsePluginCommands(fsys fs.FS, relPath, pluginName string) ([]sdkapi.CommandRegistration, []string) {
	if strings.TrimSpace(relPath) == "" {
		return nil, nil
	}
	builtins := sdkprompts.ParseWithOptions(fsys, sdkprompts.ParseOptions{
		SkillsDir:    "__none__",
		CommandsDir:  relPath,
		SubagentsDir: "__none__",
		HooksDir:     "__none__",
		Validate:     true,
	})
	warnings := collectPromptWarnings(pluginName, "commands", builtins.Errors)
	regs := make([]sdkapi.CommandRegistration, 0, len(builtins.Commands))
	for _, entry := range builtins.Commands {
		regs = append(regs, sdkapi.CommandRegistration{Definition: entry.Definition, Handler: entry.Handler})
	}
	return regs, warnings
}

func parsePluginSubagents(fsys fs.FS, relPath, pluginName string) ([]sdkapi.SubagentRegistration, []string) {
	if strings.TrimSpace(relPath) == "" {
		return nil, nil
	}
	builtins := sdkprompts.ParseWithOptions(fsys, sdkprompts.ParseOptions{
		SkillsDir:    "__none__",
		CommandsDir:  "__none__",
		SubagentsDir: relPath,
		HooksDir:     "__none__",
		Validate:     true,
	})
	warnings := collectPromptWarnings(pluginName, "agents", builtins.Errors)
	regs := make([]sdkapi.SubagentRegistration, 0, len(builtins.Subagents))
	for _, entry := range builtins.Subagents {
		regs = append(regs, sdkapi.SubagentRegistration{Definition: entry.Definition, Handler: entry.Handler})
	}
	return regs, warnings
}

func parsePluginHooks(fsys fs.FS, relPath, pluginName string) ([]corehooks.ShellHook, []string) {
	if strings.TrimSpace(relPath) == "" {
		return nil, nil
	}
	builtins := sdkprompts.ParseWithOptions(fsys, sdkprompts.ParseOptions{
		SkillsDir:    "__none__",
		CommandsDir:  "__none__",
		SubagentsDir: "__none__",
		HooksDir:     relPath,
	})
	warnings := collectPromptWarnings(pluginName, "hooks", builtins.Errors)
	return builtins.Hooks, warnings
}

func collectPromptWarnings(pluginName, component string, errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("plugin %s %s warning: %v", pluginName, component, err))
	}
	return warnings
}

func loadHooksFromFile(path, pluginName, pluginRoot, projectRoot string) ([]corehooks.ShellHook, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("plugin %s hooks file %s: %v", pluginName, path, err)}
	}

	hooks, warnings := loadHooksFromInline(data, pluginName, pluginRoot, projectRoot)
	if len(warnings) > 0 {
		for i := range warnings {
			warnings[i] = fmt.Sprintf("plugin %s hooks file %s: %s", pluginName, path, warnings[i])
		}
	}
	return hooks, warnings
}

func loadHooksFromInline(raw json.RawMessage, pluginName, pluginRoot, projectRoot string) ([]corehooks.ShellHook, []string) {
	if len(raw) == 0 {
		return nil, nil
	}

	cfg, warnings := parseHooksConfig(raw)
	if cfg == nil {
		return nil, warnings
	}
	hooks := hooksConfigToShellHooks(cfg, pluginName, pluginRoot, projectRoot)
	return hooks, warnings
}

func parseHooksConfig(raw []byte) (*sdkconfig.HooksConfig, []string) {
	if len(raw) == 0 {
		return nil, nil
	}

	var wrapper struct {
		Hooks sdkconfig.HooksConfig `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if !hooksConfigEmpty(&wrapper.Hooks) {
			return &wrapper.Hooks, nil
		}
	}

	var cfg sdkconfig.HooksConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, []string{fmt.Sprintf("parse hooks config: %v", err)}
	}
	if hooksConfigEmpty(&cfg) {
		return nil, nil
	}
	return &cfg, nil
}

func hooksConfigEmpty(cfg *sdkconfig.HooksConfig) bool {
	if cfg == nil {
		return true
	}
	return len(cfg.PreToolUse) == 0 &&
		len(cfg.PostToolUse) == 0 &&
		len(cfg.PostToolUseFailure) == 0 &&
		len(cfg.PermissionRequest) == 0 &&
		len(cfg.SessionStart) == 0 &&
		len(cfg.SessionEnd) == 0 &&
		len(cfg.SubagentStart) == 0 &&
		len(cfg.SubagentStop) == 0 &&
		len(cfg.Stop) == 0 &&
		len(cfg.Notification) == 0 &&
		len(cfg.UserPromptSubmit) == 0 &&
		len(cfg.PreCompact) == 0
}

func hooksConfigToShellHooks(cfg *sdkconfig.HooksConfig, pluginName, pluginRoot, projectRoot string) []corehooks.ShellHook {
	if cfg == nil {
		return nil
	}

	env := map[string]string{}
	if strings.TrimSpace(projectRoot) != "" {
		env["CLAUDE_PROJECT_DIR"] = strings.TrimSpace(projectRoot)
	}
	if strings.TrimSpace(pluginRoot) != "" {
		env["CLAUDE_PLUGIN_ROOT"] = strings.TrimSpace(pluginRoot)
	}

	addEntries := func(event coreevents.EventType, entries []sdkconfig.HookMatcherEntry, prefix string) []corehooks.ShellHook {
		if len(entries) == 0 {
			return nil
		}
		var hooks []corehooks.ShellHook
		for _, entry := range entries {
			normalized := normalizeToolSelectorPattern(entry.Matcher)
			selector, err := corehooks.NewSelector(normalized, "")
			if err != nil {
				continue
			}
			for _, hookDef := range entry.Hooks {
				if hookDef.Type != "" && hookDef.Type != "command" {
					continue
				}
				if strings.TrimSpace(hookDef.Command) == "" {
					continue
				}
				timeout := time.Duration(0)
				if hookDef.Timeout > 0 {
					timeout = time.Duration(hookDef.Timeout) * time.Second
				}
				name := fmt.Sprintf("plugin:%s:%s:%s", pluginName, prefix, normalized)
				hooks = append(hooks, corehooks.ShellHook{
					Event:         event,
					Command:       hookDef.Command,
					Selector:      selector,
					Timeout:       timeout,
					Env:           cloneStringMap(env),
					Name:          name,
					Async:         hookDef.Async,
					Once:          hookDef.Once,
					StatusMessage: hookDef.StatusMessage,
				})
			}
		}
		return hooks
	}

	var hooks []corehooks.ShellHook
	hooks = append(hooks, addEntries(coreevents.PreToolUse, cfg.PreToolUse, "pre")...)
	hooks = append(hooks, addEntries(coreevents.PostToolUse, cfg.PostToolUse, "post")...)
	hooks = append(hooks, addEntries(coreevents.PostToolUseFailure, cfg.PostToolUseFailure, "post_failure")...)
	hooks = append(hooks, addEntries(coreevents.PermissionRequest, cfg.PermissionRequest, "permission")...)
	hooks = append(hooks, addEntries(coreevents.SessionStart, cfg.SessionStart, "session_start")...)
	hooks = append(hooks, addEntries(coreevents.SessionEnd, cfg.SessionEnd, "session_end")...)
	hooks = append(hooks, addEntries(coreevents.SubagentStart, cfg.SubagentStart, "subagent_start")...)
	hooks = append(hooks, addEntries(coreevents.SubagentStop, cfg.SubagentStop, "subagent_stop")...)
	hooks = append(hooks, addEntries(coreevents.Stop, cfg.Stop, "stop")...)
	hooks = append(hooks, addEntries(coreevents.Notification, cfg.Notification, "notification")...)
	hooks = append(hooks, addEntries(coreevents.UserPromptSubmit, cfg.UserPromptSubmit, "user_prompt")...)
	hooks = append(hooks, addEntries(coreevents.PreCompact, cfg.PreCompact, "pre_compact")...)
	return hooks
}

func injectPluginHookEnv(hooks []corehooks.ShellHook, pluginName, pluginRoot, projectRoot string) []corehooks.ShellHook {
	if len(hooks) == 0 {
		return hooks
	}

	out := make([]corehooks.ShellHook, 0, len(hooks))
	for _, hook := range hooks {
		env := map[string]string{}
		for k, v := range hook.Env {
			env[k] = v
		}
		if strings.TrimSpace(projectRoot) != "" {
			if _, exists := env["CLAUDE_PROJECT_DIR"]; !exists {
				env["CLAUDE_PROJECT_DIR"] = strings.TrimSpace(projectRoot)
			}
		}
		if strings.TrimSpace(pluginRoot) != "" {
			if _, exists := env["CLAUDE_PLUGIN_ROOT"]; !exists {
				env["CLAUDE_PLUGIN_ROOT"] = strings.TrimSpace(pluginRoot)
			}
		}
		hook.Env = env
		if hook.Name == "" {
			hook.Name = fmt.Sprintf("plugin:%s", pluginName)
		} else if !strings.HasPrefix(hook.Name, "plugin:") {
			hook.Name = fmt.Sprintf("plugin:%s:%s", pluginName, hook.Name)
		}
		out = append(out, hook)
	}
	return out
}

func parsePluginMCPInline(raw json.RawMessage, pluginName, pluginRoot, projectRoot string) (*AgentsConfig, []string) {
	if len(raw) == 0 {
		return nil, nil
	}

	servers, warnings := parsePluginMCPServers(raw, pluginName)
	if len(servers) == 0 {
		return nil, warnings
	}
	cfg := &AgentsConfig{MCPServers: map[string]MCPServerConfig{}}
	for name, srv := range servers {
		cfg.MCPServers[name] = convertPluginMCPServer(srv, pluginRoot, projectRoot)
	}
	return cfg.normalize(), warnings
}

func parsePluginMCPFile(path, pluginName, pluginRoot, projectRoot string) (*AgentsConfig, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("read mcp config %s: %v", path, err)}
	}

	servers, warnings := parsePluginMCPServers(data, pluginName)
	if len(servers) == 0 {
		return nil, warnings
	}
	cfg := &AgentsConfig{MCPServers: map[string]MCPServerConfig{}}
	for name, srv := range servers {
		cfg.MCPServers[name] = convertPluginMCPServer(srv, pluginRoot, projectRoot)
	}
	return cfg.normalize(), warnings
}

func parsePluginMCPServers(raw []byte, pluginName string) (map[string]pluginMCPServer, []string) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Prefer the wrapper form: {"mcpServers": {...}}. Even an empty wrapper is valid and should
	// return an empty server set (not be interpreted as a single server named "mcpServers").
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err == nil {
		if _, ok := keys["mcpServers"]; ok {
			var wrapper struct {
				MCPServers map[string]pluginMCPServer `json:"mcpServers"`
			}
			if err := json.Unmarshal(raw, &wrapper); err != nil {
				return nil, []string{fmt.Sprintf("plugin %s mcp servers: invalid json", pluginName)}
			}
			return wrapper.MCPServers, nil
		}
	}

	var wrapper struct {
		MCPServers map[string]pluginMCPServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.MCPServers) > 0 {
		return wrapper.MCPServers, nil
	}

	var direct map[string]pluginMCPServer
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	return nil, []string{fmt.Sprintf("plugin %s mcp servers: invalid json", pluginName)}
}

func convertPluginMCPServer(src pluginMCPServer, pluginRoot, projectRoot string) MCPServerConfig {
	env := cloneStringMap(src.Env)
	if strings.TrimSpace(pluginRoot) != "" {
		if env == nil {
			env = map[string]string{}
		}
		if _, exists := env["CLAUDE_PLUGIN_ROOT"]; !exists {
			env["CLAUDE_PLUGIN_ROOT"] = strings.TrimSpace(pluginRoot)
		}
	}
	if strings.TrimSpace(projectRoot) != "" {
		if env == nil {
			env = map[string]string{}
		}
		if _, exists := env["CLAUDE_PROJECT_DIR"]; !exists {
			env["CLAUDE_PROJECT_DIR"] = strings.TrimSpace(projectRoot)
		}
	}

	expandedEnv := map[string]string{}
	for k, v := range env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		expandedEnv[k] = expandPluginValue(v, pluginRoot, projectRoot)
	}
	if len(expandedEnv) == 0 {
		expandedEnv = nil
	}

	headers := map[string]string{}
	for k, v := range src.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headers[k] = expandPluginValue(v, pluginRoot, projectRoot)
	}
	if len(headers) == 0 {
		headers = nil
	}

	args := make([]string, 0, len(src.Args))
	for _, arg := range src.Args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		args = append(args, expandPluginValue(arg, pluginRoot, projectRoot))
	}

	timeoutPtr := (*int)(nil)
	if src.TimeoutSeconds > 0 {
		v := src.TimeoutSeconds
		timeoutPtr = &v
	}
	toolTimeoutPtr := (*int)(nil)
	if src.ToolTimeoutSeconds > 0 {
		v := src.ToolTimeoutSeconds
		toolTimeoutPtr = &v
	}

	typ := strings.TrimSpace(src.Type)
	if typ == "" {
		if strings.TrimSpace(src.Command) != "" {
			typ = "stdio"
		} else if strings.TrimSpace(src.URL) != "" {
			typ = "http"
		}
	}

	return MCPServerConfig{
		Type:              typ,
		Command:           strings.TrimSpace(expandPluginValue(src.Command, pluginRoot, projectRoot)),
		Args:              args,
		URL:               strings.TrimSpace(expandPluginValue(src.URL, pluginRoot, projectRoot)),
		Env:               expandedEnv,
		HTTPHeaders:       headers,
		EnabledTools:      append([]string(nil), src.EnabledTools...),
		DisabledTools:     append([]string(nil), src.DisabledTools...),
		StartupTimeoutSec: timeoutPtr,
		ToolTimeoutSec:    toolTimeoutPtr,
	}
}

func expandPluginValue(value, pluginRoot, projectRoot string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return os.Expand(value, func(key string) string {
		switch strings.TrimSpace(key) {
		case "CLAUDE_PLUGIN_ROOT":
			return strings.TrimSpace(pluginRoot)
		case "CLAUDE_PROJECT_DIR":
			return strings.TrimSpace(projectRoot)
		default:
			return os.Getenv(key)
		}
	})
}

func normalizeToolSelectorPattern(pattern string) string {
	if strings.TrimSpace(pattern) == "*" {
		return ""
	}
	return strings.TrimSpace(pattern)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeComponentPath(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeComponentPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return "."
	}
	return cleaned
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info != nil && !info.IsDir()
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info != nil && info.IsDir()
}
