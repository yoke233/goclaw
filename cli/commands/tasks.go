package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
	"github.com/smallnest/goclaw/agent"
	tasksdk "github.com/smallnest/goclaw/agent/tasksdk"
	"github.com/smallnest/goclaw/config"
	"github.com/spf13/cobra"
)

var (
	taskCreateSubject     string
	taskCreateTitle       string // backward-compatible alias of subject
	taskCreateDescription string
	taskCreateActiveForm  string
	taskCreateOwner       string
	taskCreateAssignee    string // backward-compatible alias of owner
	taskCreateAcceptance  string // merged into description
	taskCreateRequirement string // deprecated
	taskCreateBlockedBy   string
	taskCreateDependsOn   string // backward-compatible alias of blockedBy
	taskCreateBlocks      string

	taskUpdateStatus    string
	taskUpdateOwner     string
	taskUpdateBlockedBy string
	taskUpdateBlocks    string

	taskAssignOwner    string
	taskAssignAssignee string // alias

	taskStatusValue   string
	taskStatusMessage string

	taskProgressMessage string
	taskProgressStatus  string
	taskProgressRunID   string

	taskListStatus        string
	taskListOwner         string
	taskListWithProgress  bool
	taskListProgressLimit int
)

// TaskCommand 任务管理命令（agentsdk task store 版本）
func TaskCommand() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks backed by AgentSDK task store",
		Long:  `Create, update and track AgentSDK tasks for orchestrated subagent execution.`,
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task",
		Run:   runTaskCreate,
	}
	createCmd.Flags().StringVar(&taskCreateSubject, "subject", "", "Task subject")
	createCmd.Flags().StringVar(&taskCreateTitle, "title", "", "Task subject (alias of --subject)")
	createCmd.Flags().StringVar(&taskCreateDescription, "description", "", "Task description")
	createCmd.Flags().StringVar(&taskCreateActiveForm, "active-form", "", "Task active form (default: subject)")
	createCmd.Flags().StringVar(&taskCreateOwner, "owner", "", "Task owner")
	createCmd.Flags().StringVar(&taskCreateAssignee, "assignee", "", "Task owner (alias of --owner)")
	createCmd.Flags().StringVar(&taskCreateAcceptance, "acceptance", "", "Backward-compatible acceptance text appended to description")
	createCmd.Flags().StringVar(&taskCreateRequirement, "requirement", "", "Deprecated: requirement id is ignored")
	createCmd.Flags().StringVar(&taskCreateBlockedBy, "blocked-by", "", "Comma-separated blocking task IDs")
	createCmd.Flags().StringVar(&taskCreateDependsOn, "depends-on", "", "Alias of --blocked-by")
	createCmd.Flags().StringVar(&taskCreateBlocks, "blocks", "", "Comma-separated task IDs blocked by this task")

	getCmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskGet,
	}

	updateCmd := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update task status/owner/dependencies",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskUpdate,
	}
	updateCmd.Flags().StringVar(&taskUpdateStatus, "status", "", "Task status: pending|in_progress|completed|blocked (aliases: todo|doing|done)")
	updateCmd.Flags().StringVar(&taskUpdateOwner, "owner", "", "Task owner")
	updateCmd.Flags().StringVar(&taskUpdateBlockedBy, "blocked-by", "", "Replace blocking task IDs (comma-separated)")
	updateCmd.Flags().StringVar(&taskUpdateBlocks, "blocks", "", "Replace blocked task IDs (comma-separated)")

	assignCmd := &cobra.Command{
		Use:   "assign <task-id>",
		Short: "Assign owner for a task (compat command)",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskAssign,
	}
	assignCmd.Flags().StringVar(&taskAssignOwner, "owner", "", "Task owner")
	assignCmd.Flags().StringVar(&taskAssignAssignee, "assignee", "", "Task owner (alias of --owner)")

	statusCmd := &cobra.Command{
		Use:   "status <task-id>",
		Short: "Update task status",
		Args:  cobra.ExactArgs(1),
		Run:   runTaskStatus,
	}
	statusCmd.Flags().StringVar(&taskStatusValue, "status", "", "Task status: pending|in_progress|completed|blocked (aliases: todo|doing|done)")
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
		Short: "List tasks and status summary",
		Run:   runTaskList,
	}
	listCmd.Flags().StringVar(&taskListStatus, "status", "", "Filter by status")
	listCmd.Flags().StringVar(&taskListOwner, "owner", "", "Filter by owner")
	listCmd.Flags().BoolVar(&taskListWithProgress, "with-progress", false, "Include latest progress entries")
	listCmd.Flags().IntVar(&taskListProgressLimit, "progress-limit", 3, "Max progress entries per task when --with-progress is enabled")

	taskCmd.AddCommand(createCmd)
	taskCmd.AddCommand(getCmd)
	taskCmd.AddCommand(updateCmd)
	taskCmd.AddCommand(assignCmd)
	taskCmd.AddCommand(statusCmd)
	taskCmd.AddCommand(progressCmd)
	taskCmd.AddCommand(listCmd)

	return taskCmd
}

