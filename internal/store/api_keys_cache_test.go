package store

import "testing"

func TestAPIKeyRepositoryExposesCandidateCacheStats(t *testing.T) {
	repository := NewAPIKeyRepository(nil)
	stats := repository.CacheStats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("CacheStats() = %+v, want zero snapshot", stats)
	}
}
