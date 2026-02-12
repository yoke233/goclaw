package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	taskstore "github.com/smallnest/goclaw/agent/tasks"
	"github.com/smallnest/goclaw/config"
	"github.com/spf13/cobra"
)

var (
	taskRequirementTitle       string
	taskRequirementDescription string

	taskCreateRequirementID string
	taskCreateTitle         string
	taskCreateDescription   string
	taskCreateRole          string
	taskCreateAssignee      string
	taskCreateAcceptance    string
	taskCreateDependsOn     string

	taskAssignRole     string
	taskAssignAssignee string

	taskStatusValue   string
	taskStatusMessage string

	taskProgressMessage string
	taskProgressStatus  string
	taskProgressRunID   string

	taskListRequirement   string
	taskListWithProgress  bool
	taskListProgressLimit int
)

// TaskCommand 任务管理命令
func TaskCommand() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage project requirements and tasks",
		Long:  `Create, assign, update and track project tasks for subagent execution.`,
	}

	requirementCmd := &cobra.Command{
		Use:   "requirement",
		Short: "Create a requirement",
		Run:   runTaskRequirementCreate,
	}
	requirementCmd.Flags().StringVar(&taskRequirementTitle, "title", "", "Requirement title")
	requirementCmd.Flags().StringVar(&taskRequirementDescription, "description", "", "Requirement description")
	_ = requirementCmd.MarkFlagRequired("title")

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task under a requirement",
		Run:   runTaskCreate,
	}
	createCmd.Flags().StringVar(&taskCreateRequirementID, "requirement", "", "Requirement ID")
	createCmd.Flags().StringVar(&taskCreateTitle, "title", "", "Task title")
	createCmd.Flags().StringVar(&taskCreateDescription, "description", "", "Task description")
	createCmd.Flags().StringVar(&taskCreateRole, "role", "", "Role for this task (frontend/backend)")
	createCmd.Flags().StringVar(&taskCreateAssignee, "assignee", "", "Assignee display name")
	createCmd.Flags().StringVar(&taskCreateAcceptance, "acceptance", "", "Acceptance criteria")
	createCmd.Flags().StringVar(&taskCreateDependsOn, "depends-on", "", "Comma-separated task IDs this task depends on")
	_ = createCmd.MarkFlagRequired("requirement")
	_ = createCmd.MarkFlagRequired("title")

	assignCmd := &cobra.Command{
		Use:   "assign <task-id>",
		Short: "Assign role/assignee for a task",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskAssign,
	}
	assignCmd.Flags().StringVar(&taskAssignRole, "role", "", "Role for the task (frontend/backend)")
	assignCmd.Flags().StringVar(&taskAssignAssignee, "assignee", "", "Assignee display name")
	_ = assignCmd.MarkFlagRequired("role")

	statusCmd := &cobra.Command{
		Use:   "status <task-id>",
		Short: "Update task status",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskStatus,
	}
	statusCmd.Flags().StringVar(&taskStatusValue, "status", "", "Task status: todo|doing|blocked|done")
	statusCmd.Flags().StringVar(&taskStatusMessage, "message", "", "Optional progress message")
	_ = statusCmd.MarkFlagRequired("status")

	progressCmd := &cobra.Command{
		Use:   "progress <task-id>",
		Short: "Append task progress message",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskProgress,
	}
	progressCmd.Flags().StringVar(&taskProgressMessage, "message", "", "Progress message")
	progressCmd.Flags().StringVar(&taskProgressStatus, "status", "", "Optional progress status")
	progressCmd.Flags().StringVar(&taskProgressRunID, "run-id", "", "Optional linked subagent run ID")
	_ = progressCmd.MarkFlagRequired("message")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks and board summary",
		Run:   runTaskList,
	}
	listCmd.Flags().StringVar(&taskListRequirement, "requirement", "", "Filter by requirement ID")
	listCmd.Flags().BoolVar(&taskListWithProgress, "with-progress", false, "Include latest progress entries")
	listCmd.Flags().IntVar(&taskListProgressLimit, "progress-limit", 3, "Max progress entries for each task when --with-progress is enabled")

	taskCmd.AddCommand(requirementCmd)
	taskCmd.AddCommand(createCmd)
	taskCmd.AddCommand(assignCmd)
	taskCmd.AddCommand(statusCmd)
	taskCmd.AddCommand(progressCmd)
	taskCmd.AddCommand(listCmd)

	return taskCmd
}

func runTaskRequirementCreate(cmd *cobra.Command, args []string) {
	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	req, err := store.CreateRequirement(taskRequirementTitle, taskRequirementDescription)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create requirement: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Requirement created")
	fmt.Printf("  ID: %s\n", req.ID)
	fmt.Printf("  Title: %s\n", req.Title)
}