func runTaskCreate(cmd *cobra.Command, args []string) {
	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	subject := strings.TrimSpace(taskCreateSubject)
	if subject == "" {
		subject = strings.TrimSpace(taskCreateTitle)
	}
	if subject == "" {
		failf("task subject is required (--subject or --title)")
	}

	activeForm := strings.TrimSpace(taskCreateActiveForm)
	if activeForm == "" {
		activeForm = subject
	}

	description := strings.TrimSpace(taskCreateDescription)
	if acc := strings.TrimSpace(taskCreateAcceptance); acc != "" {
		if description == "" {
			description = "Acceptance: " + acc
		} else {
			description = description + "\n\nAcceptance: " + acc
		}
	}
	if req := strings.TrimSpace(taskCreateRequirement); req != "" {
		fmt.Fprintf(os.Stderr, "Warning: --requirement is deprecated and ignored in AgentSDK task model (value=%s)\n", req)
	}

	created, err := store.Create(subject, description, activeForm)
	if err != nil {
		failf("Failed to create task: %v", err)
	}

	owner := strings.TrimSpace(taskCreateOwner)
	if owner == "" {
		owner = strings.TrimSpace(taskCreateAssignee)
	}
	if owner != "" {
		if _, err := store.Update(created.ID, sdktasks.TaskUpdate{Owner: &owner}); err != nil {
			failf("Failed to set task owner: %v", err)
		}
	}

	blockedBy := splitCSV(taskCreateBlockedBy)
	if len(blockedBy) == 0 {
		blockedBy = splitCSV(taskCreateDependsOn)
	}
	for _, blockerID := range blockedBy {
		if err := store.AddDependency(created.ID, blockerID); err != nil {
			failf("Failed to add blocked-by dependency %s: %v", blockerID, err)
		}
	}

	blocks := splitCSV(taskCreateBlocks)
	for _, blockedTaskID := range blocks {
		if err := store.AddDependency(blockedTaskID, created.ID); err != nil {
			failf("Failed to add blocks dependency %s: %v", blockedTaskID, err)
		}
	}

	task, err := store.Get(created.ID)
	if err != nil {
		failf("Failed to reload created task: %v", err)
	}
	printTaskCreated(task)
}

func runTaskGet(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	task, err := store.Get(taskID)
	if err != nil {
		failf("Failed to get task: %v", err)
	}
	printTaskDetail(store, task)
}

