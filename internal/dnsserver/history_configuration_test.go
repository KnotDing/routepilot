package dnsserver

import (
	"fmt"
	"sync"
	"testing"
)

func TestHistoryHotResizePrivacyAndStaleWriters(t *testing.T) {
	h := NewHistory(3, true)
	add := func(value string) { h.add(QueryRecord{Resolver: value}, h.generation.Load()) }
	for i := 0; i < 5; i++ {
		add(fmt.Sprint(i))
	}
	h.Configure(2, true)
	rows := h.Records()
	if len(rows) != 2 || rows[0].Resolver != "4" || rows[1].Resolver != "3" {
		t.Fatalf("resize did not keep newest records: %+v", rows)
	}
	h.Configure(5, true)
	add("5")
	if rows := h.Records(); len(rows) != 3 || rows[0].Resolver != "5" {
		t.Fatal("grow lost retained records")
	}
	stale := h.generation.Load()
	h.Configure(5, false)
	if len(h.Records()) != 0 {
		t.Fatal("privacy disable retained previous records")
	}
	h.Configure(5, true)
	h.add(QueryRecord{Resolver: "stale"}, stale)
	if len(h.Records()) != 0 {
		t.Fatal("old writer repopulated a cleared history")
	}
	add("new")
	if len(h.Records()) != 1 {
		t.Fatal("new generation failed to record")
	}
}

func TestHistoryConfigureWithConcurrentRequests(t *testing.T) {
	h := NewHistory(16, true)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				h.add(QueryRecord{}, h.generation.Load())
				h.Records()
			}
		}()
	}
	for i := 0; i < 100; i++ {
		h.Configure(1+i%16, i%2 == 0)
	}
	wg.Wait()
	h.Configure(16, false)
	if len(h.Records()) != 0 {
		t.Fatal("disabled history retained concurrent records")
	}
}
