package events

import (
	"surge-controller/internal/database"
	"sync"
)

type Bus struct {
	mu          sync.Mutex
	subscribers map[chan database.Event]bool
}

func New() *Bus { return &Bus{subscribers: map[chan database.Event]bool{}} }
func (b *Bus) Publish(e database.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			close(ch)
			delete(b.subscribers, ch)
		}
	}
}
func (b *Bus) Subscribe() (chan database.Event, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subscribers) >= 64 {
		return nil, false
	}
	ch := make(chan database.Event, 32)
	b.subscribers[ch] = true
	return ch, true
}
func (b *Bus) Unsubscribe(ch chan database.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers[ch] {
		delete(b.subscribers, ch)
		close(ch)
	}
}
