package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
)

type skillListItem struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Enabled   bool   `json:"enabled"`
	Dir       string `json:"dir"`
	SkillFile string `json:"skill_file,omitempty"`
}

type skillsListResult struct {
	Success bool            `json:"success"`
	Role    string          `json:"role"`
	BaseDir string          `json:"base_dir"`
	Skills  []skillListItem `json:"skills"`
	Message string          `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type skillsGetResult struct {
	Success bool `json:"success"`
	Skill   struct {
		Name      string `json:"name"`
		Role      string `json:"role"`
		Enabled   bool   `json:"enabled"`
		Dir       string `json:"dir"`
		SkillFile string `json:"skill_file,omitempty"`
		Content   string `json:"content,omitempty"`
	} `json:"skill"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type skillsPutResult struct {
	Success   bool   `json:"success"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	SkillFile string `json:"skill_file"`
	Enabled   bool   `json:"enabled"`
	Reloaded  bool   `json:"reloaded"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

type skillsDeleteResult struct {
	Success  bool   `json:"success"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Deleted  bool   `json:"deleted"`
	Reloaded bool   `json:"reloaded"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

type skillsSetEnabledResult struct {
	Success  bool   `json:"success"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Enabled  bool   `json:"enabled"`
	Reloaded bool   `json:"reloaded"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

func NewSkillsListTool(workspaceDir, skillsRoleDir string) *BaseTool {
	return NewBaseTool(
		"skills_list",
		"List skills for a given role under the role pack .agents/skills directory.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"role": map[string]interface{}{
					"type":        "string",
					"description": "Role name. Defaults to 'main'.",
					"default":     "main",
				},
				"include_disabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to include disabled skills.",
					"default":     true,
				},
			},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			_ = ctx
			role := strings.TrimSpace(asString(params["role"]))
			if role == "" {
				role = "main"
			}
			if !isSafeIdent(role) {
				return marshalSkillsError(role, skillsRoleBaseDir(workspaceDir, skillsRoleDir, role), fmt.Sprintf("invalid role: %s", role)), nil
			}

			includeDisabled := true
			if v, ok := params["include_disabled"].(bool); ok {
				includeDisabled = v
			}

			roleDir := skillsRoleBaseDir(workspaceDir, skillsRoleDir, role)
			entries, err := os.ReadDir(roleDir)
			if err != nil {
				if os.IsNotExist(err) {
					out, _ := json.Marshal(skillsListResult{
						Success: true,
						Role:    role,
						BaseDir: roleDir,
						Skills:  nil,
						Message: "role directory does not exist (no skills)",
					})
					return string(out), nil
				}
				out, _ := json.Marshal(skillsListResult{
					Success: false,
					Role:    role,
					BaseDir: roleDir,
					Error:   err.Error(),
				})
				return string(out), nil
			}

			var skills []skillListItem
			for _, entry := range entries {
				if entry == nil || !entry.IsDir() {
					continue
				}
				name := strings.TrimSpace(entry.Name())
				if name == "" || strings.HasPrefix(name, ".") {
					continue
				}
				if !isSafeIdent(name) {
					continue
				}

				dir := filepath.Join(roleDir, name)
				enabled := !fileExists(filepath.Join(dir, ".disabled"))
				if !enabled && !includeDisabled {
					continue
				}

				skillFile := resolveSkillFile(dir)
				skills = append(skills, skillListItem{
					Name:      name,
					Role:      role,
					Enabled:   enabled,
					Dir:       dir,
					SkillFile: skillFile,
				})
			}

			sort.Slice(skills, func(i, j int) bool {
				return skills[i].Name < skills[j].Name
			})

			out, _ := json.Marshal(skillsListResult{
				Success: true,
				Role:    role,
				BaseDir: roleDir,
				Skills:  skills,
				Message: fmt.Sprintf("found %d skills", len(skills)),
			})
			return string(out), nil
		},
	)
}

