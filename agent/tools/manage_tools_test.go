package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/extensions"
)

func TestSkillsCRUDAndEnableDisable(t *testing.T) {
	workspace := t.TempDir()
	ctx := context.WithValue(context.Background(), agentruntime.CtxAgentID, "default")

	invalidateCalls := 0
	invalidate := RuntimeInvalidator(func(ctx context.Context, agentID string) error {
		_ = ctx
		_ = agentID
		invalidateCalls++
		return nil
	})

	putTool := NewSkillsPutTool(workspace, "skills", invalidate)
	putOut, err := putTool.Execute(ctx, map[string]interface{}{
		"role":       "main",
		"skill_name": "demo",
		"skill_md":   "---\nname: demo\ndescription: test\n---\n# Demo\n",
		"enabled":    true,
	})
	if err != nil {
		t.Fatalf("skills_put: %v", err)
	}
	var putRes struct {
		Success   bool   `json:"success"`
		Dir       string `json:"dir"`
		SkillFile string `json:"skill_file"`
		Enabled   bool   `json:"enabled"`
		Reloaded  bool   `json:"reloaded"`
		Error     string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(putOut), &putRes); err != nil {
		t.Fatalf("unmarshal put: %v", err)
	}
	if !putRes.Success || !putRes.Enabled || !putRes.Reloaded || putRes.Error != "" {
		t.Fatalf("unexpected put result: %+v", putRes)
	}

	if want := filepath.Join(workspace, "skills", "main", ".agents", "skills", "demo"); putRes.Dir != want {
		t.Fatalf("dir=%s, want %s", putRes.Dir, want)
	}
	if want := filepath.Join(putRes.Dir, "SKILL.md"); putRes.SkillFile != want {
		t.Fatalf("skill_file=%s, want %s", putRes.SkillFile, want)
	}
	if !fileExists(putRes.SkillFile) {
		t.Fatalf("expected skill file to exist: %s", putRes.SkillFile)
	}

	listTool := NewSkillsListTool(workspace, "skills")
	listOut, err := listTool.Execute(ctx, map[string]interface{}{"role": "main"})
	if err != nil {
		t.Fatalf("skills_list: %v", err)
	}
	var listRes struct {
		Success bool `json:"success"`
		Skills  []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"skills"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(listOut), &listRes); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if !listRes.Success || listRes.Error != "" {
		t.Fatalf("unexpected list result: %+v", listRes)
	}
	if len(listRes.Skills) != 1 || listRes.Skills[0].Name != "demo" || !listRes.Skills[0].Enabled {
		t.Fatalf("unexpected list skills: %+v", listRes.Skills)
	}

	disableTool := NewSkillsSetEnabledTool(workspace, "skills", invalidate)
	disableOut, err := disableTool.Execute(ctx, map[string]interface{}{"role": "main", "skill_name": "demo", "enabled": false})
	if err != nil {
		t.Fatalf("skills_set_enabled: %v", err)
	}
	var disableRes struct {
		Success  bool   `json:"success"`
		Enabled  bool   `json:"enabled"`
		Reloaded bool   `json:"reloaded"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(disableOut), &disableRes); err != nil {
		t.Fatalf("unmarshal disable: %v", err)
	}
	if !disableRes.Success || disableRes.Enabled || !disableRes.Reloaded || disableRes.Error != "" {
		t.Fatalf("unexpected disable result: %+v", disableRes)
	}

	// List without disabled should exclude it.
	listOut2, err := listTool.Execute(ctx, map[string]interface{}{"role": "main", "include_disabled": false})
	if err != nil {
		t.Fatalf("skills_list include_disabled=false: %v", err)
	}
	if err := json.Unmarshal([]byte(listOut2), &listRes); err != nil {
		t.Fatalf("unmarshal list2: %v", err)
	}
	if !listRes.Success || listRes.Error != "" {
		t.Fatalf("unexpected list2 result: %+v", listRes)
	}
	if len(listRes.Skills) != 0 {
		t.Fatalf("expected no enabled skills, got: %+v", listRes.Skills)
	}

	deleteTool := NewSkillsDeleteTool(workspace, "skills", invalidate)
	deleteOut, err := deleteTool.Execute(ctx, map[string]interface{}{"role": "main", "skill_name": "demo"})
	if err != nil {
		t.Fatalf("skills_delete: %v", err)
	}
	var delRes struct {
		Success  bool   `json:"success"`
		Deleted  bool   `json:"deleted"`
		Reloaded bool   `json:"reloaded"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(deleteOut), &delRes); err != nil {
		t.Fatalf("unmarshal delete: %v", err)
	}
	if !delRes.Success || !delRes.Deleted || !delRes.Reloaded || delRes.Error != "" {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}

	if invalidateCalls < 3 {
		t.Fatalf("expected invalidate to be called (>=3), got %d", invalidateCalls)
	}
}