func runTaskUpdate(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	current, err := store.Get(taskID)
	if err != nil {
		failf("Failed to load task: %v", err)
	}

	var updates sdktasks.TaskUpdate
	if cmd.Flags().Changed("status") {
		status, parseErr := parseTaskStatus(taskUpdateStatus)
		if parseErr != nil {
			failf(parseErr.Error())
		}
		updates.Status = &status
	}
	if cmd.Flags().Changed("owner") {
		owner := strings.TrimSpace(taskUpdateOwner)
		updates.Owner = &owner
	}

	if updates.Status != nil || updates.Owner != nil {
		if _, err := store.Update(taskID, updates); err != nil {
			failf("Failed to update task: %v", err)
		}
	}

	if cmd.Flags().Changed("blocked-by") {
		desired := splitCSV(taskUpdateBlockedBy)
		if err := replaceBlockedBy(store, taskID, current.BlockedBy, desired); err != nil {
			failf("Failed to replace blocked-by dependencies: %v", err)
		}
		current, _ = store.Get(taskID)
	}
	if cmd.Flags().Changed("blocks") {
		desired := splitCSV(taskUpdateBlocks)
		if err := replaceBlocks(store, taskID, current.Blocks, desired); err != nil {
			failf("Failed to replace blocks dependencies: %v", err)
		}
	}

	updated, err := store.Get(taskID)
	if err != nil {
		failf("Failed to reload updated task: %v", err)
	}
	fmt.Println("Task updated")
	fmt.Printf("  ID: %s\n", updated.ID)
	fmt.Printf("  Subject: %s\n", updated.Subject)
	fmt.Printf("  Status: %s\n", updated.Status)
	fmt.Printf("  Owner: %s\n", emptyAs(updated.Owner, "-"))
	fmt.Printf("  BlockedBy: %s\n", formatSlice(updated.BlockedBy))
	fmt.Printf("  Blocks: %s\n", formatSlice(updated.Blocks))
}

func runTaskAssign(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	owner := strings.TrimSpace(taskAssignOwner)
	if owner == "" {
		owner = strings.TrimSpace(taskAssignAssignee)
	}
	if owner == "" {
		failf("owner is required (--owner or --assignee)")
	}

	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	if _, err := store.Update(taskID, sdktasks.TaskUpdate{Owner: &owner}); err != nil {
		failf("Failed to assign task owner: %v", err)
	}
	fmt.Printf("Task owner assigned: %s -> %s\n", taskID, owner)
}

func runTaskStatus(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	status, err := parseTaskStatus(taskStatusValue)
	if err != nil {
		failf(err.Error())
	}

	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	if _, err := store.Update(taskID, sdktasks.TaskUpdate{Status: &status}); err != nil {
		failf("Failed to update task status: %v", err)
	}

	if strings.TrimSpace(taskStatusMessage) != "" {
		tracker, trackerErr := openTaskTracker(store)
		if trackerErr != nil {
			failf("Failed to open task tracker: %v", trackerErr)
		}
		defer tracker.Close()

		if err := tracker.AppendTaskProgress(agent.TaskProgressInput{
			TaskID:  taskID,
			Status:  string(status),
			Message: strings.TrimSpace(taskStatusMessage),
		}); err != nil {
			failf("Failed to append task progress message: %v", err)
		}
	}

	fmt.Printf("Task status updated: %s -> %s\n", taskID, status)
}

func runTaskProgress(cmd *cobra.Command, args []string) {
	taskID := strings.TrimSpace(args[0])
	message := strings.TrimSpace(taskProgressMessage)
	if message == "" {
		failf("message is required")
	}

	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	if _, err := store.Get(taskID); err != nil {
		failf("Task not found: %v", err)
	}

	tracker, err := openTaskTracker(store)
	if err != nil {
		failf("Failed to open task tracker: %v", err)
	}
	defer tracker.Close()

	status := strings.TrimSpace(taskProgressStatus)
	if status != "" {
		normalized, parseErr := parseTaskStatus(status)
		if parseErr != nil {
			failf(parseErr.Error())
		}
		status = string(normalized)
	}

	if err := tracker.AppendTaskProgress(agent.TaskProgressInput{
		TaskID:  taskID,
		RunID:   strings.TrimSpace(taskProgressRunID),
		Status:  status,
		Message: message,
	}); err != nil {
		failf("Failed to append task progress: %v", err)
	}

	fmt.Println("Task progress appended")
	fmt.Printf("  Task ID: %s\n", taskID)
}

