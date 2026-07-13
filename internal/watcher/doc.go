// Package watcher реализует контракт WatcherFactory (ADR-9, контракт 5) —
// poll БД (вместо файловой системы) + leader election (advisory lock, R7).
//
// Пушит watcher.AuthUpdate-обновления в очередь ядра. В multi-replica
// poller работает только на лидере.
//
// Имплементация: Фаза 3 (Contracts ADR-9).
package watcher
