package tasksdk

import (
	"path/filepath"
	"testing"

	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
)

func TestSQLiteStorePersistsSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() failed: %v", err)
	}

	parent, err := store.Create("parent", "desc", "implement parent")
	if err != nil {
		t.Fatalf("Create(parent) failed: %v", err)
	}
	child, err := store.Create("child", "desc", "implement child")
	if err != nil {
		t.Fatalf("Create(child) failed: %v", err)
	}
	if err := store.AddDependency(child.ID, parent.ID); err != nil {
		t.Fatalf("AddDependency failed: %v", err)
	}

	completed := sdktasks.TaskCompleted
	if _, err := store.Update(parent.ID, sdktasks.TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("Update(parent) failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	reopened, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer reopened.Close()

	list := reopened.List()
	if len(list) != 2 {
		t.Fatalf("List length = %d, want 2", len(list))
	}
	if list[0].ID != parent.ID || list[1].ID != child.ID {
		t.Fatalf("unexpected task order after reopen: %+v", list)
	}

	gotParent, err := reopened.Get(parent.ID)
	if err != nil {
		t.Fatalf("Get(parent) failed: %v", err)
	}
	if gotParent.Status != sdktasks.TaskCompleted {
		t.Fatalf("parent status = %s, want %s", gotParent.Status, sdktasks.TaskCompleted)
	}

	gotChild, err := reopened.Get(child.ID)
	if err != nil {
		t.Fatalf("Get(child) failed: %v", err)
	}
	if len(gotChild.BlockedBy) != 1 || gotChild.BlockedBy[0] != parent.ID {
		t.Fatalf("child blockedBy = %+v, want [%s]", gotChild.BlockedBy, parent.ID)
	}
	if gotChild.Status != sdktasks.TaskPending {
		t.Fatalf("child status = %s, want %s", gotChild.Status, sdktasks.TaskPending)
	}
}
