package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellTool Shell 工具
type ShellTool struct {
	enabled       bool
	allowedCmds   []string
	deniedCmds    []string
	timeout       time.Duration
	workingDir    string
}

// NewShellTool 创建 Shell 工具
func NewShellTool(enabled bool, allowedCmds, deniedCmds []string, timeout int, workingDir string) *ShellTool {
	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = 120 * time.Second
	}

	return &ShellTool{
		enabled:     enabled,
		allowedCmds: allowedCmds,
		deniedCmds:  deniedCmds,
		timeout:     t,
		workingDir:  workingDir,
	}
}

// Exec 执行 Shell 命令
func (t *ShellTool) Exec(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("shell tool is disabled")
	}

	command, ok := params["command"].(string)
	if !ok {
		return "", fmt.Errorf("command parameter is required")
	}

	// 检查危险命令
	if t.isDenied(command) {
		return "", fmt.Errorf("command is not allowed: %s", command)
	}

	// 创建带超时的上下文
	cmdCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 执行命令
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	if t.workingDir != "" {
		cmd.Dir = t.workingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// isDenied 检查命令是否被拒绝
func (t *ShellTool) isDenied(command string) bool {
	// 检查明确拒绝的命令
	for _, denied := range t.deniedCmds {
		if strings.Contains(command, denied) {
			return true
		}
	}

	// 如果有允许列表，检查是否在允许列表中
	if len(t.allowedCmds) > 0 {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return true
		}
		cmdName := parts[0]

		for _, allowed := range t.allowedCmds {
			if cmdName == allowed {
				return false
			}
		}
		return true
	}

	return false
}

// GetTools 获取所有 Shell 工具
func (t *ShellTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"exec",
			"Execute a shell command on the host system. Use this for file operations, running scripts (Python, Node.js, etc.), installing dependencies, HTTP requests (curl), system diagnostics and more. Commands run in a non-interactive shell.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Shell command to execute",
					},
				},
				"required": []string{"command"},
			},
			t.Exec,
		),
	}
}
