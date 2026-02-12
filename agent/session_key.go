package agent

import (
	"fmt"
	"strings"
	"time"
)

// SessionKeyOptions controls how a session key is generated.
type SessionKeyOptions struct {
	Explicit       string
	Channel        string
	AccountID      string
	ChatID         string
	FreshOnDefault bool
	Now            time.Time
}

// ResolveSessionKey generates a normalized session key and reports whether it is a fresh key.
func ResolveSessionKey(opts SessionKeyOptions) (string, bool) {
	explicit := strings.TrimSpace(opts.Explicit)
	if explicit != "" {
		return explicit, false
	}

	channel := strings.TrimSpace(opts.Channel)
	if channel == "" {
		channel = "cli"
	}

	accountID := strings.TrimSpace(opts.AccountID)
	if accountID == "" {
		accountID = "default"
	}

	chatID := strings.TrimSpace(opts.ChatID)
	if chatID == "" {
		chatID = "default"
	}

	if opts.FreshOnDefault && strings.EqualFold(chatID, "default") {
		now := opts.Now
		if now.IsZero() {
			now = time.Now()
		}
		return fmt.Sprintf("%s:%s:%d", channel, accountID, now.Unix()), true
	}

	return fmt.Sprintf("%s:%s:%s", channel, accountID, chatID), false
}
