package watcher

import (
	"context"
	"testing"
)

func TestNoopFactoryReturnsUsableWatcherWrapper(t *testing.T) {
	wrapper, err := NoopFactory("", "", nil)
	if err != nil || wrapper == nil {
		t.Fatalf("NoopFactory() = %v, %v", wrapper, err)
	}
	if err := wrapper.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
