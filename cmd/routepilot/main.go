// routepilot is a JSON-first client for the RoutePilot management service.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"surge-controller/internal/buildinfo"
	"surge-controller/internal/config"
	"surge-controller/internal/controlclient"
	"syscall"
	"time"
)

const help = `RoutePilot CLI — authenticated control of the connected backend

Usage: routepilot [global options] COMMAND [options]
Global options go before COMMAND:
  --url ORIGIN         HTTP(S) IP/localhost/proxy domain (ROUTEPILOT_URL or http://127.0.0.1:1055)
  --session-dir DIR    Private cookie storage (ROUTEPILOT_SESSION_DIR or user config directory)
  --ca-file FILE       Additional trusted PEM certificates (standard TLS verification)
  --timeout 30s        Per-request timeout
  --wait-timeout 5m    Maximum background operation/configuration wait
  --no-wait           Return operation ID immediately instead of waiting for diagnostics
  --pretty            Indent JSON output

Commands:
  version | help | help --json
  login --username NAME (--password-stdin | --password-file FILE)
  session | logout [--local-only]
  status | health | events | metrics
  mode [get | set auto|force-direct|force-surge]
  settings get | schema | set --file FILE
  config export | status | wait | retry | import --file FILE --yes
  dns status | queries | test --domain NAME --client-ip IP [--protocol tcp --port 443]
  dns import-hosts
  cache list [--domain NAME --decision DIRECT --offset 0 --limit 100]
  cache flush (--domain NAME | --all)
  dhcp status | get | schema | check --file FILE | set --file FILE
  surge status | hosts
  bgp status | routing status | routing verify --file FILE
  RESOURCE list | schema | add --file FILE | update ID --file FILE | delete ID
  RESOURCE bulk-delete ID ID...
    RESOURCE = hosts, interception, overrides, peers, routes, feeds, backends
  routes preview --url RULE_URL | routes import --file FILE
  bgp export-frr --file FILE
  updates check | status | install --version vX.Y.Z --yes
  operation status ID
  api GET|POST|PUT|DELETE RELATIVE_PATH [--file FILE]

JSON input: --file - reads stdin. No passwords on command line. Success goes to
stdout; JSON errors go to stderr (exit 1; usage errors exit 2). metrics is text.
Saves return immediately; config wait/status reports runtime activation.
All actions target --url, including complete App updates on that host.
`

var resources = map[string]string{"hosts": "dns/hosts", "interception": "dns/interception", "overrides": "overrides", "peers": "bgp/peers", "routes": "bgp/routes", "feeds": "bgp/routes/feeds", "backends": "management/backends"}

type usageError struct{ text string }

func (e *usageError) Error() string { return e.text }
func usage(text string) error       { return &usageError{text} }

type runner struct {
	client *controlclient.Client
	ctx    context.Context
	in     io.Reader
	wait   bool
}

func flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	return f
}
func parse(f *flag.FlagSet, args []string) error {
	if err := f.Parse(args); err != nil {
		return usage(err.Error())
	}
	if f.NArg() != 0 {
		return usage("unexpected arguments: " + strings.Join(f.Args(), " "))
	}
	return nil
}
func env(name, fallback string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return fallback
}
func encode(value any) json.RawMessage { data, _ := json.Marshal(value); return data }

