package gateway

// SessionNotifier sends notifications to a specific session.
type SessionNotifier interface {
	Notify(sessionID string, method string, data interface{}) error
}