func NewSkillsGetTool(workspaceDir, skillsRoleDir string) *BaseTool {
	return NewBaseTool(
		"skills_get",
		"Get a skill's SKILL.md content for a given role (from role pack .agents/skills).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"role": map[string]interface{}{
					"type":        "string",
					"description": "Role name. Defaults to 'main'.",
					"default":     "main",
				},
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "Skill directory name.",
				},
				"include_content": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to include SKILL.md content in the response.",
					"default":     true,
				},
			},
			"required": []string{"skill_name"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			_ = ctx
			role := strings.TrimSpace(asString(params["role"]))
			if role == "" {
				role = "main"
			}
			if !isSafeIdent(role) {
				return marshalSkillsError(role, skillsRoleBaseDir(workspaceDir, skillsRoleDir, role), fmt.Sprintf("invalid role: %s", role)), nil
			}

			name := strings.TrimSpace(asString(params["skill_name"]))
			if name == "" {
				return marshalSkillsError(role, skillsRoleBaseDir(workspaceDir, skillsRoleDir, role), "skill_name is required"), nil
			}
			if !isSafeIdent(name) {
				return marshalSkillsError(role, skillsRoleBaseDir(workspaceDir, skillsRoleDir, role), fmt.Sprintf("invalid skill_name: %s", name)), nil
			}

			includeContent := true
			if v, ok := params["include_content"].(bool); ok {
				includeContent = v
			}

			roleDir := skillsRoleBaseDir(workspaceDir, skillsRoleDir, role)
			skillDir := filepath.Join(roleDir, name)
			enabled := !fileExists(filepath.Join(skillDir, ".disabled"))
			skillFile := resolveSkillFile(skillDir)

			result := skillsGetResult{Success: true}
			result.Skill.Name = name
			result.Skill.Role = role
			result.Skill.Enabled = enabled
			result.Skill.Dir = skillDir
			result.Skill.SkillFile = skillFile

			if !dirExists(skillDir) {
				result.Success = false
				result.Error = "skill directory not found"
				out, _ := json.Marshal(result)
				return string(out), nil
			}

			if includeContent && skillFile != "" {
				data, err := os.ReadFile(skillFile)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
					out, _ := json.Marshal(result)
					return string(out), nil
				}
				result.Skill.Content = string(data)
			}

			out, _ := json.Marshal(result)
			return string(out), nil
		},
	)
}

func NewSkillsPutTool(workspaceDir, skillsRoleDir string, invalidate RuntimeInvalidator) *BaseTool {
	return NewBaseTool(
		"skills_put",
		"Create or update a skill (writes SKILL.md) under <workspace>/<skills_role_dir>/<role>/.agents/skills/<skill_name>, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"role": map[string]interface{}{
					"type":        "string",
					"description": "Role name. Defaults to 'main'.",
					"default":     "main",
				},
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "Skill directory name.",
				},
				"skill_md": map[string]interface{}{
					"type":        "string",
					"description": "Full content of SKILL.md (including frontmatter).",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the skill should be enabled after writing.",
					"default":     true,
				},
				"overwrite": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to overwrite existing SKILL.md if present.",
					"default":     true,
				},
			},
			"required": []string{"skill_name", "skill_md"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			role := strings.TrimSpace(asString(params["role"]))
			if role == "" {
				role = "main"
			}
			if !isSafeIdent(role) {
				return marshalSkillsPutError(role, "", fmt.Sprintf("invalid role: %s", role)), nil
			}

			name := strings.TrimSpace(asString(params["skill_name"]))
			if name == "" {
				return marshalSkillsPutError(role, "", "skill_name is required"), nil
			}
			if !isSafeIdent(name) {
				return marshalSkillsPutError(role, "", fmt.Sprintf("invalid skill_name: %s", name)), nil
			}

			md := asString(params["skill_md"])
			if strings.TrimSpace(md) == "" {
				return marshalSkillsPutError(role, "", "skill_md is empty"), nil
			}

			enabled := true
			if v, ok := params["enabled"].(bool); ok {
				enabled = v
			}

			overwrite := true
			if v, ok := params["overwrite"].(bool); ok {
				overwrite = v
			}

			roleDir := skillsRoleBaseDir(workspaceDir, skillsRoleDir, role)
			skillDir := filepath.Join(roleDir, name)
			skillFile := filepath.Join(skillDir, "SKILL.md")
			disabledFile := filepath.Join(skillDir, ".disabled")

			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return marshalSkillsPutError(role, skillDir, err.Error()), nil
			}

			if !overwrite && fileExists(skillFile) {
				return marshalSkillsPutError(role, skillDir, "SKILL.md already exists and overwrite=false"), nil
			}

			if err := os.WriteFile(skillFile, []byte(md), 0o644); err != nil {
				return marshalSkillsPutError(role, skillDir, err.Error()), nil
			}

			if enabled {
				_ = os.Remove(disabledFile)
			} else {
				_ = os.WriteFile(disabledFile, []byte("disabled\n"), 0o644)
			}

			reloaded := false
			if invalidate != nil {
				agentID := strings.TrimSpace(asString(ctx.Value(agentruntime.CtxAgentID)))
				if agentID == "" {
					agentID = "default"
				}
				if err := invalidate(ctx, agentID); err == nil {
					reloaded = true
				}
			}

			out, _ := json.Marshal(skillsPutResult{
				Success:   true,
				Role:      role,
				Name:      name,
				Dir:       skillDir,
				SkillFile: skillFile,
				Enabled:   enabled,
				Reloaded:  reloaded,
				Message:   "skill saved",
			})
			return string(out), nil
		},
	)
}

