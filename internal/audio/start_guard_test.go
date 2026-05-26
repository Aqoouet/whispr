package audio

import (
	"errors"
	"testing"
	"time"
)

func TestStartGuardSuppressesRetryStormDuringCooldown(t *testing.T) {
	now := time.Unix(100, 0)
	guard := startGuard{
		now:        func() time.Time { return now },
		retryDelay: time.Second,
	}
	wantErr := errors.New("capture backend failed")
	if got := guard.recordFailure(wantErr); !errors.Is(got, wantErr) {
		t.Fatalf("recordFailure error = %v", got)
	}
	if got := guard.beforeStart(); !errors.Is(got, wantErr) {
		t.Fatalf("beforeStart error = %v, want %v", got, wantErr)
	}
	now = now.Add(1100 * time.Millisecond)
	if err := guard.beforeStart(); err != nil {
		t.Fatalf("beforeStart after cooldown = %v", err)
	}
}

func TestStartGuardClearsAfterSuccessfulStart(t *testing.T) {
	guard := newStartGuard()
	wantErr := errors.New("capture backend failed")
	guard.recordFailure(wantErr)
	guard.clear()
	if err := guard.beforeStart(); err != nil {
		t.Fatalf("beforeStart error = %v", err)
	}
}
