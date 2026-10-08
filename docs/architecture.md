# Architecture

The IPv4 transparent-ingress/loop-prevention gate passed with actual macOS PF and Docker macvlan on 2026-10-08. The normal `serve` executable now integrates SQLite, embedded React, REST/SSE, native GoBGP, health/state control and security. The legacy flags run an independent DNS/PF diagnostic harness with its Fake-IP gate deliberately closed. See [acceptance scope](acceptance.md) and [packet evidence](phase-2-docker-evidence.md).

## DNS boundary

Both UDP and TCP listeners share the dispatcher, but direct and redirected
listeners have distinct ports. Every listener binds a concrete address, avoiding
loss of the local destination through a wildcard UDP socket. IPv4 and IPv6 use
separate concrete binds. All questions carry client address/port, original
destination, transport, question and reception time.

UDP uses a fixed worker pool and a bounded queue. TCP uses bounded simultaneous
connections with sequential, correctly framed queries and read/write deadlines.
Overload does not spawn unbounded workers or processes. The Surge evaluator has
its own worker pool, queue, singleflight and deadlines. Histories and caches are
bounded; management query history is configurable, bounded and kept only in memory. No per-query service log or SQL event is emitted. An explicitly requested, mode-0600 diagnostic journal is the only on-disk query trace.

## Policy evidence and deliberately conservative DIRECT

The local Surge CLI is executed through an argument array with `--raw`. The
installed CLI was inspected read-only on 2026-10-08. Real observed explanation
fields are `rule`, `rule-policy`, `final`, `final-type`, `steps`, `notes` and
`duration-ms`. Policy-group hops and the effective `final` value determine the
result; a literal DIRECT token on a rule does not.

Surge's CLI normally supplies TCP/443 for a bare hostname. These are **not known
future connection properties at DNS time**. The adapter explicitly clears
protocol, URL, process, User-Agent, SNI and HTTP Host, uses port zero, and passes
only the known client IP. The DNS source port and resolver port are not reused
as future application connection properties.

Clearing metadata alone is not a proof: Surge can skip an earlier context rule
and later report DIRECT. The adapter separately reads `dump rule`, `dump
temp-rule`, status and policy groups. Its small allowlist audits **input
dependencies** of the entire active inventory, not rule matching. Any unexpanded
RULE-SET, IP-based rule, logical rule, unsupported option, source-port/process/
protocol/URL/script rule or enabled traffic-processing feature blocks a certain
DIRECT conclusion. Only domain and source-IP input dependencies are audited.
Dynamic/unsupported policy groups and underlying proxy chains also block DIRECT.
Surge performs all actual matching and group resolution. A complex profile can
therefore conservatively send every A/AAAA through Surge; this is intentional
and permitted by invariants 1, 2 and 8. UI exact/suffix overrides are explicit administrative policy; manual modes remain visible. The diagnostic CLI `force-direct` is also an explicit override.

Snapshots include profile, start time, mode, rules, temporary rules, group
selection/options and relevant features, with volatile uptime removed. Polling
invalidates both caches on a confirmed effective configuration/resource change.
Stable DIRECT/SURGE/REJECT classifications persist until invalidation or capacity
eviction by default; UNKNOWN and errors retain short retry TTLs. Failed reads keep
the last confirmed revision unless `invalidate_on_read_failure` is enabled; recovery
of the unchanged revision does not flush again. Group-selection invalidation of
DIRECT and REJECT is independently configurable. `expire_decisions` restores timed
expiry for stable classifications. Real DNS response TTLs and health gating are unchanged.
Cached classification has bounded
polling coherence, not atomic synchronization with an external mutable Surge
process. A race-free instant policy-change guarantee needs a Surge-provided
revision/event handshake; this has not been established. Unknown schemas,
expired audits and changed in-flight generations fail conservatively.

The decision key includes normalized domain, individual client IP and DNS class/
type. Grouping all LAN clients would be unsafe for source-address rules. Cache
flushes reject writes from older in-flight generations. Full generation
invalidation is O(1); targeted domain flush keeps other domains but also advances
the generation to reject in-flight results.