func input(file string, in io.Reader) (json.RawMessage, error) {
	if file == "" {
		return nil, usage("--file FILE required; use --file - for stdin")
	}
	r := in
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, errors.New("cannot open input JSON file")
		}
		defer f.Close()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, 512*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 512*1024 || !json.Valid(data) || !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return nil, usage("input must be one JSON object no larger than 512 KiB")
	}
	return json.RawMessage(data), nil
}
func (r *runner) call(method, path string, body json.RawMessage) (json.RawMessage, error) {
	// Parse commands and JSON before contacting the backend. Usage failures
	// must stay actionable even when the backend is offline.
	if err := r.client.Authenticate(r.ctx); err != nil {
		return nil, err
	}
	return r.client.Request(r.ctx, method, path, body, r.wait)
}
func (r *runner) fileCall(method, path string, args []string, confirm bool) (json.RawMessage, error) {
	f := flags(path)
	file := f.String("file", "", "JSON file or -")
	yes := f.Bool("yes", false, "confirm full configuration replacement")
	if err := parse(f, args); err != nil {
		return nil, err
	}
	if confirm && !*yes {
		return nil, usage("--yes required for full configuration replacement")
	}
	body, err := input(*file, r.in)
	if err != nil {
		return nil, err
	}
	return r.call(method, path, body)
}
func (r *runner) resource(name string, args []string) (json.RawMessage, error) {
	path := resources[name]
	if len(args) == 0 {
		return nil, usage("resource action required: list, schema, add, update, delete, bulk-delete")
	}
	action, args := args[0], args[1:]
	switch action {
	case "list":
		if len(args) != 0 {
			return nil, usage("list takes no arguments")
		}
		return r.call("GET", path, nil)
	case "schema":
		if len(args) != 0 {
			return nil, usage("schema takes no arguments")
		}
		return resourceSchema(name), nil
	case "add":
		return r.fileCall("POST", path, args, false)
	case "update", "delete":
		if len(args) == 0 || !validID(args[0]) {
			return nil, usage("exact resource ID required; obtain it with list")
		}
		id := args[0]
		if action == "update" {
			return r.fileCall("PUT", path+"/"+id, args[1:], false)
		}
		if len(args) != 1 {
			return nil, usage("delete takes exactly one resource ID")
		}
		return r.call("DELETE", path+"/"+id, nil)
	case "bulk-delete":
		if name == "feeds" || name == "backends" {
			return nil, usage("this resource does not support bulk deletion")
		}
		if len(args) == 0 {
			return nil, usage("one or more resource IDs required")
		}
		for _, id := range args {
			if !validID(id) {
				return nil, usage("invalid resource ID")
			}
		}
		return r.call("POST", "resources/bulk-delete", encode(map[string]any{"resource": path, "ids": args}))
	case "preview":
		if name != "routes" {
			return nil, usage("preview is only available for routes")
		}
		f := flags("routes preview")
		url := f.String("url", "", "rule source URL")
		if err := parse(f, args); err != nil {
			return nil, err
		}
		if *url == "" {
			return nil, usage("--url RULE_URL required")
		}
		return r.call("POST", "bgp/routes/import-url", encode(map[string]string{"url": *url}))
	case "import":
		if name == "routes" {
			return r.fileCall("POST", "bgp/routes/import", args, false)
		}
	}
	return nil, usage("unknown resource action")
}
func query(value string) string { return url.QueryEscape(value) }
func validID(id string) bool {
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return id != ""
}
func resourceSchema(name string) json.RawMessage {
	switch name {
	case "hosts":
		return encode(config.Host{Domain: "example.com", Match: "exact", Addresses: []string{"192.0.2.1"}, TTL: 60, Enabled: true, Source: "manual"})
	case "interception":
		return encode(config.Interception{Name: "DNS", Prefix: "192.0.2.53/32", Enabled: true})
	case "overrides":
		return encode(config.Override{Domain: "example.com", Match: "exact", Decision: "DIRECT", Enabled: true})
	case "peers":
		return encode(config.Peer{Address: "192.0.2.2", ASN: 64512, Port: 179, HoldSeconds: 30, KeepaliveSeconds: 10, Enabled: true})
	case "routes":
		return encode(config.Route{Prefix: "192.0.2.53/32", NextHop: "192.0.2.1", LocalPreference: 100, Communities: []string{}, Binding: "none", Enabled: true})
	case "feeds":
		return encode(config.RouteFeed{URL: "https://example.com/routes.txt", IntervalMinutes: 60, SyncIPv4: true, SyncIPv6: true, NextHop4: "192.0.2.1", Communities: []string{}, Binding: "none", Enabled: true, RouteEnabled: true})
	case "backends":
		return encode(config.Backend{Name: "Example", URL: "http://192.0.2.1:1055"})
	}
	return encode(nil)
}

