package runtime

import "strings"

const (
	RoleFrontend = "frontend"
	RoleBackend  = "backend"
)

// NormalizeRole 规范化角色名；空或非法值回退为 backend。
func NormalizeRole(role string) string {
	if normalized, ok := normalizeRoleToken(role); ok {
		return normalized
	}
	return RoleBackend
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
	_, remaining, ok := parseRolePrefixWithRemainder(task)
	if !ok {
		return task
	}
	return remaining
}

func parseRolePrefix(text string) string {
	role, _, ok := parseRolePrefixWithRemainder(text)
	if !ok {
		return ""
	}
	return role
}

func parseRolePrefixWithRemainder(text string) (role string, remaining string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "[") {
		return "", "", false
	}

	end := strings.Index(trimmed, "]")
	if end <= 1 {
		return "", "", false
	}

	token := trimmed[1:end]
	normalized, valid := normalizeRoleToken(token)
	if !valid {
		return "", "", false
	}

	remaining = strings.TrimSpace(trimmed[end+1:])
	return normalized, remaining, true
}

func normalizeRoleToken(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", false
	}
	for _, ch := range normalized {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return "", false
	}
	return normalized, true
}
