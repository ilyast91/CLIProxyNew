// Package modelregistry реализует контракт ModelRegistryHook (ADR-9, контракт 6)
// — зеркало in-memory реестра моделей ядра в Postgres как JSON snapshot.
//
// Источник истины моделей — ядро; бизнес-слой только зеркалирует для
// UI/model-mapping. Allow-list (R9.A.6) применяется отдельно в Selector.
package modelregistry
