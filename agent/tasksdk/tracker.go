package tasksdk

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
	_ "github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/smallnest/goclaw/agent"
)

// Tracker bridges AgentManager task tracking with agentsdk task store.
type Tracker struct {
	store sdktasks.Store
	db    *sql.DB
}

// TaskProgressEntry represents a persisted progress log entry.
type TaskProgressEntry struct {
	ID        string
	TaskID    string
	RunID     string
	Status    string
	Message   string
	CreatedAt time.Time
}

var _ agent.TaskTracker = (*Tracker)(nil)

// NewTracker creates a tracker backed by SQLite for run/progress mapping.
func NewTracker(store sdktasks.Store, dbPath string) (*Tracker, error) {
	if store == nil {
		return nil, fmt.Errorf("task store is required")
	}
	path := strings.TrimSpace(dbPath)
	if path == "" {
		return nil, fmt.Errorf("tracker sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create tracker sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open tracker sqlite: %w", err)
	}

	tracker := &Tracker{store: store, db: db}
	if err := tracker.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return tracker, nil
}

func (t *Tracker) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS subagent_task_runs (
  run_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS subagent_task_progress (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  run_id TEXT DEFAULT '',
  status TEXT DEFAULT '',
  message TEXT NOT NULL,
  created_at INTEGER NOT NULL
);`
	if _, err := t.db.Exec(schema); err != nil {
		return fmt.Errorf("init tracker schema: %w", err)
	}
	return nil
}

// Close closes underlying resources.
func (t *Tracker) Close() error {
	if t.db == nil {
		return nil
	}
	err := t.db.Close()
	t.db = nil
	return err
}

// LinkSubagentRun stores run -> task mapping.
func (t *Tracker) LinkSubagentRun(runID, taskID string) error {
	runID = strings.TrimSpace(runID)
	taskID = strings.TrimSpace(taskID)
	if runID == "" || taskID == "" {
		return fmt.Errorf("run_id and task_id are required")
	}

	_, err := t.db.Exec(
		`INSERT INTO subagent_task_runs(run_id, task_id, created_at)
     VALUES(?,?,?)
     ON CONFLICT(run_id) DO UPDATE SET task_id = excluded.task_id`,
		runID,
		taskID,
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("link subagent run: %w", err)
	}
	return nil
}

// ResolveTaskByRun resolves task id from run id.
func (t *Tracker) ResolveTaskByRun(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", nil
	}

	var taskID string
	err := t.db.QueryRow(`SELECT task_id FROM subagent_task_runs WHERE run_id = ?`, runID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve task by run: %w", err)
	}
	return strings.TrimSpace(taskID), nil
}

// UpdateTaskStatus updates task status in agentsdk task store.
func (t *Tracker) UpdateTaskStatus(taskID string, status string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}

	nextStatus, ok := normalizeTaskStatus(status)
	if !ok {
		return fmt.Errorf("invalid task status: %s", status)
	}

	_, err := t.store.Update(taskID, sdktasks.TaskUpdate{Status: &nextStatus})
	return err
}

// AppendTaskProgress stores tracker progress logs.
func (t *Tracker) AppendTaskProgress(input agent.TaskProgressInput) error {
	taskID := strings.TrimSpace(input.TaskID)
	message := strings.TrimSpace(input.Message)
	if taskID == "" || message == "" {
		return fmt.Errorf("task_id and message are required")
	}

	_, err := t.db.Exec(
		`INSERT INTO subagent_task_progress(id, task_id, run_id, status, message, created_at)
     VALUES(?,?,?,?,?,?)`,
		uuid.NewString(),
		taskID,
		strings.TrimSpace(input.RunID),
		strings.TrimSpace(input.Status),
		message,
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("append task progress: %w", err)
	}
	return nil
}

// ListTaskProgress returns latest task progress entries in reverse chronological order.
func (t *Tracker) ListTaskProgress(taskID string, limit int) ([]TaskProgressEntry, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := t.db.Query(
		`SELECT id, task_id, run_id, status, message, created_at
     FROM subagent_task_progress
     WHERE task_id = ?
     ORDER BY created_at DESC
     LIMIT ?`,
		taskID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list task progress: %w", err)
	}
	defer rows.Close()

	result := make([]TaskProgressEntry, 0)
	for rows.Next() {
		var (
			entry     TaskProgressEntry
			createdMS int64
		)
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.RunID, &entry.Status, &entry.Message, &createdMS); err != nil {
			return nil, fmt.Errorf("scan task progress: %w", err)
		}
		entry.CreatedAt = time.UnixMilli(createdMS).UTC()
		result = append(result, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task progress: %w", err)
	}
	return result, nil
}

func normalizeTaskStatus(value string) (sdktasks.TaskStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "todo":
		return sdktasks.TaskPending, true
	case "in_progress", "doing":
		return sdktasks.TaskInProgress, true
	case "completed", "done":
		return sdktasks.TaskCompleted, true
	case "blocked":
		return sdktasks.TaskBlocked, true
	default:
		return "", false
	}
}
