package runtime

import (
	"context"
	"testing"
	"time"
)

func TestSimpleRolePoolAcquireRelease(t *testing.T) {
	pool := NewSimpleRolePool(1, map[string]int{
		RoleBackend: 1,
	})

	if err := pool.Acquire(context.Background(), RoleBackend); err != nil {
		t.Fatalf("first Acquire() failed: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := pool.Acquire(timeoutCtx, RoleBackend); err == nil {
		t.Fatalf("second Acquire() should timeout when token is not released")
	}

	pool.Release(RoleBackend)

	if err := pool.Acquire(context.Background(), RoleBackend); err != nil {
		t.Fatalf("Acquire() after Release() failed: %v", err)
	}
}

func TestSimpleRolePoolRoleSpecificLimits(t *testing.T) {
	pool := NewSimpleRolePool(1, map[string]int{
		RoleFrontend: 2,
		RoleBackend:  1,
	})

	if err := pool.Acquire(context.Background(), RoleFrontend); err != nil {
		t.Fatalf("frontend acquire #1 failed: %v", err)
	}
	if err := pool.Acquire(context.Background(), RoleFrontend); err != nil {
		t.Fatalf("frontend acquire #2 failed: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := pool.Acquire(timeoutCtx, RoleFrontend); err == nil {
		t.Fatalf("frontend acquire #3 should timeout because limit is 2")
	}

	if err := pool.Acquire(context.Background(), RoleBackend); err != nil {
		t.Fatalf("backend acquire #1 failed: %v", err)
	}
	timeoutCtx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if err := pool.Acquire(timeoutCtx2, RoleBackend); err == nil {
		t.Fatalf("backend acquire #2 should timeout because limit is 1")
	}
}
