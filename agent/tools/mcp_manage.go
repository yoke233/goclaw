package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/extensions"
)

type RuntimeInvalidator func(ctx context.Context, agentID string) error

type mcpServerView struct {
	Name           string            `json:"name"`
	Enabled        bool              `json:"enabled"`
	Type           string            `json:"type"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	URL            string            `json:"url,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

type mcpListResult struct {
	Success bool           `json:"success"`
	Path    string         `json:"path"`
	Servers []mcpServerView `json:"servers"`
	Message string         `json:"message,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type mcpPutServerResult struct {
	Success   bool         `json:"success"`
	Path      string       `json:"path"`
	Server    mcpServerView `json:"server"`
	Reloaded  bool         `json:"reloaded"`
	Message   string       `json:"message,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type mcpDeleteServerResult struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	Deleted  bool   `json:"deleted"`
	Reloaded bool   `json:"reloaded"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

type mcpSetEnabledResult struct {
	Success  bool         `json:"success"`
	Path     string       `json:"path"`
	Server   mcpServerView `json:"server"`
	Reloaded bool         `json:"reloaded"`
	Message  string       `json:"message,omitempty"`
	Error    string       `json:"error,omitempty"`
}

func NewMCPListTool(workspaceDir string) *BaseTool {
	cfgPath := extensions.MCPConfigPath(workspaceDir)
	return NewBaseTool(
		"mcp_list",
		"List MCP servers configured for this workspace (from .goclaw/mcp.json).",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			_ = params

			cfg, err := extensions.LoadMCPConfig(cfgPath)
			if err != nil {
				out, _ := json.Marshal(mcpListResult{
					Success: false,
					Path:    cfgPath,
					Error:   err.Error(),
				})
				return string(out), nil
			}

			names := make([]string, 0, len(cfg.Servers))
			for name := range cfg.Servers {
				names = append(names, name)
			}
			sort.Strings(names)

			servers := make([]mcpServerView, 0, len(names))
			for _, name := range names {
				s := cfg.Servers[name]
				servers = append(servers, mcpServerView{
					Name:           name,
					Enabled:        s.Enabled,
					Type:           s.Type,
					Command:        s.Command,
					Args:           append([]string(nil), s.Args...),
					URL:            s.URL,
					Env:            cloneStringMap(s.Env),
					Headers:        cloneStringMap(s.Headers),
					TimeoutSeconds: s.TimeoutSeconds,
				})
			}

			out, _ := json.Marshal(mcpListResult{
				Success: true,
				Path:    cfgPath,
				Servers: servers,
				Message: fmt.Sprintf("found %d MCP servers", len(servers)),
			})
			return string(out), nil
		},
	)
}

func NewMCPPutServerTool(workspaceDir string, invalidate RuntimeInvalidator) *BaseTool {
	cfgPath := extensions.MCPConfigPath(workspaceDir)
	return NewBaseTool(
		"mcp_put_server",
		"Create or update an MCP server entry in .goclaw/mcp.json, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Server name used for tool namespacing (e.g. 'linear', 'time').",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether this server is enabled.",
					"default":     true,
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Transport type: stdio|http|sse.",
				},
				"command": map[string]interface{}{
					"type":        "string",
					"description": "For stdio servers: executable name/path.",
				},
				"args": map[string]interface{}{
					"type":        "array",
					"description": "For stdio servers: argv list (without command).",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "For http/sse servers: endpoint URL.",
				},
				"env": map[string]interface{}{
					"type":        "object",
					"description": "Optional environment variables passed to the MCP server transport.",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"headers": map[string]interface{}{
					"type":        "object",
					"description": "Optional headers for http/sse transports.",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"timeoutSeconds": map[string]interface{}{
					"type":        "integer",
					"description": "Optional per-transport timeout in seconds.",
				},
			},
			"required": []string{"name", "type"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			name := strings.TrimSpace(asString(params["name"]))
			if name == "" {
				return marshalMCPError(cfgPath, "name is required"), nil
			}
			if !isSafeIdent(name) {
				return marshalMCPError(cfgPath, "invalid name (allowed: a-zA-Z0-9_-; must not start with '-' or '_')"), nil
			}

			enabled := true
			if v, ok := params["enabled"].(bool); ok {
				enabled = v
			}

			typ := strings.ToLower(strings.TrimSpace(asString(params["type"])))
			switch typ {
			case "stdio", "http", "sse":
			default:
				return marshalMCPError(cfgPath, "type must be one of: stdio|http|sse"), nil
			}

			command := strings.TrimSpace(asString(params["command"]))
			url := strings.TrimSpace(asString(params["url"]))
			args := asStringSlice(params["args"])
			env := asStringMap(params["env"])
			headers := asStringMap(params["headers"])
			timeoutSeconds := asInt(params["timeoutSeconds"])
			if timeoutSeconds < 0 {
				return marshalMCPError(cfgPath, "timeoutSeconds must be >= 0"), nil
			}

			if typ == "stdio" {
				if command == "" {
					return marshalMCPError(cfgPath, "command is required for stdio servers"), nil
				}
			} else {
				if url == "" {
					return marshalMCPError(cfgPath, "url is required for http/sse servers"), nil
				}
			}

			cfg, err := extensions.LoadMCPConfig(cfgPath)
			if err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
			}

			cfg.Servers[name] = extensions.MCPServer{
				Enabled:        enabled,
				Type:           typ,
				Command:        command,
				Args:           args,
				URL:            url,
				Env:            env,
				Headers:        headers,
				TimeoutSeconds: timeoutSeconds,
			}

			if err := extensions.SaveMCPConfig(cfgPath, cfg); err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
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

			out, _ := json.Marshal(mcpPutServerResult{
				Success:  true,
				Path:     cfgPath,
				Server:   toMCPView(name, cfg.Servers[name]),
				Reloaded: reloaded,
				Message:  "server saved",
			})
			return string(out), nil
		},
	)
}

func NewMCPDeleteServerTool(workspaceDir string, invalidate RuntimeInvalidator) *BaseTool {
	cfgPath := extensions.MCPConfigPath(workspaceDir)
	return NewBaseTool(
		"mcp_delete_server",
		"Delete an MCP server entry from .goclaw/mcp.json, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Server name to delete.",
				},
			},
			"required": []string{"name"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			name := strings.TrimSpace(asString(params["name"]))
			if name == "" {
				return marshalMCPError(cfgPath, "name is required"), nil
			}

			cfg, err := extensions.LoadMCPConfig(cfgPath)
			if err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
			}

			if _, ok := cfg.Servers[name]; !ok {
				out, _ := json.Marshal(mcpDeleteServerResult{
					Success: false,
					Path:    cfgPath,
					Deleted: false,
					Error:   "server not found",
				})
				return string(out), nil
			}

			delete(cfg.Servers, name)
			if err := extensions.SaveMCPConfig(cfgPath, cfg); err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
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

			out, _ := json.Marshal(mcpDeleteServerResult{
				Success:  true,
				Path:     cfgPath,
				Deleted:  true,
				Reloaded: reloaded,
				Message:  "server deleted",
			})
			return string(out), nil
		},
	)
}

func NewMCPSetEnabledTool(workspaceDir string, invalidate RuntimeInvalidator) *BaseTool {
	cfgPath := extensions.MCPConfigPath(workspaceDir)
	return NewBaseTool(
		"mcp_set_enabled",
		"Enable or disable an MCP server entry in .goclaw/mcp.json, then request runtime reload.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Server name.",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether this server should be enabled.",
				},
			},
			"required": []string{"name", "enabled"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			name := strings.TrimSpace(asString(params["name"]))
			if name == "" {
				return marshalMCPError(cfgPath, "name is required"), nil
			}
			enabled, ok := params["enabled"].(bool)
			if !ok {
				return marshalMCPError(cfgPath, "enabled must be boolean"), nil
			}

			cfg, err := extensions.LoadMCPConfig(cfgPath)
			if err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
			}

			srv, exists := cfg.Servers[name]
			if !exists {
				out, _ := json.Marshal(mcpSetEnabledResult{
					Success: false,
					Path:    cfgPath,
					Error:   "server not found",
				})
				return string(out), nil
			}

			srv.Enabled = enabled
			cfg.Servers[name] = srv
			if err := extensions.SaveMCPConfig(cfgPath, cfg); err != nil {
				return marshalMCPError(cfgPath, err.Error()), nil
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

			out, _ := json.Marshal(mcpSetEnabledResult{
				Success:  true,
				Path:     cfgPath,
				Server:   toMCPView(name, srv),
				Reloaded: reloaded,
				Message:  "server updated",
			})
			return string(out), nil
		},
	)
}

func toMCPView(name string, srv extensions.MCPServer) mcpServerView {
	return mcpServerView{
		Name:           name,
		Enabled:        srv.Enabled,
		Type:           srv.Type,
		Command:        srv.Command,
		Args:           append([]string(nil), srv.Args...),
		URL:            srv.URL,
		Env:            cloneStringMap(srv.Env),
		Headers:        cloneStringMap(srv.Headers),
		TimeoutSeconds: srv.TimeoutSeconds,
	}
}

func marshalMCPError(path string, msg string) string {
	out, _ := json.Marshal(mcpPutServerResult{
		Success: false,
		Path:    path,
		Error:   msg,
	})
	return string(out)
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

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	default:
		return 0
	}
}

func asStringSlice(v interface{}) []string {
	raw, ok := v.([]interface{})
	if !ok || len(raw) == 0 {
		// Some providers pass []string directly.
		if ss, ok := v.([]string); ok {
			out := make([]string, 0, len(ss))
			for _, s := range ss {
				if strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s := strings.TrimSpace(asString(item))
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func asStringMap(v interface{}) map[string]string {
	raw, ok := v.(map[string]interface{})
	if !ok || len(raw) == 0 {
		if m, ok := v.(map[string]string); ok && len(m) > 0 {
			out := make(map[string]string, len(m))
			for k, val := range m {
				out[k] = val
			}
			return out
		}
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, item := range raw {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = asString(item)
	}
	return out
}

func isSafeIdent(name string) bool {
	// Keep it intentionally strict: alnum + '-' + '_' and must start with alnum.
	if name == "" {
		return false
	}
	for i, r := range name {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_'
		if !ok {
			return false
		}
		if i == 0 && (r == '-' || r == '_') {
			return false
		}
	}
	return true
}