func TestMCPCrudAndEnableDisable(t *testing.T) {
	workspace := t.TempDir()
	ctx := context.WithValue(context.Background(), agentruntime.CtxAgentID, "default")

	invalidateCalls := 0
	invalidate := RuntimeInvalidator(func(ctx context.Context, agentID string) error {
		_ = ctx
		_ = agentID
		invalidateCalls++
		return nil
	})

	putTool := NewMCPPutServerTool(workspace, "skills", invalidate)
	putOut, err := putTool.Execute(ctx, map[string]interface{}{
		"name":           "time",
		"enabled":        true,
		"type":           "stdio",
		"command":        "uvx",
		"args":           []interface{}{"mcp-server-time"},
		"timeoutSeconds": 10,
	})
	if err != nil {
		t.Fatalf("mcp_put_server: %v", err)
	}
	var putRes struct {
		Success  bool   `json:"success"`
		Reloaded bool   `json:"reloaded"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(putOut), &putRes); err != nil {
		t.Fatalf("unmarshal put: %v", err)
	}
	if !putRes.Success || !putRes.Reloaded || putRes.Error != "" {
		t.Fatalf("unexpected put result: %+v", putRes)
	}

	cfgPath := extensions.AgentsConfigPath(workspace)
	cfg, err := extensions.LoadAgentsConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadAgentsConfig: %v", err)
	}
	if cfg == nil || len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %+v", cfg)
	}

	listTool := NewMCPListTool(workspace, "skills")
	listOut, err := listTool.Execute(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("mcp_list: %v", err)
	}
	var listRes struct {
		Success bool `json:"success"`
		Servers []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
			Type    string `json:"type"`
		} `json:"servers"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(listOut), &listRes); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if !listRes.Success || listRes.Error != "" {
		t.Fatalf("unexpected list result: %+v", listRes)
	}
	if len(listRes.Servers) != 1 || listRes.Servers[0].Name != "time" || !listRes.Servers[0].Enabled {
		t.Fatalf("unexpected list servers: %+v", listRes.Servers)
	}

	setEnabledTool := NewMCPSetEnabledTool(workspace, "skills", invalidate)
	disableOut, err := setEnabledTool.Execute(ctx, map[string]interface{}{"name": "time", "enabled": false})
	if err != nil {
		t.Fatalf("mcp_set_enabled: %v", err)
	}
	var disableRes struct {
		Success bool `json:"success"`
		Server  struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"server"`
		Reloaded bool   `json:"reloaded"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(disableOut), &disableRes); err != nil {
		t.Fatalf("unmarshal disable: %v", err)
	}
	if !disableRes.Success || disableRes.Server.Name != "time" || disableRes.Server.Enabled || !disableRes.Reloaded || disableRes.Error != "" {
		t.Fatalf("unexpected disable result: %+v", disableRes)
	}

	deleteTool := NewMCPDeleteServerTool(workspace, "skills", invalidate)
	deleteOut, err := deleteTool.Execute(ctx, map[string]interface{}{"name": "time"})
	if err != nil {
		t.Fatalf("mcp_delete_server: %v", err)
	}
	var delRes struct {
		Success  bool   `json:"success"`
		Deleted  bool   `json:"deleted"`
		Reloaded bool   `json:"reloaded"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(deleteOut), &delRes); err != nil {
		t.Fatalf("unmarshal delete: %v", err)
	}
	if !delRes.Success || !delRes.Deleted || !delRes.Reloaded || delRes.Error != "" {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}

	cfg2, err := extensions.LoadAgentsConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadAgentsConfig after delete: %v", err)
	}
	if cfg2 == nil || len(cfg2.MCPServers) != 0 {
		t.Fatalf("expected 0 servers, got %+v", cfg2)
	}

	if invalidateCalls < 3 {
		t.Fatalf("expected invalidate to be called (>=3), got %d", invalidateCalls)
	}
}

func TestSkillsScopeWorkspaceAndRepo(t *testing.T) {
	workspace := t.TempDir()
	ctx := context.WithValue(context.Background(), agentruntime.CtxAgentID, "default")

	invalidate := RuntimeInvalidator(func(ctx context.Context, agentID string) error {
		_ = ctx
		_ = agentID
		return nil
	})

	// workspace scope: <workspace>/.agents/skills/<skill>
	putTool := NewSkillsPutTool(workspace, "skills", invalidate)
	putOut, err := putTool.Execute(ctx, map[string]interface{}{
		"scope":      "workspace",
		"skill_name": "w1",
		"skill_md":   "---\nname: w1\ndescription: test\n---\n# W1\n",
		"enabled":    true,
	})
	if err != nil {
		t.Fatalf("skills_put workspace: %v", err)
	}
	var putRes struct {
		Success bool   `json:"success"`
		Scope   string `json:"scope"`
		Dir     string `json:"dir"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(putOut), &putRes); err != nil {
		t.Fatalf("unmarshal put workspace: %v", err)
	}
	if !putRes.Success || putRes.Scope != "workspace" || putRes.Error != "" {
		t.Fatalf("unexpected put workspace result: %+v", putRes)
	}
	if want := filepath.Join(workspace, ".agents", "skills", "w1"); putRes.Dir != want {
		t.Fatalf("workspace dir=%s, want %s", putRes.Dir, want)
	}

	// repo scope: <repo>/.agents/skills/<skill> (repo_dir must be within workspace)
	repoDir := filepath.Join(workspace, "repos", "demo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repoDir: %v", err)
	}
	putOut2, err := putTool.Execute(ctx, map[string]interface{}{
		"scope":      "repo",
		"repo_dir":   filepath.Join("repos", "demo"),
		"skill_name": "r1",
		"skill_md":   "---\nname: r1\ndescription: test\n---\n# R1\n",
		"enabled":    true,
	})
	if err != nil {
		t.Fatalf("skills_put repo: %v", err)
	}
	if err := json.Unmarshal([]byte(putOut2), &putRes); err != nil {
		t.Fatalf("unmarshal put repo: %v", err)
	}
	if !putRes.Success || putRes.Scope != "repo" || putRes.Error != "" {
		t.Fatalf("unexpected put repo result: %+v", putRes)
	}
	if want := filepath.Join(repoDir, ".agents", "skills", "r1"); putRes.Dir != want {
		t.Fatalf("repo dir=%s, want %s", putRes.Dir, want)
	}
}

