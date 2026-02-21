package events_test

import (
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/models"
)

func TestBusSubscribePublish(t *testing.T) {
	bus := events.NewBus()

	ch := bus.Subscribe("test1")

	state := models.DefaultState()
	state.Info.Version = "test-1.0"

	bus.Publish(state)

	select {
	case got := <-ch:
		if got.Info.Version != "test-1.0" {
			t.Errorf("got version %q, want %q", got.Info.Version, "test-1.0")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := events.NewBus()
	ch := bus.Subscribe("test-unsub")

	bus.Unsubscribe("test-unsub")

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBusDropsEventsWhenFull(t *testing.T) {
	bus := events.NewBus()
	ch := bus.Subscribe("slow-reader")

	// Publish many events without reading — should not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			bus.Publish(models.DefaultState())
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked for too long (should drop events)")
	}

	bus.Unsubscribe("slow-reader")
	_ = ch
}

func TestBusSubscriberCount(t *testing.T) {
	bus := events.NewBus()
	if n := bus.SubscriberCount(); n != 0 {
		t.Errorf("expected 0 subscribers, got %d", n)
	}
	bus.Subscribe("s1")
	bus.Subscribe("s2")
	if n := bus.SubscriberCount(); n != 2 {
		t.Errorf("expected 2 subscribers, got %d", n)
	}
	bus.Unsubscribe("s1")
	if n := bus.SubscriberCount(); n != 1 {
		t.Errorf("expected 1 subscriber, got %d", n)
	}
}
