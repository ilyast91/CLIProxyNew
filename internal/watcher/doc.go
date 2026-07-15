// Package watcher реализует контракт WatcherFactory (ADR-9, контракт 5) —
// отключает file-backed watcher SDK и синхронизирует auth через DB revision
// с controlled restart.
//
// Для scheduled jobs реализован advisory leader election; первый job очищает
// истёкшие sessions. Upstream AuthUpdate недоступен без импорта internal/*.
package watcher
