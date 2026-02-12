package tasks

import "time"

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusTodo    TaskStatus = "todo"
	StatusDoing   TaskStatus = "doing"
	StatusBlocked TaskStatus = "blocked"
	StatusDone    TaskStatus = "done"
)

// Requirement 需求实体
type Requirement struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Task 任务实体
type Task struct {
	ID                 string     `json:"id"`
	RequirementID      string     `json:"requirement_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Role               string     `json:"role,omitempty"`
	Assignee           string     `json:"assignee,omitempty"`
	Status             TaskStatus `json:"status"`
	AcceptanceCriteria string     `json:"acceptance_criteria,omitempty"`
	DependsOn          []string   `json:"depends_on,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// TaskProgressEntry 任务进度日志
type TaskProgressEntry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	RunID     string    `json:"run_id,omitempty"`
	Status    string    `json:"status,omitempty"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskBoardSummary 看板统计
type TaskBoardSummary struct {
	RequirementID string `json:"requirement_id,omitempty"`
	Total         int    `json:"total"`
	Todo          int    `json:"todo"`
	Doing         int    `json:"doing"`
	Blocked       int    `json:"blocked"`
	Done          int    `json:"done"`
}

// CreateTaskInput 创建任务输入
type CreateTaskInput struct {
	RequirementID      string
	Title              string
	Description        string
	Role               string
	Assignee           string
	AcceptanceCriteria string
	DependsOn          []string
}

// AppendProgressInput 追加任务进度输入
type AppendProgressInput struct {
	TaskID  string
	RunID   string
	Status  string
	Message string
}

// Store 任务存储接口
type Store interface {
	Close() error
	CreateRequirement(title, description string) (*Requirement, error)
	CreateTask(input CreateTaskInput) (*Task, error)
	AssignTaskRole(taskID, role, assignee string) error
	UpdateTaskStatus(taskID string, status TaskStatus) error
	AppendTaskProgress(input AppendProgressInput) (*TaskProgressEntry, error)
	ListTasksByRequirement(requirementID string) ([]*Task, error)
	ListTaskProgress(taskID string, limit int) ([]*TaskProgressEntry, error)
	GetTaskBoardSummary(requirementID string) (*TaskBoardSummary, error)
	LinkSubagentRun(runID, taskID string) error
	ResolveTaskByRun(runID string) (string, error)
}

// IsValidTaskStatus 检查状态是否合法
func IsValidTaskStatus(status TaskStatus) bool {
	switch status {
	case StatusTodo, StatusDoing, StatusBlocked, StatusDone:
		return true
	default:
		return false
	}
}
