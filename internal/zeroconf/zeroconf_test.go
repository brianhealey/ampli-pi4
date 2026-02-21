package zeroconf_test

import (
	"context"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/zeroconf"
)

// TestNew verifies that New returns a non-nil service without panicking.
func TestNew(t *testing.T) {
	svc := zeroconf.New("amplipi-test", 8080)
	if svc == nil {
		t.Fatal("New() returned nil")
	}
}

// TestStart_Cancel starts the service and cancels the context within 1 second.
// It verifies that Start returns without blocking.
func TestStart_Cancel(t *testing.T) {
	svc := zeroconf.New("amplipi-test", 18080)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- svc.Start(ctx)
	}()

	select {
	case err := <-done:
		// Start may return an error if mDNS is unavailable in the test environment;
		// that is acceptable — what matters is that it returned.
		if err != nil {
			t.Logf("Start returned error (may be expected in CI): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return within 3 seconds after context cancellation")
	}
}

// TestUpdateTXT verifies that UpdateTXT does not panic when server is nil.
func TestUpdateTXT_BeforeStart(t *testing.T) {
	svc := zeroconf.New("amplipi-test", 18080)
	err := svc.UpdateTXT([]string{"version=test"})
	if err == nil {
		t.Error("UpdateTXT before Start should return an error")
	}
}
