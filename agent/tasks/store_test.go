package tasks

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStoreTaskLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	req, err := store.CreateRequirement("用户系统改造", "拆分为前后端并行开发")
	if err != nil {
		t.Fatalf("CreateRequirement() error = %v", err)
	}
	if req.ID == "" {
		t.Fatalf("CreateRequirement() returned empty id")
	}

	task, err := store.CreateTask(CreateTaskInput{
		RequirementID:      req.ID,
		Title:              "实现登录页",
		Description:        "含验证码和错误提示",
		Role:               "frontend",
		Assignee:           "alice",
		AcceptanceCriteria: "提交后跳转控制台",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.Status != StatusTodo {
		t.Fatalf("task status = %s, want %s", task.Status, StatusTodo)
	}

	if err := store.AssignTaskRole(task.ID, "frontend", "bob"); err != nil {
		t.Fatalf("AssignTaskRole() error = %v", err)
	}

	if err := store.UpdateTaskStatus(task.ID, StatusDoing); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}

	progress, err := store.AppendTaskProgress(AppendProgressInput{
		TaskID:  task.ID,
		RunID:   "run-123",
		Status:  "doing",
		Message: "已完成页面骨架",
	})
	if err != nil {
		t.Fatalf("AppendTaskProgress() error = %v", err)
	}
	if progress.ID == "" {
		t.Fatalf("AppendTaskProgress() returned empty progress id")
	}

	if err := store.LinkSubagentRun("run-123", task.ID); err != nil {
		t.Fatalf("LinkSubagentRun() error = %v", err)
	}
	taskID, err := store.ResolveTaskByRun("run-123")
	if err != nil {
		t.Fatalf("ResolveTaskByRun() error = %v", err)
	}
	if taskID != task.ID {
		t.Fatalf("ResolveTaskByRun() = %s, want %s", taskID, task.ID)
	}

	tasks, err := store.ListTasksByRequirement(req.ID)
	if err != nil {
		t.Fatalf("ListTasksByRequirement() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("ListTasksByRequirement() len = %d, want 1", len(tasks))
	}
	if tasks[0].Assignee != "bob" {
		t.Fatalf("task assignee = %s, want bob", tasks[0].Assignee)
	}
	if tasks[0].Status != StatusDoing {
		t.Fatalf("task status = %s, want %s", tasks[0].Status, StatusDoing)
	}

	entries, err := store.ListTaskProgress(task.ID, 10)
	if err != nil {
		t.Fatalf("ListTaskProgress() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListTaskProgress() len = %d, want 1", len(entries))
	}
	if entries[0].RunID != "run-123" {
		t.Fatalf("progress run_id = %s, want run-123", entries[0].RunID)
	}

	summary, err := store.GetTaskBoardSummary(req.ID)
	if err != nil {
		t.Fatalf("GetTaskBoardSummary() error = %v", err)
	}
	if summary.Total != 1 || summary.Doing != 1 {
		t.Fatalf("summary unexpected: %+v", summary)
	}
}

func TestInvalidStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	req, err := store.CreateRequirement("需求", "")
	if err != nil {
		t.Fatalf("CreateRequirement() error = %v", err)
	}
	task, err := store.CreateTask(CreateTaskInput{
		RequirementID: req.ID,
		Title:         "任务",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	if err := store.UpdateTaskStatus(task.ID, TaskStatus("unknown")); err == nil {
		t.Fatalf("UpdateTaskStatus() expected error for invalid status")
	}
}