## DNS answers

Only a certain DIRECT decision uses the real resolver in AUTO. Errors and
ambiguity stay UNKNOWN/SURGE. Certain REJECT returns REFUSED. Ordinary non-address
record types use the real resolver; ANY is refused under RFC 8482 to prevent an
address-answer escape from A/AAAA classification. SURGE HTTPS/SVCB returns a successful empty
answer with no hints, aliases, AD bit or invented DNSSEC proof. RFC 9460 §3 allows
the client to fall back to A/AAAA when service binding resolution yields no
usable record. The local synthetic response has no authoritative SOA and is not
negative-cached as an authenticated denial.

The real resolver supports UDP, TCP, UDP truncation fallback, certificate-verified
DoT and HTTPS DNS wire format. DoH uses an explicit bootstrap IP without invoking
system DNS, rejects redirects, bounds response size and checks content type.
Responses are checked against transaction ID, opcode, question count, QNAME,
QTYPE and QCLASS. Response caching partitions upstream identity, question and
DNSSEC flags, ages TTLs, uses RFC 2308 negative TTLs and does not cache responses
to ECS/other EDNS options or checking-disabled queries.

Fake-IP always comes from the configured Surge DNS endpoint. No allocator exists.
The dispatcher requires both a current safety snapshot and an issuance lease;
readiness loss while a Surge query is in flight suppresses its answer. It also
rejects unexpected real addresses in Fake-IP answers. The integrated state manager stops new leases, invalidates outstanding leases and drains response writes before route withdrawal. Fake answer TTL is clamped to the configured grace maximum; the manager also retains observed issuance expiry. Real upstream answers are rejected if any A/AAAA belongs to a configured Fake-IP network, preventing a broken bypass from evading the safety gate.

## macOS transparent ingress: PF, not wildcard sockets

The chosen mechanism is Apple's PF `rdr` on the actual ingress interface(s).
Gateway routing sends non-local resolver-destination traffic to the Mac's MAC
address. PF processes incoming UDP/TCP destination port 53 before ordinary local
delivery and redirects selected resolver prefixes to a concrete local listener
on port 1054. This is the subsystem that makes a non-local destination receivable.

PF's state performs reverse translation on replies, so clients continue seeing
the original resolver address/port. TCP framing and connection lifetime remain
kernel TCP behavior; UDP replies use the received session to retain interface
information. Every transparent packet/connection queries `DIOCNATLOOK` using its
translated client-to-listener tuple with `PF_OUT`, restoring the external
destination. Lookup failure, ambiguous state, non-53 original port, or a local
original destination is never silently treated as a direct-addressed request.

Apple's current private ABI differs from common OpenBSD examples:

| Field | Byte offset / size |
| --- | --- |
| saddr, daddr, rsaddr, rdaddr | 0, 16, 32, 48 / 16 each |
| sxport, dxport, rsxport, rdxport | 64, 68, 72, 76 / 4 each |
| af, proto, proto_variant, direction | 80, 81, 82, 83 / 1 each |
| ioctl structure size | 84 |
| DIOCNATLOOK | `_IOWR('D',23,84) = 0xc0544417` |

The Go implementation uses the read-only `/dev/pf` ioctl and network-order port
bytes, with native AF_INET/AF_INET6 values. This private ABI must be verified
against actual UDP and TCP states on the deployment OS; successful compilation
and fake lookups alone do not prove it. There is no fallback that invents an
original destination. On other OSes PF startup explicitly fails, while portable
offline tests can inject a lookup abstraction.

Generated rules live only in `com.apple/surge-controller`, reached by macOS's
existing `com.apple/*` root anchors. Never replace the root PF ruleset or Surge's
anchor. `rdr pass` is deliberately avoided because it bypasses filter rules and
could bypass the loop guard. Installation must check anchor reachability, actual
interface/bridge traversal, rule order and existing Surge PF behavior. An
en0/bridge100 list reflects this Mac's observed bridge topology, not an assumed
universal macOS setup. PF enable references use `pfctl -E`/`-X TOKEN`; removing
this component must not disable PF globally.

