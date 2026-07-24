package engine

import "sync"

// Broker is a tiny per-run pub/sub used to push run updates to WebSocket
// subscribers (state trace + status changes).
type Broker struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}

// NewBroker builds an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: map[string]map[chan []byte]struct{}{}}
}

// Subscribe registers a channel for a run's updates and returns an
// unsubscribe func.
func (b *Broker) Subscribe(runID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	b.mu.Lock()
	if b.subs[runID] == nil {
		b.subs[runID] = map[chan []byte]struct{}{}
	}
	b.subs[runID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if m := b.subs[runID]; m != nil {
			delete(m, ch)
		}
		b.mu.Unlock()
		close(ch)
	}
}

// Publish delivers a message to all subscribers of a run (non-blocking).
func (b *Broker) Publish(runID string, msg []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[runID] {
		select {
		case ch <- msg:
		default:
		}
	}
}
