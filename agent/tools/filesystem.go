package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileSystemTool 文件系统工具
type FileSystemTool struct {
	allowedPaths []string
	deniedPaths  []string
}

// NewFileSystemTool 创建文件系统工具
func NewFileSystemTool(allowedPaths, deniedPaths []string) *FileSystemTool {
	return &FileSystemTool{
		allowedPaths: allowedPaths,
		deniedPaths:  deniedPaths,
	}
}

// ReadFile 读取文件
func (t *FileSystemTool) ReadFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// WriteFile 写入文件
func (t *FileSystemTool) WriteFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// ListDir 列出目录
func (t *FileSystemTool) ListDir(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var result []string
	for _, entry := range entries {
		info := ""
		if entry.IsDir() {
			info = "[DIR] "
		}
		result = append(result, info+entry.Name())
	}

	return strings.Join(result, "\n"), nil
}

// isAllowed 检查路径是否允许访问
func (t *FileSystemTool) isAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 检查拒绝列表
	for _, denied := range t.deniedPaths {
		if strings.HasPrefix(absPath, denied) {
			return false
		}
	}

	// 如果没有允许列表，允许所有路径
	if len(t.allowedPaths) == 0 {
		return true
	}

	// 检查允许列表
	for _, allowed := range t.allowedPaths {
		if strings.HasPrefix(absPath, allowed) {
			return true
		}
	}

	return false
}

// GetTools 获取所有文件系统工具
func (t *FileSystemTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"read_file",
			"Read the contents of a file",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to read",
					},
				},
				"required": []string{"path"},
			},
			t.ReadFile,
		),
		NewBaseTool(
			"write_file",
			"Write content to a file",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to write",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"path", "content"},
			},
			t.WriteFile,
		),
		NewBaseTool(
			"list_dir",
			"List contents of a directory",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the directory",
					},
				},
				"required": []string{"path"},
			},
			t.ListDir,
		),
	}
}
