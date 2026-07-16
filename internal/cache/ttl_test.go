package cache

import (
	"testing"
	"time"
)

func TestTTLCacheExpiresAndDeletesEntries(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	cache := NewTTL[string, int](10*time.Second, func() time.Time { return now })
	cache.Set("key", 42)

	if value, ok := cache.Get("key"); !ok || value != 42 {
		t.Fatalf("Get() = %d, %t", value, ok)
	}
	now = now.Add(10 * time.Second)
	if _, ok := cache.Get("key"); ok {
		t.Fatal("Get() returned expired entry")
	}

	cache.Set("key", 7)
	cache.Delete("key")
	if _, ok := cache.Get("key"); ok {
		t.Fatal("Get() returned deleted entry")
	}

	cache.Set("first", 1)
	cache.Set("second", 2)
	cache.Clear()
	if _, ok := cache.Get("first"); ok {
		t.Fatal("Get() returned first entry after Clear()")
	}
	if _, ok := cache.Get("second"); ok {
		t.Fatal("Get() returned second entry after Clear()")
	}
}

func TestTTLCacheRejectsNonPositiveTTL(t *testing.T) {
	cache := NewTTL[string, int](0, time.Now)
	cache.Set("key", 42)
	if _, ok := cache.Get("key"); ok {
		t.Fatal("Get() returned value for disabled cache")
	}
}

func TestTTLCacheReportsHitAndMissCounts(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	cache := NewTTL[string, int](time.Second, func() time.Time { return now })
	cache.Set("cached", 1)

	if _, ok := cache.Get("cached"); !ok {
		t.Fatal("Get(cached) did not return cached entry")
	}
	if _, ok := cache.Get("unknown"); ok {
		t.Fatal("Get(unknown) returned an entry")
	}
	now = now.Add(time.Second)
	if _, ok := cache.Get("cached"); ok {
		t.Fatal("Get(cached) returned expired entry")
	}

	stats := cache.Stats()
	if stats.Hits != 1 || stats.Misses != 2 {
		t.Fatalf("Stats() = %+v, want hits=1 misses=2", stats)
	}
}
