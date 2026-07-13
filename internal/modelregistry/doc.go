// Package modelregistry реализует контракт ModelRegistryHook (ADR-9, контракт 6)
// — зеркало in-memory реестра моделей ядра в Postgres.
//
// Источник истины моделей — ядро; бизнес-слой только зеркалирует для
// UI/model-mapping и применяет allow-list (R9.A.6) в Selector.
//
// Имплементация: Фаза 3 (Contracts ADR-9).
package modelregistry
