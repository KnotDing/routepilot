# Phase 2 acceptance gate

Status: **IPv4 transparent interception and loop-prevention gate passed using Docker on 2026-10-08**. See [dated evidence](phase-2-docker-evidence.md). IPv6 remains disabled until separately verified. Offline tests are necessary but
do not establish that macOS PF accepted a routed non-local packet or that the
gateway's source policy bypassed its interception route. Specification §61 says
"Do not continue until transparent interception and loop prevention are proven."

## Implementation checks

- [x] Direct UDP/TCP listeners exchange real DNS wire packets on loopback.
- [x] Shared transparent handler preserves client, original destination and
  transport using an injected original-destination lookup.
- [x] Preserve-original and configured-pool forwarding have separate tested paths.
- [x] macOS PF rule generator covers IPv4/IPv6, UDP/TCP and source loop guard.
- [x] Real upstream requests use an explicit dialer and fail closed on missing
  interface/source configuration.
- [x] Re-entered self-originated queries never trigger another upstream exchange.
- [x] DNS forwarding supports UDP truncation fallback, DoT and DoH with TLS
  identity checks. Tests use local servers, never Surge/router/Internet.
- [x] Decision/cache tests cover uncertainty, failures, deadlines, per-client
  isolation, singleflight, domain flush and in-flight generation invalidation.
- [x] Fake-IP route-gate tests prohibit issuance before readiness and suppress
  an in-flight answer after readiness loss.
- [x] Actual macOS PF parser accepts the generated anchor (`pfctl -n`, 2026-10-08).
- [x] Actual PF NATLOOK restores an IPv4 UDP original destination.
- [x] Actual PF NATLOOK restores an IPv4 TCP original destination.
- [x] Docker client receives replies from its original resolver's address and port.
- [x] Gateway source policy permits real preserved upstream DNS in both transports.
- [x] Packet trace shows no upstream re-entry during interception.
- [x] Injected self-origin UDP/TCP return traffic is dropped by actual PF, with
  zero additional Controller queries or upstream exchanges. The user-owned
  gateway policy was not removed; its failure was simulated at the ingress.
- [ ] IPv6 path proven if IPv6 interception is enabled.

## Network prerequisites

Observed deployment example: Mac `en0=192.168.20.2`, gateway `192.168.20.1`,
physical `en0` member of Surge's `bridge100`. Confirm actual PF traversal on both
interfaces. Reserve source ports 20000–20999 for the Controller.

The user has authorized manually configuring a gateway source-policy exception.
Configure a higher-priority rule for source `192.168.20.2` that uses a normal WAN
path/routing table **without** DNS interception routes. A WAN rule that still
looks up the same BGP resolver route is insufficient. Do not automatically modify
the router. During Phase 2, use an explicitly chosen temporary test route for
one resolver prefix (e.g. `8.8.8.8/32 → 192.168.20.2`) instead of requiring a
BGP announcement during the isolated packet test. Choose a resolver that avoids disrupting active
clients, and remove the temporary route after testing.

## Commands for this Mac

Build `make build`. Generate and review the anchor; this does not change PF:

```sh
bin/surge-controller \
  --egress-interface=en0 --egress-source4=192.168.20.2 \
  --gateway4=192.168.20.1 \
  --ingress-interface=en0 --ingress-interface=bridge100 \
  --transparent-listen=192.168.20.2:1054 \
  --intercept-prefix=8.8.8.8/32 \
  --write-pf=docs/macos-test.pf
sudo pfctl -n -a com.apple/surge-controller -f docs/macos-test.pf
```

After syntax validation, source-policy configuration and explicit approval to
modify local PF, enable a reference, retain its returned token, and load only
this anchor:

```sh
sudo pfctl -E
sudo pfctl -a com.apple/surge-controller -f docs/macos-test.pf
```

Then run the core in an **explicit** FORCE_DIRECT diagnostic mode with a new
private journal file. This tests transparent real DNS independently of the
integrated BGP/state implementation; it does not pretend AUTO policy is ready:

```sh
sudo bin/surge-controller \
  --listen=192.168.20.2:1053 \
  --transparent-listen=192.168.20.2:1054 \
  --egress-interface=en0 --egress-source4=192.168.20.2 \
  --intercept-prefix=8.8.8.8/32 \
  --gateway-bypass-verified --mode=force-direct \
  --verification-journal=/private/tmp/surge-phase2-unique.jsonl
```

Capture the actual LAN path, narrowly filtered to test traffic:

```sh
sudo tcpdump -ni en0 'host 8.8.8.8 and port 53'
```

Use Docker, as requested by the user. The default OrbStack bridge does not
traverse this Mac's PF ingress. Create a temporary macvlan with the Mac's virtual
bridge as gateway; do not change a LAN client's routes:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/dns-probe-linux-arm64 ./cmd/dns-probe
docker network create --driver macvlan --subnet 172.30.31.0/24 \
  --ip-range 172.30.31.128/28 --gateway 172.30.31.3 --opt parent=eth0 \
  routepilot-phase2-macvlan
docker run --rm --network=routepilot-phase2-macvlan --ip=172.30.31.130 \
  --mount type=bind,src=/ABSOLUTE/PATH/bin/dns-probe-linux-arm64,dst=/probe,readonly \
  --platform=linux/amd64 node:22-bookworm /probe --resolver=8.8.8.8:53 --domain=example.org > docker-probe.jsonl
```

The recorded addresses are deployment-specific; inspect the Docker host's
interfaces and Mac's bridge before reusing them. Use `docs/macos-docker-test.pf`
and `scripts/start_phase2.sh docker` for this deployment's test anchor/capture.
The diagnostic re-entry generator `scripts/dns_reentry_linux.go` refuses to run
unless its isolated container has the expected test address.

A successful client reply alone is insufficient: without interception the
client could simply have reached its real resolver. Match both transactions
to the Controller's journal, verify `intercepted=true`, non-local original
`8.8.8.8:53`, correct Docker client address and both transport values. Compare the
wire trace with pinned source `192.168.20.2:20000..20999`, resolver reply and zero
incoming self-sourced DNS. The journal checker is:

```sh
python3 scripts/check_phase2.py \
  --journal=/private/tmp/surge-phase2-unique.jsonl \
  --probe=docker-probe.jsonl --client=172.30.31.130
```

The checker confirms correlated transactions only; review packet captures and
gateway policy separately. Keep a redacted, dated evidence record here before
opening Phase 3. Test IPv6 separately before enabling IPv6 prefixes.

## Cleanup

Stop the Controller; delete the temporary Docker network. The user should remove
the manually configured gateway test route when it is no longer needed. Empty
only this anchor and release only the enable reference obtained above:

```sh
sudo pfctl -a com.apple/surge-controller -F rules
sudo pfctl -a com.apple/surge-controller -F nat
sudo pfctl -X RETAINED_TOKEN
```

Never run `pfctl -d` or flush the global rules/states; other services (including
Surge) own their own PF references and anchors. Verification journals contain
DNS browsing metadata; remove them after the evidence is recorded.

## Subsequent implementation

The normal service now includes phases 3–6: SQLite/REST/SSE/React, native GoBGP and route limits, deterministic health/HA, sessions/CSRF/privacy/metrics and launchd packaging. See [acceptance.md](acceptance.md) for the tested invariants and deployment verification boundaries. This guide remains the independent, reproducible Phase 2 diagnostic procedure.
