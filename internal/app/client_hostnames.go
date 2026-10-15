package app

import (
	"context"
	"net/netip"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"sync"
	"time"
)

const (
	hostnameProbeInterval = 5 * time.Second
	hostnameProbeTimeout  = 2 * time.Second
	hostnameSuccessTTL    = time.Hour
	hostnameFailureTTL    = 5 * time.Minute
	hostnameProbeBatch    = 8
)

type clientHostname struct {
	name, error string
	retryAt     time.Time
}

// One worker per enabled runtime, independent of request handling and UI polling.
// Results are ephemeral display hints; they never change client configuration.
type clientHostnames struct {
	mu      sync.RWMutex
	entries map[string]clientHostname
	named   map[string]bool
	lookup  func(context.Context, string) ([]string, error)
	done    chan struct{}
	cursor  int
}

func newClientHostnames(clients []config.Client, lookup func(context.Context, string) ([]string, error)) *clientHostnames {
	h := &clientHostnames{entries: make(map[string]clientHostname), named: make(map[string]bool), lookup: lookup, done: make(chan struct{})}
	for _, c := range clients {
		if strings.TrimSpace(c.Name) != "" {
			if ip, err := netip.ParseAddr(c.IP); err == nil {
				h.named[ip.Unmap().String()] = true
			}
		}
	}
	return h
}

func (h *clientHostnames) run(ctx context.Context, clients *dnsserver.ClientStatistics) {
	defer close(h.done)
	ticker := time.NewTicker(hostnameProbeInterval)
	defer ticker.Stop()
	for {
		h.probe(ctx, clients.Snapshot().Clients, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *clientHostnames) probe(ctx context.Context, clients []dnsserver.ClientStats, now time.Time) {
	active := make(map[string]bool, len(clients))
	for _, client := range clients {
		ip, err := netip.ParseAddr(client.ClientIP)
		if err == nil && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast() {
			key := ip.Unmap().String()
			if !h.named[key] {
				active[key] = true
			}
		}
	}
	h.mu.Lock()
	for ip := range h.entries {
		if !active[ip] {
			delete(h.entries, ip)
		}
	}
	h.mu.Unlock()
	remaining := hostnameProbeBatch
	start := h.cursor
	for offset := 0; offset < len(clients); offset++ {
		index := (start + offset) % len(clients)
		client := clients[index]
		ip, err := netip.ParseAddr(client.ClientIP)
		if err != nil {
			continue
		}
		key := ip.Unmap().String()
		if !active[key] {
			continue
		}
		h.mu.RLock()
		cached := h.entries[key]
		h.mu.RUnlock()
		if now.Before(cached.retryAt) {
			continue
		}
		if ctx.Err() != nil || remaining == 0 {
			return
		}
		remaining--
		h.cursor = (index + 1) % len(clients)
		lookupCtx, cancel := context.WithTimeout(ctx, hostnameProbeTimeout)
		names, lookupErr := h.lookup(lookupCtx, key)
		cancel()
		if ctx.Err() != nil {
			return
		}
		result := clientHostname{retryAt: now.Add(hostnameFailureTTL)}
		if lookupErr != nil {
			result.error = lookupErr.Error()
		} else {
			for _, name := range names {
				value, err := model.NormalizeDomain(name)
				if err == nil && value != "." {
					result.name = strings.TrimSuffix(value, ".")
					result.retryAt = now.Add(hostnameSuccessTTL)
					break
				}
			}
			if result.name == "" {
				result.error = "No usable PTR record"
			}
		}
		h.mu.Lock()
		h.entries[key] = result
		h.mu.Unlock()
	}
}

func (h *clientHostnames) decorate(snapshot *dnsserver.ClientStatsSnapshot) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := time.Now()
	for i := range snapshot.Clients {
		entry := h.entries[snapshot.Clients[i].ClientIP]
		if now.Before(entry.retryAt) {
			snapshot.Clients[i].Hostname = entry.name
			snapshot.Clients[i].HostnameError = entry.error
		}
	}
}
