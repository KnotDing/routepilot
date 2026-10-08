// dns-probe is run on a separate LAN client during Phase 2 validation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"net/netip"
	"os"
	"time"
)

func main() {
	destination := flag.String("resolver", "8.8.8.8:53", "original non-local resolver IP:53")
	domain := flag.String("domain", "example.com", "domain to resolve")
	flag.Parse()
	target, err := netip.ParseAddrPort(*destination)
	if err != nil || target.Port() != 53 {
		fmt.Fprintln(os.Stderr, "resolver must be a literal non-local IP:53")
		os.Exit(1)
	}
	failed := false
	for _, proto := range []string{"udp", "tcp"} {
		q := new(dns.Msg)
		q.SetQuestion(dns.Fqdn(*domain), dns.TypeA)
		client := &dns.Client{Net: proto, Timeout: 4 * time.Second}
		r, duration, err := client.Exchange(q, *destination)
		record := map[string]any{"protocol": proto, "destination": *destination, "id": q.Id, "domain": q.Question[0].Name, "duration_ms": duration.Milliseconds(), "sent_at": time.Now().UTC()}
		if err != nil {
			record["error"] = err.Error()
			failed = true
		} else {
			record["rcode"] = r.Rcode
			var answers []string
			for _, a := range r.Answer {
				answers = append(answers, a.String())
			}
			record["answers"] = answers
			if r.Rcode != dns.RcodeSuccess || len(r.Answer) == 0 {
				failed = true
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(record)
	}
	if failed {
		os.Exit(1)
	}
}