func (r *runner) execute(args []string) (json.RawMessage, error) {
	command := args[0]
	args = args[1:]
	if command == "session" {
		if len(args) != 0 {
			return nil, usage("session takes no arguments")
		}
		value, _, err := r.client.Session(r.ctx)
		return value, err
	}
	if command == "login" {
		f := flags("login")
		user := f.String("username", "admin", "management username")
		stdin := f.Bool("password-stdin", false, "read password from stdin")
		file := f.String("password-file", "", "private password file")
		if err := parse(f, args); err != nil {
			return nil, err
		}
		if *stdin == (*file != "") {
			return nil, usage("choose exactly one of --password-stdin and --password-file")
		}
		reader := r.in
		if *file != "" {
			info, err := os.Lstat(*file)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return nil, usage("password file must be a private regular file (0600), without symlinks")
			}
			f, err := os.Open(*file)
			if err != nil {
				return nil, errors.New("cannot open password file")
			}
			defer f.Close()
			reader = f
		}
		password, err := io.ReadAll(io.LimitReader(reader, 4097))
		if err != nil {
			return nil, err
		}
		password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"))
		if len(password) == 0 || len(password) > 4096 {
			return nil, usage("password must contain 1–4096 bytes")
		}
		return r.client.Login(r.ctx, *user, string(password))
	}
	if command == "logout" {
		f := flags("logout")
		local := f.Bool("local-only", false, "forget local session without network request")
		if err := parse(f, args); err != nil {
			return nil, err
		}
		if !*local {
			if _, _, err := r.client.Session(r.ctx); err != nil {
				return nil, err
			}
			if _, err := r.client.Request(r.ctx, "POST", "logout", nil, false); err != nil {
				return nil, err
			}
		}
		if err := r.client.Forget(); err != nil {
			return nil, err
		}
		return encode(map[string]bool{"ok": true}), nil
	}
	if _, ok := resources[command]; ok {
		return r.resource(command, args)
	}
	switch command {
	case "status", "health", "events", "metrics":
		if len(args) != 0 {
			return nil, usage(command + " takes no arguments")
		}
		return r.call("GET", command, nil)
	case "mode":
		if len(args) == 0 || len(args) == 1 && args[0] == "get" {
			return r.call("GET", "mode", nil)
		}
		if len(args) != 2 || args[0] != "set" || args[1] != "auto" && args[1] != "force-direct" && args[1] != "force-surge" {
			return nil, usage("mode set auto|force-direct|force-surge")
		}
		return r.call("PUT", "mode", encode(map[string]string{"mode": args[1]}))
	case "dhcp", "settings", "config", "updates", "dns", "cache", "surge", "bgp", "routing", "operation":
		if len(args) == 0 {
			return nil, usage(command + " action required")
		}
		action := args[0]
		rest := args[1:]
		switch command + " " + action {
		case "dhcp status":
			if len(rest) == 0 {
				return r.call("GET", "dhcp/status", nil)
			}
		case "dhcp get":
			if len(rest) == 0 {
				return r.call("GET", "dhcp/configuration", nil)
			}
		case "dhcp schema":
			if len(rest) == 0 {
				return encode(config.DHCP{Options: config.DHCPOptions{LeaseSeconds: 86400}}), nil
			}
		case "dhcp check":
			return r.fileCall("POST", "dhcp/check", rest, false)
		case "dhcp set":
			return r.fileCall("PUT", "dhcp/configuration", rest, false)
		case "settings get":
			if len(rest) == 0 {
				return r.call("GET", "settings", nil)
			}
		case "settings schema":
			if len(rest) == 0 {
				return encode(config.Default().Redacted().Settings), nil
			}
		case "settings set":
			return r.fileCall("PUT", "settings", rest, false)
		case "config export":
			if len(rest) == 0 {
				return r.call("GET", "configuration/export", nil)
			}
		case "config status":
			if len(rest) == 0 {
				return r.call("GET", "configuration/status", nil)
			}
		case "config wait":
			if len(rest) == 0 {
				if err := r.client.Authenticate(r.ctx); err != nil {
					return nil, err
				}
				return r.client.WaitConfiguration(r.ctx)
			}
		case "config retry":
			if len(rest) == 0 {
				return r.call("POST", "configuration/retry", nil)
			}
		case "config import":
			return r.fileCall("POST", "configuration/import", rest, true)
		case "updates check", "updates status":
			if len(rest) == 0 {
				return r.call("GET", "updates/"+action, nil)
			}
		case "updates install":
			f := flags("updates install")
			version := f.String("version", "", "release from updates check")
			yes := f.Bool("yes", false, "confirm complete App update on target host")
			if err := parse(f, rest); err != nil {
				return nil, err
			}
			if *version == "" || !*yes {
				return nil, usage("updates install --version vX.Y.Z --yes required; run updates check first")
			}
			return r.call("POST", "updates/install", encode(map[string]string{"version": *version}))
		case "dns status":
			if len(rest) == 0 {
				return r.call("GET", "status", nil)
			}
		case "dns queries":
			if len(rest) == 0 {
				return r.call("GET", "dns/queries", nil)
			}
		case "dns import-hosts":
			if len(rest) == 0 {
				return r.call("POST", "dns/hosts/import-surge", nil)
			}
		case "dns test":
			f := flags("dns test")
			domain := f.String("domain", "", "domain")
			ip := f.String("client-ip", "", "actual client IP")
			protocol := f.String("protocol", "tcp", "tcp or udp")
			port := f.Int("port", 443, "connection port")
			if err := parse(f, rest); err != nil {
				return nil, err
			}
			if *domain == "" || *ip == "" || *port < 1 || *port > 65535 || *protocol != "tcp" && *protocol != "udp" {
				return nil, usage("domain, client IP, TCP/UDP and valid port required")
			}
			return r.call("POST", "test/domain", encode(map[string]any{"domain": *domain, "client_ip": *ip, "protocol": *protocol, "port": *port}))
		case "cache list":
			f := flags("cache list")
			domain := f.String("domain", "", "filter domain")
			decision := f.String("decision", "", "DIRECT, SURGE, REJECT or UNKNOWN")
			offset := f.Int("offset", 0, "offset")
			limit := f.Int("limit", 100, "0–1000")
			if err := parse(f, rest); err != nil {
				return nil, err
			}
			if *offset < 0 || *limit < 0 || *limit > 1000 {
				return nil, usage("invalid cache offset or limit")
			}
			return r.call("GET", fmt.Sprintf("dns/cache?offset=%d&limit=%d&domain=%s&decision=%s", *offset, *limit, query(*domain), query(*decision)), nil)
		case "cache flush":
			f := flags("cache flush")
			domain := f.String("domain", "", "exact domain")
			all := f.Bool("all", false, "clear all DNS caches")
			if err := parse(f, rest); err != nil {
				return nil, err
			}
			if *all == (*domain != "") {
				return nil, usage("choose exactly one of --domain NAME and --all")
			}
			return r.call("POST", "dns/cache/flush", encode(map[string]string{"domain": *domain}))
		case "surge status", "surge hosts", "bgp status", "routing status":
			if len(rest) == 0 {
				return r.call("GET", command+"/"+action, nil)
			}
		case "routing verify":
			return r.fileCall("POST", "routing/verify", rest, false)
		case "bgp export-frr":
			return r.fileCall("POST", "bgp/export-frr", rest, false)
		case "operation status":
			if len(rest) == 1 && validID(rest[0]) {
				return r.call("GET", "operations/status?id="+rest[0], nil)
			}
		}
		return nil, usage("unknown action or unexpected arguments; see routepilot help")
	case "resource":
		if len(args) >= 2 && resources[args[0]] != "" {
			return r.resource(args[0], args[1:])
		}
		return nil, usage("resource NAME ACTION required")
	case "api":
		if len(args) < 2 {
			return nil, usage("api METHOD RELATIVE_PATH [--file FILE]")
		}
		method, path := args[0], args[1]
		if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" {
			return nil, usage("method must be GET, POST, PUT or DELETE")
		}
		parsed, parseErr := url.Parse(path)
		if parseErr != nil {
			return nil, usage("invalid API path")
		}
		authPath := parsed.Path
		if authPath == "session" || authPath == "login" || authPath == "logout" || strings.HasSuffix(authPath, "/session") || strings.HasSuffix(authPath, "/login") || strings.HasSuffix(authPath, "/logout") {
			return nil, usage("use session/login/logout commands to manage authentication")
		}
		if err := controlclient.Path(path); err != nil {
			return nil, usage(err.Error())
		}
		f := flags("api")
		file := f.String("file", "", "JSON file")
		if err := parse(f, args[2:]); err != nil {
			return nil, err
		}
		var body json.RawMessage
		if *file != "" {
			if method == "GET" {
				return nil, usage("GET must not have a body")
			}
			var err error
			body, err = input(*file, r.in)
			if err != nil {
				return nil, err
			}
		}
		return r.call(method, path, body)
	}
	return nil, usage("unknown command; see routepilot help")
}

