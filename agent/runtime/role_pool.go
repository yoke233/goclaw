package runtime

import (
	"context"
	"sync"
)

// RolePool 角色并发池接口。
type RolePool interface {
	Acquire(ctx context.Context, role string) error
	Release(role string)
}

// SimpleRolePool 使用 role -> semaphore 的简单实现。
type SimpleRolePool struct {
	mu           sync.RWMutex
	defaultLimit int
	roleLimits   map[string]int
	semaphores   map[string]chan struct{}
}

// NewSimpleRolePool 创建角色并发池。
// defaultLimit <= 0 时回退为 1。
func NewSimpleRolePool(defaultLimit int, roleLimits map[string]int) *SimpleRolePool {
	if defaultLimit <= 0 {
		defaultLimit = 1
	}

	limits := make(map[string]int)
	for role, limit := range roleLimits {
		if limit <= 0 {
			continue
		}
		limits[NormalizeRole(role)] = limit
	}

	return &SimpleRolePool{
		defaultLimit: defaultLimit,
		roleLimits:   limits,
		semaphores:   make(map[string]chan struct{}),
	}
}

func (p *SimpleRolePool) Acquire(ctx context.Context, role string) error {
	sem := p.getSemaphore(role)
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *SimpleRolePool) Release(role string) {
	sem := p.getSemaphore(role)
	select {
	case <-sem:
	default:
	}
}

func (p *SimpleRolePool) getSemaphore(role string) chan struct{} {
	normalized := NormalizeRole(role)

	p.mu.RLock()
	sem, ok := p.semaphores[normalized]
	p.mu.RUnlock()
	if ok {
		return sem
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if sem, ok = p.semaphores[normalized]; ok {
		return sem
	}

	limit := p.defaultLimit
	if n, exists := p.roleLimits[normalized]; exists && n > 0 {
		limit = n
	}

	sem = make(chan struct{}, limit)
	p.semaphores[normalized] = sem
	return sem
}
