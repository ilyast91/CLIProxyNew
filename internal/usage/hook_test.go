package usage

import (
	"context"
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestHookCountsAuthLifecycleAndResults(t *testing.T) {
	hook := NewHook()
	hook.OnAuthRegistered(context.Background(), &coreauth.Auth{ID: "auth-1"})
	hook.OnAuthUpdated(context.Background(), &coreauth.Auth{ID: "auth-1"})
	hook.OnResult(context.Background(), coreauth.Result{Success: true})
	hook.OnResult(context.Background(), coreauth.Result{Success: false})

	got := hook.Snapshot()
	want := HookSnapshot{AuthRegistered: 1, AuthUpdated: 1, Results: 2, Succeeded: 1, Failed: 1}
	if got != want {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}
