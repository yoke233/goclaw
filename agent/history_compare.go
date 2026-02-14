package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdkmessage "github.com/cexll/agentsdk-go/pkg/message"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

const defaultAgentsdkHistoryCleanupDays = 7

// AgentHistoryMode controls how agent history is persisted.
const (
	HistoryModeSessionOnly  = "session_only"
	HistoryModeDual         = "dual"
	HistoryModeAgentsdkOnly = "agentsdk_only"
)

type persistedHistory struct {
	Version   int                  `json:"version"`
	SessionID string               `json:"session_id,omitempty"`
	UpdatedAt time.Time            `json:"updated_at,omitempty"`
	Messages  []sdkmessage.Message `json:"messages,omitempty"`
}

// ShouldPersistAgentSDKHistory reports whether agentsdk-go history should be enabled.
func ShouldPersistAgentSDKHistory(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Agents.Defaults.History.Mode))
	if mode == "" {
		mode = HistoryModeSessionOnly
	}
	if mode != HistoryModeSessionOnly {
		return true
	}
	return cfg.Agents.Defaults.History.Compare
}

// AgentSDKHistoryCleanupDays returns the configured cleanup days or a default.
func AgentSDKHistoryCleanupDays(cfg *config.Config) int {
	if cfg == nil {
		return defaultAgentsdkHistoryCleanupDays
	}
	if cfg.Agents.Defaults.History.AgentsdkCleanupDays > 0 {
		return cfg.Agents.Defaults.History.AgentsdkCleanupDays
	}
	return defaultAgentsdkHistoryCleanupDays
}

// CompareSessionHistory compares session history with agentsdk-go persisted history.
// Returns true if histories appear consistent. Warnings are logged on mismatch.
func CompareSessionHistory(cfg *config.Config, sess *session.Session, workspace string) bool {
	if cfg == nil || sess == nil {
		return true
	}
	if !cfg.Agents.Defaults.History.Compare {
		return true
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return true
	}
	sdkMsgs, err := loadAgentSDKHistory(sess.Key, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		logger.Warn("AgentSDK history compare failed",
			zap.String("session", sess.Key),
			zap.String("workspace", workspace),
			zap.Error(err))
		return false
	}
	sessionMsgs := sess.GetHistory(0)
	ok, reason := compareHistory(sessionMsgs, sdkMsgs)
	if !ok {
		logger.Warn("AgentSDK history mismatch",
			zap.String("session", sess.Key),
			zap.String("workspace", workspace),
			zap.String("reason", reason))
	}
	return ok
}

func compareHistory(sessionMsgs []session.Message, sdkMsgs []sdkmessage.Message) (bool, string) {
	sessionFiltered := filterSessionMessages(sessionMsgs)
	sdkFiltered := filterSDKMessages(sdkMsgs)

	if len(sessionFiltered) == 0 && len(sdkFiltered) == 0 {
		return true, ""
	}

	compareCount := 2
	if len(sessionFiltered) < compareCount {
		compareCount = len(sessionFiltered)
	}
	if len(sdkFiltered) < compareCount {
		compareCount = len(sdkFiltered)
	}
	if compareCount == 0 {
		return true, ""
	}

	for i := 0; i < compareCount; i++ {
		a := sessionFiltered[len(sessionFiltered)-1-i]
		b := sdkFiltered[len(sdkFiltered)-1-i]
		if !strings.EqualFold(strings.TrimSpace(a.Role), strings.TrimSpace(b.Role)) {
			return false, fmt.Sprintf("role mismatch at -%d: session=%s sdk=%s", i+1, a.Role, b.Role)
		}
		if strings.TrimSpace(a.Content) != strings.TrimSpace(b.Content) {
			return false, fmt.Sprintf("content mismatch at -%d", i+1)
		}
	}
	return true, ""
}

func filterSessionMessages(msgs []session.Message) []session.Message {
	out := make([]session.Message, 0, len(msgs))
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "user", "assistant":
			out = append(out, msg)
		}
	}
	return out
}

func filterSDKMessages(msgs []sdkmessage.Message) []sdkmessage.Message {
	out := make([]sdkmessage.Message, 0, len(msgs))
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "user", "assistant":
			out = append(out, msg)
		}
	}
	return out
}

func loadAgentSDKHistory(sessionKey, workspace string) ([]sdkmessage.Message, error) {
	path := agentSDKHistoryPath(sessionKey, workspace)
	if path == "" {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wrapper persistedHistory
	if err := json.Unmarshal(data, &wrapper); err == nil {
		if wrapper.Version != 0 || wrapper.SessionID != "" || !wrapper.UpdatedAt.IsZero() || wrapper.Messages != nil {
			return sdkmessage.CloneMessages(wrapper.Messages), nil
		}
	}

	var msgs []sdkmessage.Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("decode agentsdk history: %w", err)
	}
	return sdkmessage.CloneMessages(msgs), nil
}

func agentSDKHistoryPath(sessionKey, workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	name := sanitizePathComponent(sessionKey)
	if name == "" {
		return ""
	}
	return filepath.Join(workspace, ".claude", "history", name+".json")
}

func sanitizePathComponent(value string) string {
	const fallback = "default"
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	sanitized := strings.Trim(b.String(), "-")
	if sanitized == "" {
		return fallback
	}
	return sanitized
}