func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	f := flags("routepilot")
	endpoint := f.String("url", env("ROUTEPILOT_URL", "http://127.0.0.1:1055"), "management origin")
	dir := f.String("session-dir", env("ROUTEPILOT_SESSION_DIR", ""), "private session directory")
	ca := f.String("ca-file", "", "trusted certificates")
	timeout := f.Duration("timeout", 30*time.Second, "request timeout")
	waitTimeout := f.Duration("wait-timeout", 5*time.Minute, "operation wait timeout")
	noWait := f.Bool("no-wait", false, "return operation ID immediately")
	pretty := f.Bool("pretty", false, "indent JSON")
	if err := f.Parse(args); errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(out, help)
		return err
	} else if err != nil {
		return usage(err.Error())
	}
	args = f.Args()
	if len(args) == 0 || args[0] == "help" {
		if len(args) > 1 {
			if len(args) != 2 || args[1] != "--json" {
				return usage("help [--json]")
			}
			return json.NewEncoder(out).Encode(map[string]any{"name": "routepilot", "version": buildinfo.Current(), "usage": help, "resources": resources, "output": "JSON; metrics is Prometheus text", "configuration_save": "durable save; activation asynchronous", "exit_codes": map[string]int{"success": 0, "error": 1, "usage": 2}})
		}
		_, err := io.WriteString(out, help)
		return err
	}
	if args[0] == "version" {
		if len(args) != 1 {
			return usage("version takes no arguments")
		}
		return json.NewEncoder(out).Encode(buildinfo.Current())
	}
	if err := controlclient.CheckAccess(); err != nil {
		return err
	}
	c, err := controlclient.New(controlclient.Options{URL: *endpoint, SessionDir: *dir, CAFile: *ca, Timeout: *timeout, WaitTimeout: *waitTimeout})
	if err != nil {
		return err
	}
	r := runner{c, ctx, in, !*noWait}
	value, err := r.execute(args)
	if err != nil {
		return err
	}
	if args[0] == "metrics" || args[0] == "api" && len(args) > 2 && args[2] == "metrics" {
		_, err = out.Write(value)
		return err
	}
	if *pretty {
		var formatted strings.Builder
		var data any
		decoder := json.NewDecoder(strings.NewReader(string(value)))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			return err
		}
		enc := json.NewEncoder(&formatted)
		enc.SetIndent("", "  ")
		if err := enc.Encode(data); err != nil {
			return err
		}
		_, err = io.WriteString(out, formatted.String())
		return err
	}
	_, err = fmt.Fprintln(out, string(value))
	return err
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		status := 1
		value := &controlclient.Error{Code: "client", Message: err.Error()}
		var api *controlclient.Error
		if errors.As(err, &api) {
			value = api
		}
		var invalid *usageError
		if errors.As(err, &invalid) {
			status = 2
			value.Code = "usage"
		}
		_ = json.NewEncoder(os.Stderr).Encode(value)
		os.Exit(status)
	}
}
