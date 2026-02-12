package runtime

import "strings"

const (
	RoleFrontend = "frontend"
	RoleBackend  = "backend"
)

// NormalizeRole 规范化角色名；未知角色回退为 backend。
func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleFrontend:
		return RoleFrontend
	case RoleBackend:
		return RoleBackend
	default:
		return RoleBackend
	}
}

// ParseRole 按 label 优先、task 次之解析角色。
func ParseRole(task, label string) string {
	if role := parseRolePrefix(label); role != "" {
		return role
	}
	if role := parseRolePrefix(task); role != "" {
		return role
	}
	return RoleBackend
}

// StripRolePrefix 移除任务开头的角色标记，避免把标记透传给执行器。
func StripRolePrefix(task string) string {
	trimmed := strings.TrimSpace(task)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "[frontend]"):
		return strings.TrimSpace(trimmed[len("[frontend]"):])
	case strings.HasPrefix(lower, "[backend]"):
		return strings.TrimSpace(trimmed[len("[backend]"):])
	default:
		return task
	}
}

func parseRolePrefix(text string) string {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(trimmed, "[frontend]"):
		return RoleFrontend
	case strings.HasPrefix(trimmed, "[backend]"):
		return RoleBackend
	default:
		return ""
	}
}
