package runtime

// CtxKey 用于隔离 context key，避免字符串冲突。
type CtxKey string

const (
	CtxSessionKey CtxKey = "goclaw.session_key"
	CtxAgentID    CtxKey = "goclaw.agent_id"
	CtxChannel    CtxKey = "goclaw.channel"
	CtxAccountID  CtxKey = "goclaw.account_id"
	CtxChatID     CtxKey = "goclaw.chat_id"
)