func TestMCPScopeRoleAndRepo(t *testing.T) {
	workspace := t.TempDir()
	ctx := context.WithValue(context.Background(), agentruntime.CtxAgentID, "default")

	invalidate := RuntimeInvalidator(func(ctx context.Context, agentID string) error {
		_ = ctx
		_ = agentID
		return nil
	})

	putTool := NewMCPPutServerTool(workspace, "skills", invalidate)

	// role scope: <workspace>/<skills_role_dir>/<role>/.agents/config.toml
	putOut, err := putTool.Execute(ctx, map[string]interface{}{
		"scope":          "role",
		"role":           "frontend",
		"name":           "time",
		"enabled":        true,
		"type":           "stdio",
		"command":        "uvx",
		"args":           []interface{}{"mcp-server-time"},
		"timeoutSeconds": 10,
	})
	if err != nil {
		t.Fatalf("mcp_put_server role: %v", err)
	}
	var putRes struct {
		Success bool   `json:"success"`
		Scope   string `json:"scope"`
		Path    string `json:"path"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(putOut), &putRes); err != nil {
		t.Fatalf("unmarshal put role: %v", err)
	}
	if !putRes.Success || putRes.Scope != "role" || putRes.Error != "" {
		t.Fatalf("unexpected put role result: %+v", putRes)
	}

	roleRoot := filepath.Join(workspace, "skills", "frontend")
	cfgPath := extensions.AgentsConfigPath(roleRoot)
	cfg, err := extensions.LoadAgentsConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadAgentsConfig role: %v", err)
	}
	if cfg == nil || len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server in role config, got %+v", cfg)
	}

	// repo scope: <repo>/.agents/config.toml (repo_dir must be within workspace)
	repoDir := filepath.Join(workspace, "repos", "demo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repoDir: %v", err)
	}
	putOut2, err := putTool.Execute(ctx, map[string]interface{}{
		"scope":          "repo",
		"repo_dir":       filepath.Join("repos", "demo"),
		"name":           "figma",
		"enabled":        true,
		"type":           "http",
		"url":            "https://mcp.figma.com/mcp",
		"timeoutSeconds": 15,
	})
	if err != nil {
		t.Fatalf("mcp_put_server repo: %v", err)
	}
	if err := json.Unmarshal([]byte(putOut2), &putRes); err != nil {
		t.Fatalf("unmarshal put repo: %v", err)
	}
	if !putRes.Success || putRes.Scope != "repo" || putRes.Error != "" {
		t.Fatalf("unexpected put repo result: %+v", putRes)
	}

	cfg2Path := extensions.AgentsConfigPath(repoDir)
	cfg2, err := extensions.LoadAgentsConfig(cfg2Path)
	if err != nil {
		t.Fatalf("LoadAgentsConfig repo: %v", err)
	}
	if cfg2 == nil || len(cfg2.MCPServers) != 1 {
		t.Fatalf("expected 1 server in repo config, got %+v", cfg2)
	}
}