## Egress bypass and the single-interface topology

On the inspected Mac the physical LAN address is `192.168.20.2` on `en0`; its
physical gateway route is `192.168.20.1`. Surge also installs a `utun4` default
route. The user confirmed a single physical ingress interface and explicitly
authorized **manual** gateway source-policy configuration preserving original
resolvers. The Controller never configures the router.

A real DNS socket binds its source to the configured physical address and its
interface through `IP_BOUND_IF` or `IPV6_BOUND_IF`. It uses a bounded pool of
reserved source ports 20000–20999, below the normal dynamic port range. PF applies
`route-to (en0 physical-gateway)` only to those source ports and DNS/DoT/DoH
destination ports, preventing a Surge/VPN host route from taking the upstream
request. Interface disappearance, missing family source address, exhausted ports
or source-address changes fail closed; generic system DNS exchange is not a
fallback.

**Host pinning does not override gateway BGP decisions.** In this single-NIC
topology the gateway must have a higher-priority policy for the Mac's pinned
source address (optionally narrowed to its reserved source ports) that selects
a WAN routing table/path without the Controller's resolver-interception routes.
Merely selecting the same physical next hop on the Mac cannot prevent a router
from routing the packet back to the Mac. IPv6 needs its own working source
policy before IPv6 preserve-original forwarding is enabled.

PF excludes the pinned source from redirection and drops self-sourced incoming
DNS. The dispatcher also refuses self-originated intercepted queries before any
classification or upstream exchange. A missing router exception therefore
causes a bounded failure instead of an amplifying forwarding loop. Successful
original-resolver preservation still requires real gateway-path proof. The
operator confirmation flag cannot substitute for that proof.

Surge Fake-IP DNS uses a separate configured internal path; its endpoint cannot
overlap configured interception prefixes. External real DNS must not use that
path, since independent Surge-failure fallback is a mandatory invariant.

## BGP implications that must remain explicit

A BGP resolver prefix affects **all traffic to that prefix**, not only port 53.
This listener intercepts classic DNS only. It does not provide DoH/DoT/DoQ/Private
Relay interception, and a resolver route does not by itself preserve non-DNS
traffic to that resolver. Later deployment must document this effect rather than
claiming port-based BGP routing or inventing a forwarding engine.

The native GoBGP manager owns only UUIDs for its fixed configuration-derived advertisements. Fake-IP routes bind to Surge, interception routes bind to Controller, and manual routes default to None. Global validation counts all possible managed routes against the hard limit before persistence; runtime reconciliation checks again before any add. There is no DNS/cache-to-route path. Imported peer routes are globally rejected; the Controller cannot become a transit speaker.

For BGP-backed Fake-IP routes, route readiness requires enabled peers ESTABLISHED, negotiated address families and matching routes in Adj-RIB-Out, followed by the configured stabilization interval. This demonstrates advertisement, not a router FIB acknowledgement. Live deployment must inspect the receiving gateway. On session loss GoBGP removes peer paths; local loopback IPv4/IPv6 NLRI/session-loss tests verify this independently of hardware. BGP graceful restart is not requested because retaining stale DNS interception routes could prevent fallback.

## State, persistence and management

The monitor probes CLI inventory, optional HTTP API, Fake-IP DNS, pinned real DNS, listener/interface/source availability, database and BGP separately. Consecutive successes/failures drive hysteresis. State transitions are deterministic: STARTING → RECOVERING → HEALTHY; Surge failure closes issuance and enters DIRECT_FALLBACK while previously issued BGP routes remain until grace/TTL expiry. External static routes remain untouched. Interception routes remain when Controller remains healthy. Recovery installs/advertises routes first and reopens issuance after stabilization. FORCE_SURGE cannot bypass readiness; FORCE_DIRECT closes issuance and uses the independent real resolver. Controller failure enters STOPPING and removes owned Controller-bound BGP advertisements; external static interception is not removed.

