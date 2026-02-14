package session

import (
	"testing"
	"time"
)

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize:      2,
		DefaultTTL:   time.Minute,
		CleanupIntvl: time.Hour,
	})
	defer cache.Close()

	cache.Set("a", buildSessionWithMessages("a", 1))
	cache.Set("b", buildSessionWithMessages("b", 1))
	if _, ok := cache.Get("a"); !ok {
		t.Fatalf("expected cache hit for key a")
	}

	cache.Set("c", buildSessionWithMessages("c", 1))

	if cache.Contains("b") {
		t.Fatalf("expected key b to be evicted as LRU")
	}
	if !cache.Contains("a") || !cache.Contains("c") {
		t.Fatalf("expected keys a and c to remain in cache")
	}

	stats := cache.Stats()
	if stats.Evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCacheExpiredEntryReturnsMiss(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize:      4,
		DefaultTTL:   time.Minute,
		CleanupIntvl: time.Hour,
	})
	defer cache.Close()

	cache.SetWithTTL("k", buildSessionWithMessages("k", 1), 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	if _, ok := cache.Get("k"); ok {
		t.Fatalf("expected cache miss for expired key")
	}
	if cache.Contains("k") {
		t.Fatalf("expired key should be removed from cache after Get")
	}

	stats := cache.Stats()
	if stats.Expirations != 1 {
		t.Fatalf("expected 1 expiration, got %d", stats.Expirations)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestCachePruneByAccessCount(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize:      10,
		DefaultTTL:   time.Minute,
		CleanupIntvl: time.Hour,
	})
	defer cache.Close()

	cache.Set("a", buildSessionWithMessages("a", 1))
	cache.Set("b", buildSessionWithMessages("b", 1))
	cache.Set("c", buildSessionWithMessages("c", 1))

	_, _ = cache.Get("a")
	_, _ = cache.Get("b")

	pruned := cache.PruneByAccessCount(2)
	if pruned != 1 {
		t.Fatalf("expected to prune 1 key, got %d", pruned)
	}

	if cache.Contains("c") {
		t.Fatalf("expected key c to be pruned due to low access count")
	}
	if !cache.Contains("a") || !cache.Contains("b") {
		t.Fatalf("expected keys a and b to remain")
	}
}

func TestCacheContainsShouldReturnFalseForExpiredEntry(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize:      4,
		DefaultTTL:   time.Minute,
		CleanupIntvl: time.Hour,
	})
	defer cache.Close()

	cache.SetWithTTL("expired", buildSessionWithMessages("expired", 1), 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	if cache.Contains("expired") {
		t.Fatalf("expected Contains to report false for expired entry")
	}
}

func TestCacheKeysShouldNotIncludeExpiredEntry(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize:      4,
		DefaultTTL:   time.Minute,
		CleanupIntvl: time.Hour,
	})
	defer cache.Close()

	cache.Set("active", buildSessionWithMessages("active", 1))
	cache.SetWithTTL("expired", buildSessionWithMessages("expired", 1), 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	keys := cache.Keys()
	if len(keys) != 1 || keys[0] != "active" {
		t.Fatalf("expected only active key after expiration, got %v", keys)
	}
}
