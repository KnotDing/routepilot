package events

import (
	"surge-controller/internal/database"
	"sync"
	"testing"
)

func TestSubscriberLimitAndSlowConsumerEviction(t *testing.T) {
	b := New()
	var subscribers []chan database.Event
	for range 64 {
		ch, ok := b.Subscribe()
		if !ok {
			t.Fatal("early subscriber rejection")
		}
		subscribers = append(subscribers, ch)
	}
	if ch, ok := b.Subscribe(); ok || ch != nil {
		t.Fatal("subscriber limit ignored")
	}
	for range 33 {
		b.Publish(database.Event{})
	}
	for _, ch := range subscribers {
		count := 0
		for range ch {
			count++
		}
		if count != 32 {
			t.Fatal("bounded event queue changed", count)
		}
		b.Unsubscribe(ch)
	}
	if _, ok := b.Subscribe(); !ok {
		t.Fatal("evicted subscriber leaked slot")
	}
}
func TestConcurrentPublishSubscribeAndUnsubscribe(t *testing.T) {
	b := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				ch, ok := b.Subscribe()
				if ok {
					b.Publish(database.Event{})
					b.Unsubscribe(ch)
					b.Unsubscribe(ch)
				}
			}
		}()
	}
	wg.Wait()
	if len(b.subscribers) != 0 {
		t.Fatal("subscriber leaked")
	}
}