func runTaskList(cmd *cobra.Command, args []string) {
	store, err := openTaskStore()
	if err != nil {
		failf("Failed to open task store: %v", err)
	}
	defer store.Close()

	filterStatus := ""
	if strings.TrimSpace(taskListStatus) != "" {
		status, parseErr := parseTaskStatus(taskListStatus)
		if parseErr != nil {
			failf(parseErr.Error())
		}
		filterStatus = string(status)
	}
	filterOwner := strings.TrimSpace(taskListOwner)

	list := filterTasks(store.List(), filterStatus, filterOwner)
	counts := countStatuses(list)

	fmt.Println("Task Summary")
	fmt.Println("============")
	fmt.Printf("Total: %d  Pending: %d  InProgress: %d  Completed: %d  Blocked: %d\n",
		len(list), counts.pending, counts.inProgress, counts.completed, counts.blocked)
	if filterStatus != "" || filterOwner != "" {
		fmt.Printf("Filters: status=%s owner=%s\n", emptyAs(filterStatus, "-"), emptyAs(filterOwner, "-"))
	}

	if len(list) == 0 {
		fmt.Println("\nNo tasks found.")
		return
	}

	var tracker *tasksdk.Tracker
	if taskListWithProgress {
		tracker, err = openTaskTracker(store)
		if err != nil {
			failf("Failed to open task tracker: %v", err)
		}
		defer tracker.Close()
	}

	fmt.Println("\nTasks")
	fmt.Println("=====")
	for _, task := range list {
		fmt.Printf("- [%s] %s (%s)\n", task.Status, task.Subject, task.ID)
		if strings.TrimSpace(task.Owner) != "" {
			fmt.Printf("  Owner: %s\n", task.Owner)
		}
		if strings.TrimSpace(task.Description) != "" {
			fmt.Printf("  Description: %s\n", task.Description)
		}
		if len(task.BlockedBy) > 0 {
			fmt.Printf("  BlockedBy: %s\n", strings.Join(task.BlockedBy, ", "))
		}
		if len(task.Blocks) > 0 {
			fmt.Printf("  Blocks: %s\n", strings.Join(task.Blocks, ", "))
		}

		if tracker != nil {
			entries, progressErr := tracker.ListTaskProgress(task.ID, taskListProgressLimit)
			if progressErr == nil && len(entries) > 0 {
				fmt.Println("  Progress:")
				for _, entry := range entries {
					parts := []string{entry.CreatedAt.Format("2006-01-02 15:04:05")}
					if strings.TrimSpace(entry.Status) != "" {
						parts = append(parts, entry.Status)
					}
					if strings.TrimSpace(entry.RunID) != "" {
						parts = append(parts, "run="+entry.RunID)
					}
					fmt.Printf("    - [%s] %s\n", strings.Join(parts, " | "), entry.Message)
				}
			}
		}
	}
}

type statusCounter struct {
	pending    int
	inProgress int
	completed  int
	blocked    int
}

func countStatuses(tasks []*sdktasks.Task) statusCounter {
	var c statusCounter
	for _, task := range tasks {
		if task == nil {
			continue
		}
		switch task.Status {
		case sdktasks.TaskPending:
			c.pending++
		case sdktasks.TaskInProgress:
			c.inProgress++
		case sdktasks.TaskCompleted:
			c.completed++
		case sdktasks.TaskBlocked:
			c.blocked++
		}
	}
	return c
}

