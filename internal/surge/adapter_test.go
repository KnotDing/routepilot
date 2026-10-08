package surge

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

type runner struct {
	rules       []string
	explanation string
	groups      string
	resources   string
	args        []string
}

func (r *runner) Run(_ context.Context, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "status":
		return []byte(`{"profile":"fixture","mode":"rule","start-time":1,"features":{"mitm":false,"rewrite":false,"scripting":false}}`), nil
	case "dump rule":
		return json.Marshal(map[string]any{"rules": r.rules})
	case "dump temp-rule":
		return []byte(`{"rules":[]}`), nil
	case "policy-group list":
		if r.groups != "" {
			return []byte(r.groups), nil
		}
		return []byte(`{"groups":[]}`), nil
	case "external-resource list":
		if r.resources != "" {
			return []byte(r.resources), nil
		}
		return []byte(`{"defines":[]}`), nil
	}
	if len(args) > 2 && args[0] == "rule" {
		r.args = append([]string(nil), args...)
		return []byte(r.explanation), nil
	}
	return nil, errors.New("unexpected CLI command")
}

type transport struct{}

func (transport) Exchange(context.Context, resolver.ResolverEndpoint, *dns.Msg) (*dns.Msg, error) {
	return nil, errors.New("unused")
}
func TestActiveRulesContextGuard(t *testing.T) {
	for _, kind := range []string{"PROCESS-NAME", "URL-REGEX", "USER-AGENT", "DEST-PORT", "SRC-PORT", "SCRIPT", "PROTOCOL", "RULE-SET", "IP-CIDR", "AND", "OR", "NOT", "UNKNOWN"} {
		if safe, _ := ContextGuard([]string{"DOMAIN,example.test,DIRECT", kind + ",value,Proxy", "FINAL,DIRECT"}); safe {
			t.Fatalf("%s allowed a DIRECT proof", kind)
		}
	}
	if safe, _ := ContextGuard([]string{"DOMAIN,example.test,DIRECT", "FINAL,Proxy"}); !safe {
		t.Fatal("safe input dependency audit rejected")
	}
	if safe, _ := ContextGuard([]string{"DOMAIN,example.test,DIRECT,extended-matching", "FINAL,Proxy"}); safe {
		t.Fatal("HTTP Host/SNI-dependent domain rule allowed")
	}
}
func TestRawExplainFinalPolicyAndArguments(t *testing.T) {
	r := &runner{rules: []string{"DOMAIN,example.test,Group", "FINAL,Proxy"}, explanation: `{"rule":"DOMAIN,example.test,Group","rule-policy":"Group","final":"DIRECT","final-type":"Direct","steps":[{"group":"Group","type":"select","selected":"DIRECT","reason":"manual"}],"notes":[]}`}
	a, err := New(r, transport{}, resolver.ResolverEndpoint{Protocol: "udp", Address: "198.18.0.2:53"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	m, err := a.ExplainRule(context.Background(), model.RuleMatchRequest{Domain: "EXAMPLE.test.", ClientIP: netip.MustParseAddr("192.0.2.10")})
	if err != nil || !m.ContextSafe || m.FinalPolicy != "DIRECT" {
		t.Fatalf("effective final policy lost: %+v %v", m, err)
	}
	joined := strings.Join(r.args, " ")
	for _, arg := range []string{"dest-port=0", "protocol=", "source-address=192.0.2.10", "source-port=0", "is-local=off"} {
		if !strings.Contains(joined, arg) {
			t.Fatalf("missing %s in argument array", arg)
		}
	}
	if _, err = a.ExplainRule(context.Background(), model.RuleMatchRequest{Domain: "example.test;touch /tmp/pwn", ClientIP: netip.MustParseAddr("192.0.2.10")}); err == nil {
		t.Fatal("unsafe CLI hostname accepted")
	}
	r.explanation = `{"rule":"FINAL,Group","rule-policy":"Group","final":"DIRECT","final-type":"Direct","steps":[{"type":"url-test"}]}`
	m, err = a.ExplainRule(context.Background(), model.RuleMatchRequest{Domain: "example.test", ClientIP: netip.MustParseAddr("192.0.2.10")})
	if err != nil || m.ContextSafe {
		t.Fatal("dynamic group accepted as certain")
	}
}

func TestSnapshotSeparatesSelectionsAndEffectiveResourceRevisions(t *testing.T) {
	r := &runner{rules: []string{"FINAL,DIRECT"}, groups: `{"groups":[{"name":"Auto","type":"url-test","selected":"A","options":[]}]}`, resources: `{"defines":[{"key":"rules","type":"ruleset","ready":true,"updatedAt":10,"updating":false}]}`}
	a, err := New(r, transport{}, resolver.ResolverEndpoint{Protocol: "udp", Address: "198.18.0.2:53"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r.groups = strings.ReplaceAll(r.groups, `"selected":"A"`, `"selected":"B"`)
	selected, err := a.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != selected.Digest || first.SelectionDigest == selected.SelectionDigest {
		t.Fatal("selection was treated as a configuration reload")
	}
	r.resources = strings.ReplaceAll(r.resources, `"updating":false`, `"updating":true`)
	a.resourceCheckedAt = time.Time{}
	updating, err := a.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updating.Digest != first.Digest {
		t.Fatal("transient download cleared decisions")
	}
	r.resources = strings.ReplaceAll(r.resources, `"updatedAt":10`, `"updatedAt":20`)
	a.resourceCheckedAt = time.Time{}
	revised, err := a.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if revised.Digest == first.Digest {
		t.Fatal("completed resource update failed to invalidate decisions")
	}
}
