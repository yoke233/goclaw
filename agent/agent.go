package agent

import (
	"fmt"
	"sync"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/session"
)

// Agent represents static per-agent configuration used by AgentManager.
// Main execution is handled by MainRuntime (agentsdk-go), not by this struct.
type Agent struct {
	mu        sync.RWMutex
	state     *AgentState
	workspace string
}

// NewAgentConfig configures managed agent metadata.
type NewAgentConfig struct {
	Bus          *bus.MessageBus
	SessionMgr   *session.Manager
	Tools        *ToolRegistry
	Context      *ContextBuilder
	Workspace    string
	MaxIteration int
}

// NewAgent creates a lightweight managed agent container.
func NewAgent(cfg *NewAgentConfig) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	state := NewAgentState()
	if cfg.Context != nil {
		state.SystemPrompt = cfg.Context.BuildSystemPrompt()
	}
	state.SessionKey = "main"

	return &Agent{
		state:     state,
		workspace: cfg.Workspace,
	}, nil
}

// GetState returns a cloned agent state.
func (a *Agent) GetState() *AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state.Clone()
}

// SetSystemPrompt updates the agent system prompt.
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.SystemPrompt = prompt
}

// GetWorkspace returns the agent workspace.
func (a *Agent) GetWorkspace() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace
}
