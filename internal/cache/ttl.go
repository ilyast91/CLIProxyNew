package cache

import (
	"sync"
	"time"
)

type ttlEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// TTL хранит значения в памяти ограниченное время.
type TTL[K comparable, V any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[K]ttlEntry[V]
}

// NewTTL создаёт потокобезопасный TTL-кэш. Неположительный TTL отключает хранение.
func NewTTL[K comparable, V any](ttl time.Duration, now func() time.Time) *TTL[K, V] {
	if now == nil {
		now = time.Now
	}

	return &TTL[K, V]{
		ttl:     ttl,
		now:     now,
		entries: make(map[K]ttlEntry[V]),
	}
}

// Get возвращает неистёкшее значение по ключу.
func (c *TTL[K, V]) Get(key K) (V, bool) {
	var zero V
	if c == nil || c.ttl <= 0 {
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return zero, false
	}

	return entry.value, true
}

// Set сохраняет значение по ключу до истечения TTL.
func (c *TTL[K, V]) Set(key K, value V) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = ttlEntry[V]{
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	}
}

// Delete удаляет значение по ключу.
func (c *TTL[K, V]) Delete(key K) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Clear удаляет все значения кэша.
func (c *TTL[K, V]) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	clear(c.entries)
}
