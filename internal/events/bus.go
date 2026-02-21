// Package events provides a simple publish-subscribe event bus for SSE delivery.
package events

import (
	"sync"

	"github.com/micro-nova/amplipi-go/internal/models"
)

const subBufferSize = 8

// Bus is a non-blocking publish-subscribe event bus.
// Subscribers that are slow to consume events will have events dropped rather
// than blocking publishers.
type Bus struct {
	mu   sync.Mutex
	subs map[string]chan models.State
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subs: make(map[string]chan models.State),
	}
}

// Subscribe creates a new subscription with the given ID.
// The returned channel will receive state updates.
// Call Unsubscribe when done to clean up.
func (b *Bus) Subscribe(id string) <-chan models.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan models.State, subBufferSize)
	b.subs[id] = ch
	return ch
}

// Unsubscribe removes a subscription and closes its channel.
func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
}

// Publish sends a state update to all subscribers.
// If a subscriber's channel is full, the event is dropped (non-blocking).
func (b *Bus) Publish(state models.State) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- state:
		default:
			// Drop if subscriber is slow
		}
	}
}

// SubscriberCount returns the current number of subscribers.
func (b *Bus) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
