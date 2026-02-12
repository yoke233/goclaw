package tasks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/sqlite"
	"github.com/google/uuid"
)

// SQLiteStore 使用 SQLite 持久化任务数据
type SQLiteStore struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// NewSQLiteStore 创建任务存储
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("db path is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create task db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open task db: %w", err)
	}

	store := &SQLiteStore{
		db:   db,
		path: dbPath,
	}

	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS requirements (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  requirement_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT DEFAULT '',
  role TEXT DEFAULT '',
  assignee TEXT DEFAULT '',
  status TEXT NOT NULL,
  acceptance_criteria TEXT DEFAULT '',
  depends_on_json TEXT DEFAULT '[]',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY(requirement_id) REFERENCES requirements(id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_requirement ON tasks(requirement_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS task_progress (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  run_id TEXT DEFAULT '',
  status TEXT DEFAULT '',
  message TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_task_progress_task ON task_progress(task_id, created_at DESC);

CREATE TABLE IF NOT EXISTS task_runs (
  run_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id)
);`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize task schema: %w", err)
	}
	return nil
}

// Close 关闭存储
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreateRequirement 创建需求
func (s *SQLiteStore) CreateRequirement(title, description string) (*Requirement, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	now := time.Now().UTC()
	req := &Requirement{
		ID:          uuid.NewString(),
		Title:       title,
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := s.db.Exec(
		`INSERT INTO requirements(id, title, description, created_at, updated_at) VALUES(?,?,?,?,?)`,
		req.ID, req.Title, req.Description, req.CreatedAt.UnixMilli(), req.UpdatedAt.UnixMilli(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create requirement: %w", err)
	}
	return req, nil
}

// CreateTask 创建任务
func (s *SQLiteStore) CreateTask(input CreateTaskInput) (*Task, error) {
	reqID := strings.TrimSpace(input.RequirementID)
	if reqID == "" {
		return nil, fmt.Errorf("requirement_id is required")
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	exists, err := s.requirementExists(reqID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("requirement not found: %s", reqID)
	}

	now := time.Now().UTC()
	task := &Task{
		ID:                 uuid.NewString(),
		RequirementID:      reqID,
		Title:              title,
		Description:        strings.TrimSpace(input.Description),
		Role:               normalizeRole(input.Role),
		Assignee:           strings.TrimSpace(input.Assignee),
		Status:             StatusTodo,
		AcceptanceCriteria: strings.TrimSpace(input.AcceptanceCriteria),
		DependsOn:          compactStringSlice(input.DependsOn),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	depJSON, err := json.Marshal(task.DependsOn)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal depends_on: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO tasks(
      id, requirement_id, title, description, role, assignee, status, acceptance_criteria, depends_on_json, created_at, updated_at
    ) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		task.ID,
		task.RequirementID,
		task.Title,
		task.Description,
		task.Role,
		task.Assignee,
		string(task.Status),
		task.AcceptanceCriteria,
		string(depJSON),
		task.CreatedAt.UnixMilli(),
		task.UpdatedAt.UnixMilli(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

// AssignTaskRole 更新任务角色与负责人
func (s *SQLiteStore) AssignTaskRole(taskID, role, assignee string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}

	_, err := s.db.Exec(
		`UPDATE tasks SET role = ?, assignee = ?, updated_at = ? WHERE id = ?`,
		normalizeRole(role),
		strings.TrimSpace(assignee),
		time.Now().UTC().UnixMilli(),
		taskID,
	)
	if err != nil {
		return fmt.Errorf("failed to assign task role: %w", err)
	}
	return nil
}

// UpdateTaskStatus 更新任务状态
func (s *SQLiteStore) UpdateTaskStatus(taskID string, status TaskStatus) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !IsValidTaskStatus(status) {
		return fmt.Errorf("invalid task status: %s", status)
	}

	result, err := s.db.Exec(
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`,
		string(status),
		time.Now().UTC().UnixMilli(),
		taskID,
	)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// AppendTaskProgress 追加任务进度
func (s *SQLiteStore) AppendTaskProgress(input AppendProgressInput) (*TaskProgressEntry, error) {
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}

	entry := &TaskProgressEntry{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		RunID:     strings.TrimSpace(input.RunID),
		Status:    strings.TrimSpace(input.Status),
		Message:   message,
		CreatedAt: time.Now().UTC(),
	}

	_, err := s.db.Exec(
		`INSERT INTO task_progress(id, task_id, run_id, status, message, created_at) VALUES(?,?,?,?,?,?)`,
		entry.ID,
		entry.TaskID,
		entry.RunID,
		entry.Status,
		entry.Message,
		entry.CreatedAt.UnixMilli(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to append task progress: %w", err)
	}
	return entry, nil
}

// ListTasksByRequirement 按需求列任务，requirementID 为空时返回全部任务
func (s *SQLiteStore) ListTasksByRequirement(requirementID string) ([]*Task, error) {
	reqID := strings.TrimSpace(requirementID)

	var (
		rows *sql.Rows
		err  error
	)
	if reqID == "" {
		rows, err = s.db.Query(
			`SELECT id, requirement_id, title, description, role, assignee, status, acceptance_criteria, depends_on_json, created_at, updated_at
       FROM tasks ORDER BY created_at DESC`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, requirement_id, title, description, role, assignee, status, acceptance_criteria, depends_on_json, created_at, updated_at
       FROM tasks WHERE requirement_id = ? ORDER BY created_at DESC`,
			reqID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tasks: %w", err)
	}
	return tasks, nil
}

// ListTaskProgress 列出任务进度日志
func (s *SQLiteStore) ListTaskProgress(taskID string, limit int) ([]*TaskProgressEntry, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(
		`SELECT id, task_id, run_id, status, message, created_at
     FROM task_progress WHERE task_id = ? ORDER BY created_at DESC LIMIT ?`,
		taskID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query task progress: %w", err)
	}
	defer rows.Close()

	var entries []*TaskProgressEntry
	for rows.Next() {
		var (
			entry     TaskProgressEntry
			createdAt int64
		)
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.RunID, &entry.Status, &entry.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan task progress: %w", err)
		}
		entry.CreatedAt = time.UnixMilli(createdAt).UTC()
		entries = append(entries, &entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task progress: %w", err)
	}
	return entries, nil
}

// GetTaskBoardSummary 获取看板统计
func (s *SQLiteStore) GetTaskBoardSummary(requirementID string) (*TaskBoardSummary, error) {
	summary := &TaskBoardSummary{
		RequirementID: strings.TrimSpace(requirementID),
	}

	var (
		rows *sql.Rows
		err  error
	)
	if summary.RequirementID == "" {
		rows, err = s.db.Query(`SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	} else {
		rows, err = s.db.Query(`SELECT status, COUNT(*) FROM tasks WHERE requirement_id = ? GROUP BY status`, summary.RequirementID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query board summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			status string
			count  int
		)
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan board summary: %w", err)
		}
		summary.Total += count
		switch TaskStatus(status) {
		case StatusTodo:
			summary.Todo = count
		case StatusDoing:
			summary.Doing = count
		case StatusBlocked:
			summary.Blocked = count
		case StatusDone:
			summary.Done = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate board summary: %w", err)
	}
	return summary, nil
}

// LinkSubagentRun 建立 run 与 task 的映射
func (s *SQLiteStore) LinkSubagentRun(runID, taskID string) error {
	runID = strings.TrimSpace(runID)
	taskID = strings.TrimSpace(taskID)
	if runID == "" || taskID == "" {
		return fmt.Errorf("run_id and task_id are required")
	}

	_, err := s.db.Exec(
		`INSERT INTO task_runs(run_id, task_id, created_at) VALUES(?,?,?)
     ON CONFLICT(run_id) DO UPDATE SET task_id = excluded.task_id`,
		runID, taskID, time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("failed to link subagent run: %w", err)
	}
	return nil
}

// ResolveTaskByRun 通过 run_id 反查 task_id
func (s *SQLiteStore) ResolveTaskByRun(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}

	var taskID string
	err := s.db.QueryRow(`SELECT task_id FROM task_runs WHERE run_id = ?`, runID).Scan(&taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to resolve task by run: %w", err)
	}
	return taskID, nil
}

func (s *SQLiteStore) requirementExists(requirementID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM requirements WHERE id = ?`, requirementID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to query requirement: %w", err)
	}
	return count > 0, nil
}

func scanTask(rows *sql.Rows) (*Task, error) {
	var (
		task        Task
		status      string
		depJSON     string
		createdAtMS int64
		updatedAtMS int64
	)

	if err := rows.Scan(
		&task.ID,
		&task.RequirementID,
		&task.Title,
		&task.Description,
		&task.Role,
		&task.Assignee,
		&status,
		&task.AcceptanceCriteria,
		&depJSON,
		&createdAtMS,
		&updatedAtMS,
	); err != nil {
		return nil, fmt.Errorf("failed to scan task: %w", err)
	}

	task.Status = TaskStatus(status)
	task.CreatedAt = time.UnixMilli(createdAtMS).UTC()
	task.UpdatedAt = time.UnixMilli(updatedAtMS).UTC()

	if strings.TrimSpace(depJSON) != "" {
		if err := json.Unmarshal([]byte(depJSON), &task.DependsOn); err != nil {
			return nil, fmt.Errorf("failed to decode depends_on: %w", err)
		}
	}
	if task.DependsOn == nil {
		task.DependsOn = []string{}
	}

	return &task, nil
}

func normalizeRole(role string) string {
	v := strings.ToLower(strings.TrimSpace(role))
	switch v {
	case "frontend", "backend":
		return v
	default:
		return ""
	}
}

func compactStringSlice(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, v := range values {
		item := strings.TrimSpace(v)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
