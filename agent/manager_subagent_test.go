package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	agentruntime "github.com/smallnest/goclaw/agent/runtime"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/config"
)

type mockSubagentRuntime struct {
	mu         sync.Mutex
	spawnCalls int
	spawnReq   agentruntime.SubagentRunRequest

	waitCalled chan string
	waitResult *agentruntime.SubagentRunResult
	waitErr    error
}

type mockTaskStore struct {
	mu          sync.Mutex
	statusByID  map[string]string
	progressLog []TaskProgressInput
	runToTask   map[string]string
}

func newMockTaskStore() *mockTaskStore {
	return &mockTaskStore{
		statusByID:  make(map[string]string),
		progressLog: make([]TaskProgressInput, 0),
		runToTask:   make(map[string]string),
	}
}

func (m *mockTaskStore) UpdateTaskStatus(taskID string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusByID[taskID] = status
	return nil
}

func (m *mockTaskStore) AppendTaskProgress(input TaskProgressInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progressLog = append(m.progressLog, input)
	return nil
}

func (m *mockTaskStore) LinkSubagentRun(runID, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runToTask[runID] = taskID
	return nil
}

func (m *mockTaskStore) ResolveTaskByRun(runID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runToTask[runID], nil
}

func (m *mockSubagentRuntime) Spawn(_ context.Context, req agentruntime.SubagentRunRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spawnCalls++
	m.spawnReq = req
	return req.RunID, nil
}

func (m *mockSubagentRuntime) Wait(_ context.Context, runID string) (*agentruntime.SubagentRunResult, error) {
	if m.waitCalled != nil {
		m.waitCalled <- runID
	}
	return m.waitResult, m.waitErr
}

func (m *mockSubagentRuntime) Cancel(_ context.Context, _ string) error {
	return nil
}

func TestHandleSubagentSpawnBuildsRuntimeRequestAndMarksCompleted(t *testing.T) {
	tmp := t.TempDir()
	runtime := &mockSubagentRuntime{
		waitCalled: make(chan string, 1),
		waitResult: &agentruntime.SubagentRunResult{
			Status: agentruntime.RunStatusOK,
			Output: "frontend task done",
		},
	}
	taskStore := newMockTaskStore()

	mgr := &AgentManager{
		subagentRegistry: NewSubagentRegistry(tmp),
		subagentRuntime:  runtime,
		taskStore:        taskStore,
		workspace:        tmp,
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Subagents: &config.SubagentsConfig{
						SkillsRoleDir:  "skills",
						WorkdirBase:    "subagents",
						TimeoutSeconds: 123,
					},
				},
			},
		},
	}

	err := mgr.subagentRegistry.RegisterRun(&SubagentRunParams{
		RunID:               "run-1",
		ChildSessionKey:     "agent:default:subagent:abc",
		RequesterSessionKey: "telegram:bot1:chat42",
		Task:                "[frontend] build login page",
		TaskID:              "task-1",
		Cleanup:             "keep",
		ArchiveAfterMinutes: 60,
	})
	if err != nil {
		t.Fatalf("RegisterRun() failed: %v", err)
	}

	if err := mgr.handleSubagentSpawn(&tools.SubagentSpawnResult{
		RunID:           "run-1",
		ChildSessionKey: "agent:default:subagent:abc",
	}); err != nil {
		t.Fatalf("handleSubagentSpawn() failed: %v", err)
	}

	select {
	case gotRunID := <-runtime.waitCalled:
		if gotRunID != "run-1" {
			t.Fatalf("runtime.Wait runID = %q, want %q", gotRunID, "run-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runtime.Wait was not called")
	}

	runtime.mu.Lock()
	spawnReq := runtime.spawnReq
	spawnCalls := runtime.spawnCalls
	runtime.mu.Unlock()

	if spawnCalls != 1 {
		t.Fatalf("spawn call count = %d, want 1", spawnCalls)
	}
	if spawnReq.Role != agentruntime.RoleFrontend {
		t.Fatalf("spawn role = %q, want %q", spawnReq.Role, agentruntime.RoleFrontend)
	}
	if spawnReq.TimeoutSeconds != 123 {
		t.Fatalf("spawn timeout = %d, want 123", spawnReq.TimeoutSeconds)
	}
	wantRepoDir := filepath.Join(tmp, "subagents", "run-1", "repo")
	if spawnReq.RepoDir != wantRepoDir {
		t.Fatalf("spawn repo dir = %q, want %q", spawnReq.RepoDir, wantRepoDir)
	}
	if spawnReq.GoClawDir != tmp {
		t.Fatalf("spawn goclaw dir = %q, want %q", spawnReq.GoClawDir, tmp)
	}
	if spawnReq.MCPConfigPath != "" {
		t.Fatalf("spawn mcp config path = %q, want empty", spawnReq.MCPConfigPath)
	}
	wantRoleDir := filepath.Join(tmp, "skills", agentruntime.RoleFrontend)
	if spawnReq.RoleDir != wantRoleDir {
		t.Fatalf("spawn role dir = %q, want %q", spawnReq.RoleDir, wantRoleDir)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		record, ok := mgr.subagentRegistry.GetRun("run-1")
		if !ok {
			t.Fatalf("run record disappeared unexpectedly")
		}
		if record.Outcome != nil {
			if record.Outcome.Status != agentruntime.RunStatusOK {
				t.Fatalf("outcome status = %q, want %q", record.Outcome.Status, agentruntime.RunStatusOK)
			}
			if record.Outcome.Result != "frontend task done" {
				t.Fatalf("outcome result = %q, want %q", record.Outcome.Result, "frontend task done")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run outcome not updated before timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		taskStore.mu.Lock()
		status := taskStore.statusByID["task-1"]
		progressCount := len(taskStore.progressLog)
		linkedTaskID := taskStore.runToTask["run-1"]
		taskStore.mu.Unlock()

		if status == taskStatusCompleted && progressCount >= 2 && linkedTaskID == "task-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task store was not updated as expected: status=%s progress=%d linkedTaskID=%q", status, progressCount, linkedTaskID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestParseRequesterSessionKey(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		channel   string
		accountID string
		chatID    string
	}{
		{
			name:      "full triple",
			in:        "telegram:bot1:chat42",
			channel:   "telegram",
			accountID: "bot1",
			chatID:    "chat42",
		},
		{
			name:      "double fallback chat",
			in:        "cli:default",
			channel:   "cli",
			accountID: "default",
			chatID:    "default",
		},
		{
			name:      "single fallback",
			in:        "standalone-chat",
			channel:   "cli",
			accountID: "default",
			chatID:    "standalone-chat",
		},
		{
			name:      "empty fallback all",
			in:        "",
			channel:   "cli",
			accountID: "default",
			chatID:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel, accountID, chatID := parseRequesterSessionKey(tt.in)
			if channel != tt.channel || accountID != tt.accountID || chatID != tt.chatID {
				t.Fatalf("parseRequesterSessionKey(%q) = (%q,%q,%q), want (%q,%q,%q)",
					tt.in, channel, accountID, chatID, tt.channel, tt.accountID, tt.chatID)
			}
		})
	}
}

func TestNormalizeRuntimeStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "ok", want: agentruntime.RunStatusOK},
		{in: "timeout", want: agentruntime.RunStatusTimeout},
		{in: "error", want: agentruntime.RunStatusError},
		{in: "UNKNOWN", want: agentruntime.RunStatusError},
	}

	for _, tt := range tests {
		got := normalizeRuntimeStatus(tt.in)
		if got != tt.want {
			t.Fatalf("normalizeRuntimeStatus(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