func filterTasks(tasks []*sdktasks.Task, status, owner string) []*sdktasks.Task {
	filtered := make([]*sdktasks.Task, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if status != "" && string(task.Status) != status {
			continue
		}
		if owner != "" && !strings.EqualFold(strings.TrimSpace(task.Owner), owner) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}

func printTaskCreated(task *sdktasks.Task) {
	if task == nil {
		failf("created task is nil")
	}
	fmt.Println("Task created")
	fmt.Printf("  ID: %s\n", task.ID)
	fmt.Printf("  Subject: %s\n", task.Subject)
	fmt.Printf("  Status: %s\n", task.Status)
	fmt.Printf("  Owner: %s\n", emptyAs(task.Owner, "-"))
}

func printTaskDetail(store sdktasks.Store, task *sdktasks.Task) {
	if task == nil {
		failf("task is nil")
	}
	blockedBy := store.GetBlockingTasks(task.ID)
	blocks := store.GetBlockedTasks(task.ID)

	fmt.Println("Task")
	fmt.Println("====")
	fmt.Printf("ID: %s\n", task.ID)
	fmt.Printf("Subject: %s\n", task.Subject)
	fmt.Printf("Status: %s\n", task.Status)
	fmt.Printf("Owner: %s\n", emptyAs(task.Owner, "-"))
	fmt.Printf("ActiveForm: %s\n", emptyAs(task.ActiveForm, "-"))
	fmt.Printf("Description: %s\n", emptyAs(task.Description, "-"))
	fmt.Printf("BlockedBy: %s\n", joinTaskIDs(blockedBy))
	fmt.Printf("Blocks: %s\n", joinTaskIDs(blocks))
}

func joinTaskIDs(tasks []*sdktasks.Task) string {
	if len(tasks) == 0 {
		return "-"
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || strings.TrimSpace(task.ID) == "" {
			continue
		}
		ids = append(ids, task.ID)
	}
	if len(ids) == 0 {
		return "-"
	}
	return strings.Join(ids, ", ")
}

func parseTaskStatus(raw string) (sdktasks.TaskStatus, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "pending", "todo":
		return sdktasks.TaskPending, nil
	case "in_progress", "in-progress", "doing":
		return sdktasks.TaskInProgress, nil
	case "completed", "done":
		return sdktasks.TaskCompleted, nil
	case "blocked":
		return sdktasks.TaskBlocked, nil
	default:
		return "", fmt.Errorf("invalid task status: %s (allowed: pending|in_progress|completed|blocked; aliases: todo|doing|done)", raw)
	}
}

func replaceBlockedBy(store sdktasks.Store, taskID string, existing []string, desired []string) error {
	existingSet := toSet(existing)
	desiredSet := toSet(desired)

	for blockerID := range existingSet {
		if _, keep := desiredSet[blockerID]; keep {
			continue
		}
		if err := store.RemoveDependency(taskID, blockerID); err != nil {
			return err
		}
	}
	for blockerID := range desiredSet {
		if _, exists := existingSet[blockerID]; exists {
			continue
		}
		if err := store.AddDependency(taskID, blockerID); err != nil {
			return err
		}
	}
	return nil
}

func replaceBlocks(store sdktasks.Store, blockerID string, existing []string, desired []string) error {
	existingSet := toSet(existing)
	desiredSet := toSet(desired)

	for blockedID := range existingSet {
		if _, keep := desiredSet[blockedID]; keep {
			continue
		}
		if err := store.RemoveDependency(blockedID, blockerID); err != nil {
			return err
		}
	}
	for blockedID := range desiredSet {
		if _, exists := existingSet[blockedID]; exists {
			continue
		}
		if err := store.AddDependency(blockedID, blockerID); err != nil {
			return err
		}
	}
	return nil
}

func toSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	return set
}

func splitCSV(raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func formatSlice(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func openTaskStore() (*tasksdk.SQLiteStore, error) {
	workspace, err := getWorkspacePath()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(workspace, "data", "agentsdk_tasks.db")
	return tasksdk.NewSQLiteStore(dbPath)
}

func openTaskTracker(store sdktasks.Store) (*tasksdk.Tracker, error) {
	workspace, err := getWorkspacePath()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(workspace, "data", "subagent_task_tracker.db")
	return tasksdk.NewTracker(store, dbPath)
}

func getWorkspacePath() (string, error) {
	cfg, err := config.Load("")
	if err != nil {
		return "", err
	}
	return config.GetWorkspacePath(cfg)
}

func failf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
