package agent

import "context"

// MainRunMedia represents an optional media attachment for a user turn.
type MainRunMedia struct {
	Type     string
	URL      string
	Base64   string
	MimeType string
}

// MainRunRequest describes a single main-agent execution request.
type MainRunRequest struct {
	AgentID      string
	SessionKey   string
	Prompt       string
	SystemPrompt string
	Workspace    string
	Metadata     map[string]any
	Media        []MainRunMedia
}

// MainRunResult carries the main-agent execution output.
type MainRunResult struct {
	Output string
}

// MainRuntime executes main-agent requests.
type MainRuntime interface {
	Run(ctx context.Context, req MainRunRequest) (*MainRunResult, error)
	Close() error
}
