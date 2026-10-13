package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"

	"surge-controller/internal/dnsserver"
)

type monitoringFilteredService interface {
	DNSMonitoringFilteredView(context.Context, dnsserver.MonitoringFilter) (dnsserver.DNSMonitoringSnapshot, error)
}

type monitoringService interface {
	DNSMonitoringView(context.Context, string) (dnsserver.DNSMonitoringSnapshot, error)
}

func (a *API) serveDNSMonitoring(w http.ResponseWriter, r *http.Request, path string) {
	source := r.URL.Query().Get("source")
	if source != "" && source != "unknown" && source != "other" && source != "legacy" {
		ip, err := netip.ParseAddr(source)
		if err != nil {
			fail(w, 400, errors.New("invalid monitoring source"))
			return
		}
		source = ip.Unmap().String()
	}
	query := r.URL.Query()
	filter := dnsserver.MonitoringFilter{Source: source, Destination: query.Get("destination"), Policy: query.Get("policy"), FinalPolicy: query.Get("final_policy"), Domain: dnsserver.MonitoringDomain(query.Get("domain")), ResponseCode: strings.ToUpper(query.Get("response_code"))}
	if filter.Destination != "" {
		valid := false
		for _, v := range []string{"local", "cache", "optimistic_cache", "real", "surge", "blocked", "unavailable"} {
			valid = valid || v == filter.Destination
		}
		if !valid {
			fail(w, 400, errors.New("invalid monitoring destination"))
			return
		}
	}
	for _, field := range []*string{&filter.Policy, &filter.FinalPolicy} {
		value := *field
		if value == "" {
			continue
		}
		var identity []string
		if len(value) > 2048 || json.Unmarshal([]byte(value), &identity) != nil || len(identity) != 2 || (identity[0] != "named" && identity[0] != "unnamed") || (identity[0] == "unnamed" && identity[1] != "") || (identity[0] == "named" && identity[1] == "") {
			fail(w, 400, errors.New("invalid monitoring policy"))
			return
		}
		*field = dnsserver.MonitoringPolicyValue(identity[1])
	}
	if filter.Domain != "" {
		if _, valid := dns.IsDomainName(dns.Fqdn(filter.Domain)); !valid || len(filter.Domain) > 253 {
			fail(w, 400, errors.New("invalid monitoring domain"))
			return
		}
	}
	if filter.ResponseCode != "" {
		valid := false
		for _, name := range dns.RcodeToString {
			valid = valid || name == filter.ResponseCode
		}
		if strings.HasPrefix(filter.ResponseCode, "RCODE") {
			var number int
			_, err := fmt.Sscanf(filter.ResponseCode, "RCODE%d", &number)
			valid = err == nil && number >= 0 && number <= 4095 && filter.ResponseCode == fmt.Sprintf("RCODE%d", number)
		}
		if !valid {
			fail(w, 400, errors.New("invalid monitoring response code"))
			return
		}
	}
	if path == "dns/monitoring" {
		service, ok := a.Service.(monitoringService)
		if !ok {
			fail(w, 501, errors.New("monitoring statistics are unsupported"))
			return
		}
		var view dnsserver.DNSMonitoringSnapshot
		var err error
		if advanced, ok := a.Service.(monitoringFilteredService); ok {
			view, err = advanced.DNSMonitoringFilteredView(r.Context(), filter)
		} else if filter.HasDimensions() {
			fail(w, 501, errors.New("monitoring filters are unsupported"))
			return
		} else {
			view, err = service.DNSMonitoringView(r.Context(), source)
		}
		if err != nil {
			fail(w, 404, err)
			return
		}
		write(w, 200, view)
		return
	}
	if source != "" {
		if service, ok := a.Service.(monitoringService); ok {
			if _, err := service.DNSMonitoringView(r.Context(), source); err != nil {
				fail(w, 404, err)
				return
			}
		}
	}
	if a.Store == nil {
		fail(w, 503, errors.New("monitoring history storage is unavailable"))
		return
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		fail(w, 400, errors.New("invalid monitoring history start"))
		return
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		fail(w, 400, errors.New("invalid monitoring history end"))
		return
	}
	if !to.After(from) || to.Sub(from) > 7*24*time.Hour || !from.Equal(from.Truncate(time.Minute)) || !to.Equal(to.Truncate(time.Minute)) {
		fail(w, 400, errors.New("monitoring history range must use whole minutes and span at most 7 days"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	view, err := a.Store.DNSMonitoringFilteredHistory(ctx, from.UTC(), to.UTC(), filter)
	if err != nil {
		fail(w, 503, err)
		return
	}
	if filter.HasDimensions() && view.TotalReceived > 0 && !view.Summary.DimensionsSupported {
		fail(w, 422, errors.New("monitoring dimensions are unavailable for this history range"))
		return
	}
	write(w, 200, view)
}