func runTaskCreate(cmd *cobra.Command, args []string) {
	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	task, err := store.CreateTask(taskstore.CreateTaskInput{
		RequirementID:      taskCreateRequirementID,
		Title:              taskCreateTitle,
		Description:        taskCreateDescription,
		Role:               taskCreateRole,
		Assignee:           taskCreateAssignee,
		AcceptanceCriteria: taskCreateAcceptance,
		DependsOn:          splitCSV(taskCreateDependsOn),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create task: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Task created")
	fmt.Printf("  ID: %s\n", task.ID)
	fmt.Printf("  Requirement: %s\n", task.RequirementID)
	fmt.Printf("  Title: %s\n", task.Title)
	fmt.Printf("  Status: %s\n", task.Status)
}

func runTaskAssign(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.AssignTaskRole(taskID, taskAssignRole, taskAssignAssignee); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to assign task: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Task assigned: %s (role=%s assignee=%s)\n", taskID, taskAssignRole, taskAssignAssignee)
}

func runTaskStatus(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	status := taskstore.TaskStatus(strings.ToLower(strings.TrimSpace(taskStatusValue)))
	if !taskstore.IsValidTaskStatus(status) {
		fmt.Fprintf(os.Stderr, "Invalid task status: %s (allowed: todo|doing|blocked|done)\n", taskStatusValue)
		os.Exit(1)
	}

	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.UpdateTaskStatus(taskID, status); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update task status: %v\n", err)
		os.Exit(1)
	}

	if strings.TrimSpace(taskStatusMessage) != "" {
		if _, err := store.AppendTaskProgress(taskstore.AppendProgressInput{
			TaskID:  taskID,
			Status:  string(status),
			Message: taskStatusMessage,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to append progress message: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Task status updated: %s -> %s\n", taskID, status)
}

func runTaskProgress(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	entry, err := store.AppendTaskProgress(taskstore.AppendProgressInput{
		TaskID:  taskID,
		RunID:   taskProgressRunID,
		Status:  taskProgressStatus,
		Message: taskProgressMessage,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to append progress: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Task progress appended")
	fmt.Printf("  Entry ID: %s\n", entry.ID)
	fmt.Printf("  Task ID: %s\n", entry.TaskID)
}

func runTaskList(cmd *cobra.Command, args []string) {
	store, err := openTaskStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open task store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	summary, err := store.GetTaskBoardSummary(taskListRequirement)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get task board summary: %v\n", err)
		os.Exit(1)
	}

	tasks, err := store.ListTasksByRequirement(taskListRequirement)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list tasks: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Task Board Summary")
	fmt.Println("==================")
	if taskListRequirement != "" {
		fmt.Printf("Requirement: %s\n", taskListRequirement)
	}
	fmt.Printf("Total: %d  Todo: %d  Doing: %d  Blocked: %d  Done: %d\n",
		summary.Total, summary.Todo, summary.Doing, summary.Blocked, summary.Done)

	if len(tasks) == 0 {
		fmt.Println("\nNo tasks found.")
		return
	}

	fmt.Println("\nTasks")
	fmt.Println("=====")
	for _, task := range tasks {
		fmt.Printf("- [%s] %s (%s)\n", task.Status, task.Title, task.ID)
		if task.Role != "" || task.Assignee != "" {
			fmt.Printf("  Role: %s  Assignee: %s\n", emptyAs(task.Role, "-"), emptyAs(task.Assignee, "-"))
		}
		if task.AcceptanceCriteria != "" {
			fmt.Printf("  Acceptance: %s\n", task.AcceptanceCriteria)
		}
		if len(task.DependsOn) > 0 {
			fmt.Printf("  Depends On: %s\n", strings.Join(task.DependsOn, ", "))
		}

		if taskListWithProgress {
			entries, progressErr := store.ListTaskProgress(task.ID, taskListProgressLimit)
			if progressErr == nil && len(entries) > 0 {
				fmt.Println("  Progress:")
				for _, entry := range entries {
					parts := []string{entry.CreatedAt.Format("2006-01-02 15:04:05")}
					if entry.Status != "" {
						parts = append(parts, entry.Status)
					}
					if entry.RunID != "" {
						parts = append(parts, "run="+entry.RunID)
					}
					fmt.Printf("    - [%s] %s\n", strings.Join(parts, " | "), entry.Message)
				}
			}
		}
	}
}

func openTaskStore() (taskstore.Store, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}

	workspace, err := config.GetWorkspacePath(cfg)
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(workspace, "data", "tasks.db")
	return taskstore.NewSQLiteStore(dbPath)
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		result = append(result, v)
	}
	return result
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