func NewSkillsDeleteTool(workspaceDir, skillsRoleDir string, invalidate RuntimeInvalidator) *BaseTool {
	return NewBaseTool(
		"skills_delete",
		"Delete a skill directory under <workspace>/<skills_role_dir>/<role>/.agents/skills/<skill_name>, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"role": map[string]interface{}{
					"type":        "string",
					"description": "Role name. Defaults to 'main'.",
					"default":     "main",
				},
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "Skill directory name.",
				},
			},
			"required": []string{"skill_name"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			role := strings.TrimSpace(asString(params["role"]))
			if role == "" {
				role = "main"
			}
			if !isSafeIdent(role) {
				return marshalSkillsDeleteError(role, "", fmt.Sprintf("invalid role: %s", role)), nil
			}

			name := strings.TrimSpace(asString(params["skill_name"]))
			if name == "" {
				return marshalSkillsDeleteError(role, "", "skill_name is required"), nil
			}
			if !isSafeIdent(name) {
				return marshalSkillsDeleteError(role, "", fmt.Sprintf("invalid skill_name: %s", name)), nil
			}

			roleDir := skillsRoleBaseDir(workspaceDir, skillsRoleDir, role)
			skillDir := filepath.Join(roleDir, name)
			if !dirExists(skillDir) {
				out, _ := json.Marshal(skillsDeleteResult{
					Success: false,
					Role:    role,
					Name:    name,
					Dir:     skillDir,
					Deleted: false,
					Error:   "skill directory not found",
				})
				return string(out), nil
			}

			if err := os.RemoveAll(skillDir); err != nil {
				return marshalSkillsDeleteError(role, skillDir, err.Error()), nil
			}

			reloaded := false
			if invalidate != nil {
				agentID := strings.TrimSpace(asString(ctx.Value(agentruntime.CtxAgentID)))
				if agentID == "" {
					agentID = "default"
				}
				if err := invalidate(ctx, agentID); err == nil {
					reloaded = true
				}
			}

			out, _ := json.Marshal(skillsDeleteResult{
				Success:  true,
				Role:     role,
				Name:     name,
				Dir:      skillDir,
				Deleted:  true,
				Reloaded: reloaded,
				Message:  "skill deleted",
			})
			return string(out), nil
		},
	)
}

