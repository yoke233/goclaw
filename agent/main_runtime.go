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
	// ToolWhitelist restricts which tools are exposed to the model for this
	// request. Nil means "no restriction" (default behaviour). Note that in
	// agentsdk-go an empty slice is treated the same as nil, so to effectively
	// disable tools callers should pass a non-empty list that doesn't match any
	// registered tool (e.g. ["__no_tools__"]).
	ToolWhitelist []string
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
