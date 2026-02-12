package tasksdk

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdktasks "github.com/cexll/agentsdk-go/pkg/runtime/tasks"
	_ "github.com/glebarez/sqlite"
)

// SQLiteStore persists agentsdk task store state into a single SQLite snapshot row.
type SQLiteStore struct {
	mu    sync.Mutex
	db    *sql.DB
	inner *sdktasks.TaskStore
}

var _ sdktasks.Store = (*SQLiteStore)(nil)

// NewSQLiteStore creates a persistent agentsdk task store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	path := strings.TrimSpace(dbPath)
	if path == "" {
		return nil, fmt.Errorf("task sqlite path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create task sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open task sqlite: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	snapshot, err := store.loadSnapshot()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	store.inner = sdktasks.NewTaskStoreFromSnapshot(snapshot)
	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS sdk_task_store_snapshot (
  id INTEGER PRIMARY KEY CHECK(id = 1),
  payload_json TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("init task sqlite schema: %w", err)
	}
	return nil
}

func (s *SQLiteStore) loadSnapshot() ([]*sdktasks.Task, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload_json FROM sdk_task_store_snapshot WHERE id = 1`).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load task sqlite snapshot: %w", err)
	}

	var list []*sdktasks.Task
	if err := json.Unmarshal([]byte(payload), &list); err != nil {
		return nil, fmt.Errorf("decode task sqlite snapshot: %w", err)
	}
	return list, nil
}

func (s *SQLiteStore) saveSnapshot(snapshot []*sdktasks.Task) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode task sqlite snapshot: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO sdk_task_store_snapshot(id, payload_json, updated_at)
     VALUES(1, ?, ?)
     ON CONFLICT(id) DO UPDATE SET payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		string(payload),
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("save task sqlite snapshot: %w", err)
	}
	return nil
}

func (s *SQLiteStore) withMutation(fn func(inner *sdktasks.TaskStore) error) error {
	before := s.inner.Snapshot()
	if err := fn(s.inner); err != nil {
		return err
	}
	after := s.inner.Snapshot()
	if err := s.saveSnapshot(after); err != nil {
		s.inner = sdktasks.NewTaskStoreFromSnapshot(before)
		return err
	}
	return nil
}

func (s *SQLiteStore) Create(subject, description, activeForm string) (*sdktasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var created *sdktasks.Task
	err := s.withMutation(func(inner *sdktasks.TaskStore) error {
		task, err := inner.Create(subject, description, activeForm)
		if err != nil {
			return err
		}
		created = task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *SQLiteStore) Get(id string) (*sdktasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Get(id)
}

func (s *SQLiteStore) Update(id string, updates sdktasks.TaskUpdate) (*sdktasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updated *sdktasks.Task
	err := s.withMutation(func(inner *sdktasks.TaskStore) error {
		task, err := inner.Update(id, updates)
		if err != nil {
			return err
		}
		updated = task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *SQLiteStore) List() []*sdktasks.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.List()
}

func (s *SQLiteStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withMutation(func(inner *sdktasks.TaskStore) error {
		return inner.Delete(id)
	})
}

func (s *SQLiteStore) AddDependency(taskID, blockedByID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withMutation(func(inner *sdktasks.TaskStore) error {
		return inner.AddDependency(taskID, blockedByID)
	})
}

func (s *SQLiteStore) RemoveDependency(taskID, blockedByID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withMutation(func(inner *sdktasks.TaskStore) error {
		return inner.RemoveDependency(taskID, blockedByID)
	})
}

func (s *SQLiteStore) GetBlockedTasks(taskID string) []*sdktasks.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.GetBlockedTasks(taskID)
}

func (s *SQLiteStore) GetBlockingTasks(taskID string) []*sdktasks.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.GetBlockingTasks(taskID)
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