func NewSkillsSetEnabledTool(workspaceDir, skillsRoleDir string, invalidate RuntimeInvalidator) *BaseTool {
	return NewBaseTool(
		"skills_set_enabled",
		"Enable or disable a skill (role pack .agents/skills) by toggling its .disabled file, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"role": map[string]interface{}{
					"type":        "string",
					"description": "Role name. Defaults to 'main'.",
					"default":     "main",
				},
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "Skill directory name.",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the skill should be enabled.",
				},
			},
			"required": []string{"skill_name", "enabled"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			role := strings.TrimSpace(asString(params["role"]))
			if role == "" {
				role = "main"
			}
			if !isSafeIdent(role) {
				return marshalSkillsSetEnabledError(role, "", fmt.Sprintf("invalid role: %s", role)), nil
			}

			name := strings.TrimSpace(asString(params["skill_name"]))
			if name == "" {
				return marshalSkillsSetEnabledError(role, "", "skill_name is required"), nil
			}
			if !isSafeIdent(name) {
				return marshalSkillsSetEnabledError(role, "", fmt.Sprintf("invalid skill_name: %s", name)), nil
			}

			enabled, ok := params["enabled"].(bool)
			if !ok {
				return marshalSkillsSetEnabledError(role, "", "enabled must be boolean"), nil
			}

			roleDir := skillsRoleBaseDir(workspaceDir, skillsRoleDir, role)
			skillDir := filepath.Join(roleDir, name)
			if !dirExists(skillDir) {
				return marshalSkillsSetEnabledError(role, skillDir, "skill directory not found"), nil
			}

			disabledFile := filepath.Join(skillDir, ".disabled")
			if enabled {
				_ = os.Remove(disabledFile)
			} else {
				_ = os.WriteFile(disabledFile, []byte("disabled\n"), 0o644)
			}

			reloaded := false
			if invalidate != nil {
				agentID := strings.TrimSpace(asString(ctx.Value(agentruntime.CtxAgentID)))
				if agentID == "" {
					agentID = "default"
				}
				if err := invalidate(ctx, agentID); err == nil {
					reloaded = true
				}
			}

			out, _ := json.Marshal(skillsSetEnabledResult{
				Success:  true,
				Role:     role,
				Name:     name,
				Dir:      skillDir,
				Enabled:  enabled,
				Reloaded: reloaded,
				Message:  "skill updated",
			})
			return string(out), nil
		},
	)
}

func skillsRoleBaseDir(workspaceDir, skillsRoleDir, role string) string {
	base := strings.TrimSpace(skillsRoleDir)
	if base == "" {
		base = "skills"
	}
	role = strings.TrimSpace(role)
	if role == "" {
		role = "main"
	}
	roleRoot := filepath.Join(workspaceDir, base, role)
	return filepath.Join(roleRoot, ".agents", "skills")
}

func resolveSkillFile(skillDir string) string {
	candidates := []string{
		filepath.Join(skillDir, "SKILL.md"),
		filepath.Join(skillDir, "skill.md"),
	}
	for _, path := range candidates {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func marshalSkillsError(role, baseDir, msg string) string {
	out, _ := json.Marshal(skillsListResult{
		Success: false,
		Role:    role,
		BaseDir: baseDir,
		Error:   msg,
	})
	return string(out)
}

func marshalSkillsPutError(role, dir, msg string) string {
	out, _ := json.Marshal(skillsPutResult{
		Success: false,
		Role:    role,
		Dir:     dir,
		Error:   msg,
	})
	return string(out)
}

func marshalSkillsDeleteError(role, dir, msg string) string {
	out, _ := json.Marshal(skillsDeleteResult{
		Success: false,
		Role:    role,
		Dir:     dir,
		Error:   msg,
	})
	return string(out)
}

func marshalSkillsSetEnabledError(role, dir, msg string) string {
	out, _ := json.Marshal(skillsSetEnabledResult{
		Success: false,
		Role:    role,
		Dir:     dir,
		Error:   msg,
	})
	return string(out)
}
