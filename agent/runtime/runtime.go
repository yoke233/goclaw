package runtime

import "context"

const (
	RunStatusOK      = "ok"
	RunStatusError   = "error"
	RunStatusTimeout = "timeout"
)

// SubagentRuntime 抽象分身执行运行时。
// 当前用于让 AgentManager 与具体运行时（agentsdk 或其它实现）解耦。
type SubagentRuntime interface {
	Spawn(ctx context.Context, req SubagentRunRequest) (runID string, err error)
	Wait(ctx context.Context, runID string) (*SubagentRunResult, error)
	Cancel(ctx context.Context, runID string) error
}

// SubagentRunRequest 定义一次分身任务的执行参数。
type SubagentRunRequest struct {
	RunID          string
	Task           string
	Role           string
	WorkDir        string
	SkillsDir      string
	SystemPrompt   string
	TimeoutSeconds int
}

// SubagentRunResult 定义分身执行结果。
type SubagentRunResult struct {
	Status   string // ok|error|timeout
	Output   string
	ErrorMsg string
}
