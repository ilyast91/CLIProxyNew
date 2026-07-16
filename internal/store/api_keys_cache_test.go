package store

import (
	"crypto/sha256"
	"testing"
)

func TestAPIKeyRepositoryExposesVerifiedCacheStats(t *testing.T) {
	repository := NewAPIKeyRepository(nil)
	stats := repository.CacheStats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("CacheStats() = %+v, want zero snapshot", stats)
	}
}

func TestAPIKeyRepositoryInvalidatesVerifiedCacheByUser(t *testing.T) {
	repository := NewAPIKeyRepository(nil)
	firstKey := verifiedAPIKeyCacheKey{digest: sha256.Sum256([]byte("first")), identitySource: "static"}
	secondKey := verifiedAPIKeyCacheKey{digest: sha256.Sum256([]byte("second")), identitySource: "static"}
	repository.verifiedCache.Set(firstKey, APIKeyPrincipal{UserID: 7, APIKeyID: 70})
	repository.verifiedCache.Set(secondKey, APIKeyPrincipal{UserID: 8, APIKeyID: 80})

	repository.InvalidateUser(7)

	if _, ok := repository.verifiedCache.Get(firstKey); ok {
		t.Fatal("verified cache returned invalidated user")
	}
	if principal, ok := repository.verifiedCache.Get(secondKey); !ok || principal.UserID != 8 {
		t.Fatalf("verified cache returned other user = %+v, %t", principal, ok)
	}
}