Every DNS response holds a read barrier through its socket write. Withdrawal drains that barrier after gate closure, preventing an answer already prepared by a dispatcher from being sent after withdrawal. Shutdown stops listeners and monitor, drains writes, withdraws owned BGP routes/releases owned PF and closes SQLite. Static routes remain external. Abrupt process failure relies on BGP session loss; a private PF enable-reference/digest record permits scoped restart cleanup, never a global PF flush.

SQLite uses migrations, foreign keys, WAL/FULL synchronization and transactions; files are private and token ciphertext uses AES-GCM with a separate private key. All unvalidated configuration documents are deep copies. Rejected edits cannot mutate live slices. Runtime rebuild failure attempts the old configuration; mode/override changes invalidate decision generations without rebuilding listeners. Management address/TLS changes use a controlled HTTP restart. Core reconfiguration can briefly interrupt DNS/BGP and is reported through events.

The embedded React UI uses actual REST state and resource CRUD, bounded memory query history and important-event SSE. HTTP connections/requests, sessions, login attempts, tester calls, SSE subscribers and request bodies are bounded. Authentication uses bcrypt, expiring HttpOnly/SameSite cookies and CSRF even when authentication is disabled. Literal-IP/localhost Host validation prevents browser rebinding; remote listeners require TLS, authentication and allowed networks. Secret fields are masked and excluded from export. Prometheus uses bounded state/peer labels, counters and DNS latency histograms.

The LaunchDaemon runs as root for PF and privileged ports, while Surge CLI child calls enter the configured GUI user context and drop credentials. No command invokes a shell around the Surge argument list. A logged-out/missing GUI session marks Surge unavailable while management and real-DNS fallback remain operational. Default service configuration only exposes loopback management; an operator must configure pinned egress before DNS startup.


## Primary sources

- [Surge official CLI documentation](https://manual.nssurge.com/tools/cli.html)
- Installed `/Applications/Surge.app/Contents/Resources/Skills/surge/SKILL.md`
  and its command reference; user specification takes priority over its preference
  for rendered output because this implementation requires structured automation.
- [Apple XNU PF ABI](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/net/pfvar.h)
- [Apple XNU NAT lookup implementation](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/net/pf_ioctl.c)
- Installed macOS `pf.conf(5)` and `pfctl(8)` manuals, and `/etc/pf.conf`.
- [RFC 9460 service binding](https://www.rfc-editor.org/rfc/rfc9460.html)
- [RFC 2308 negative caching](https://www.rfc-editor.org/rfc/rfc2308.html)
- [RFC 8484 DNS over HTTPS](https://www.rfc-editor.org/rfc/rfc8484.html)


## Routing backend abstraction

`internal/routing.Manager` is the routing contract consumed by application health,
issuance state and shutdown. GoBGP is an optional implementation behind its
`Backend` interface. The state machine accepts routing readiness, with no BGP
input. Pure Static never starts a BGP speaker; Hybrid chooses Fake-IP and DNS
interception backends independently. BGP startup/peer failures do not disable
DNS policy, Surge or real resolvers, and cannot block confirmed static Fake-IP
readiness when BGP belongs only to the interception path.

Static rows have `backend=static`, `managed=false`, `installed=null`,
`verified=false` and an explicit operator-confirmed flag. The common manager
never sends them to BGP Add/Delete or alters an external router. A connectivity
probe records an actual timestamped TCP outcome with a limited Mac-only scope;
it neither verifies route installation nor opens the gate without the operator
confirmation. Static interception requires independent persistent-route opt-in.

Routing-only edits suspend Fake-IP leases, drain response writes and replace
only the routing backend. DNS listeners, local maps, classifiers and real/fake
resolvers remain alive. Previously issued BGP Fake-IP TTLs expire before that
backend is closed. Mode/prefix/next-hop changes invalidate old static
confirmations. Surge failure enters DIRECT_FALLBACK immediately on health
failure while BGP routes retain the issuance grace; static declarations are not
withdrawn or reported as removed.
