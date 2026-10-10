
# Surge DNS & BGP Controller
## Complete Engineering Specification

## 0. Codex Implementation Directive

Implement the complete `surge-controller` project described in this specification.

Treat this document as the architectural source of truth.

The final result must be a production-quality macOS application/service consisting of:

- A Go backend.
- A complete embedded Web UI.
- A DNS server capable of both normal DNS serving and transparent interception of classic DNS traffic.
- Integration with the currently running Surge for Mac instance.
- DNS policy classification using Surge as the authoritative policy engine.
- Real-IP resolution for traffic that is confidently DIRECT.
- Surge-generated Fake-IP resolution for traffic that must continue through Surge.
- An integrated BGP speaker.
- Automatic BGP route management.
- User-configurable manual BGP routes.
- Health monitoring and deterministic failover/recovery state machines.
- Persistent configuration using SQLite.
- REST API and real-time status/event delivery.
- Tests that do not require a real Surge installation or real router.

Do not reinterpret the core architecture.

When an implementation detail is unspecified, choose the simplest production-safe solution that preserves the invariants in this document.

Document significant implementation decisions in:

`docs/architecture.md`

Do not silently weaken an invariant.

---

# 1. Purpose

The Controller is a combined:

- DNS policy controller
- transparent DNS interception service
- Surge integration layer
- BGP control plane
- health/failover controller
- management application

Its primary purpose is to allow most DIRECT traffic to bypass Surge completely while traffic requiring Surge continues to enter Surge using Fake-IP.

The Controller must avoid creating large routing tables.

It must never generate per-domain `/32` or `/128` BGP routes as part of normal DNS classification.

The normal data-plane model is:

```text
                        ┌───────────────┐
                        │     Surge     │
                        │               │
                        │ Rule Engine   │
                        │ Fake-IP DNS   │
                        │ VM Gateway    │
                        │ HTTP API      │
                        └───────┬───────┘
                                │
                         Fake-IP traffic
                                │
                                ▲
                                │
Client ───────── Gateway ───────┴──────── Internet
   │                │
   │                └── Real-IP ───────────→ Internet
   │
   │ DNS to arbitrary resolver
   ▼
 Gateway
   │
   │ BGP DNS interception route
   ▼
Controller
```

The Controller determines whether a DNS request can safely bypass Surge.

If yes:

```text
DNS → Real-IP
Data → normal gateway → Internet
```

If not:

```text
DNS → Surge Fake-IP
Data → Fake-IP route → Surge
```

---

# 2. Fundamental Design Principle

Surge is the authoritative policy engine.

The Controller must NOT attempt to reproduce the complete Surge rule engine.

The fundamental classification rule is:

> Return Real-IP only when the Controller can confidently establish, using Surge's active policy engine, that the effective result is DIRECT.

Everything else must remain under Surge control.

Therefore:

```text
Definitely DIRECT
        ↓
      Real-IP


Proxy
Unknown
Ambiguous
Unsupported
Insufficient context
        ↓
   Surge Fake-IP
```

False positives toward Surge are acceptable.

False DIRECT decisions are not.

Sending traffic unnecessarily through Surge is acceptable because Surge can still make the final routing decision.

Sending traffic DIRECT when Surge should have processed it is considered a correctness failure.

---

# 3. Important Non-Goals

Do NOT implement:

- A replacement Surge rule engine.
- A custom Fake-IP allocator.
- A packet proxy.
- A TUN/VIF implementation.
- A traffic forwarding engine.
- Automatic UCG/router configuration.
- GeoIP-to-BGP route compilation.
- ASN-to-BGP route compilation.
- DNS-result-to-/32 BGP route generation.
- Large dynamic route tables derived from DNS.
- A full recursive DNS server.
- Automatic modification of the Surge profile.
- Automatic rewriting of Surge rules.

The Controller is a:

> DNS policy controller + transparent DNS interception service + Surge integration layer + BGP control plane.

It is not another Surge.

---

# 4. Target Platform

Primary platform:

```text
macOS
```

Primary deployment:

```text
Mac mini running Surge for Mac
```

The Controller must run independently of the Surge process.

Recommended backend:

```text
Go 1.25+
```

Recommended libraries:

```text
DNS
github.com/miekg/dns

BGP
github.com/osrg/gobgp/v3

Database
SQLite

HTTP/API
Go standard library or chi

Metrics
Prometheus Go client
```

Frontend:

```text
React
TypeScript
Vite
```

The production frontend must be compiled and embedded into the Go binary using `go:embed`.

The normal deployment should require only something similar to:

```text
surge-controller
controller.db
```

Configuration must primarily live in SQLite.

YAML/JSON may exist for import/export, backup, development, and migration, but normal operation must not require manual editing of configuration files.

---

# 5. High-Level Architecture

```text
┌────────────────────────────────────────────────────────────┐
│                    Surge Controller                        │
│                                                            │
│  ┌──────────────────── DNS Ingress ─────────────────────┐ │
│  │                                                      │ │
│  │ Local DNS Listener      Transparent DNS Listener     │ │
│  │ Controller-IP:53        arbitrary-destination:53     │ │
│  │          │                       │                   │ │
│  │          └───────────┬───────────┘                   │ │
│  │                      ▼                               │ │
│  │                DNS Dispatcher                        │ │
│  └──────────────────────┬───────────────────────────────┘ │
│                         │                                  │
│                         ▼                                  │
│                Decision Cache                              │
│                         │                                  │
│                         ▼                                  │
│                Policy Classifier                           │
│                         │                                  │
│                         ▼                                  │
│                   Surge Adapter                            │
│                                                            │
│              ┌──────────┴──────────┐                       │
│              │                     │                       │
│              ▼                     ▼                       │
│       Real DNS Resolver     Surge Fake-IP Resolver         │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ Health Monitor → State Machine → BGP Manager        │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                            │
│  SQLite      REST API      SSE      Metrics      Events    │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐ │
│  │                Management Web UI                    │ │
│  └──────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

---

# 6. DNS Ingress Modes

The Controller must support both modes simultaneously.

## 6.1 Direct DNS

Queries explicitly addressed to the Controller:

```text
Client
  ↓
Controller-IP:53
```

Support:

```text
UDP/53
TCP/53
```

---

## 6.2 Transparent DNS Interception

The Controller must also process DNS packets whose destination IP is NOT a local Controller address.

Example:

```text
Client DNS configuration:

8.8.8.8
```

Normal interception:

```text
Client
   │
   │ UDP/TCP 53 → 8.8.8.8
   ▼
Gateway
   │
   │ BGP route for 8.8.8.8/32
   ▼
Controller
```

The Controller must be capable of accepting and processing such traffic.

This capability must be implemented as an explicit subsystem:

```text
Transparent DNS Listener
```

Do not assume a normal UDP/TCP socket bound only to a local address is sufficient.

The implementation must use an appropriate macOS mechanism to receive traffic routed to the host for non-local destination addresses.

The exact low-level mechanism should be researched and documented in `docs/architecture.md`.

It must preserve the original destination.

---

# 7. Transparent DNS Scope

Version 1 interception scope is explicitly:

```text
UDP port 53
TCP port 53
```

Do NOT claim transparent support for:

```text
DoH
DoT
DoQ
Apple Private Relay
arbitrary encrypted DNS
```

Those may be future features.

The UI should make this limitation clear.

---

# 8. DNS Query Context

Every DNS request must internally become a `DNSQueryContext`.

Conceptually:

```go
type DNSQueryContext struct {
    ClientIP   netip.Addr
    ClientPort uint16

    OriginalDestinationIP   netip.Addr
    OriginalDestinationPort uint16

    Transport DNSProtocol

    Question DNSQuestion

    Intercepted bool

    ReceivedAt time.Time
}
```

For transparent traffic:

```text
ClientIP
192.168.20.50

OriginalDestinationIP
8.8.8.8

OriginalDestinationPort
53

Transport
UDP

Question
google.com A

Intercepted
true
```

The original destination must never be lost.

---

# 9. DNS Classification Pipeline

For every relevant query:

```text
Receive DNS request
        ↓
Validate packet
        ↓
Normalize QNAME
        ↓
Build DNSQueryContext
        ↓
Decision Cache
    ┌───────┴───────┐
    │               │
   HIT             MISS
    │               │
    │        Surge Rule Evaluator
    │               │
    └───────┬───────┘
            ↓
       DecisionResult
      /      |       \
 DIRECT    SURGE    REJECT
    │         │        │
    ▼         ▼        ▼
 Real DNS   Surge     reject
 Resolver   Fake-IP
            Resolver
```

---

# 10. Decision Model

Use a strongly typed decision model.

Conceptually:

```go
type Decision int

const (
    DecisionDirect Decision = iota
    DecisionSurge
    DecisionReject
    DecisionUnknown
)
```

Result:

```go
type DecisionResult struct {
    Decision   Decision
    Confidence Confidence

    Domain   string
    ClientIP netip.Addr

    MatchedRule string

    Policy      string
    FinalPolicy string

    Source DecisionSource

    CreatedAt time.Time
    ExpiresAt time.Time
}
```

The Controller must distinguish:

```text
DIRECT
SURGE
REJECT
UNKNOWN
```

---

# 11. Classification Safety Rule

In AUTO mode:

```text
Definitely DIRECT
→ Real-IP
```

All of:

```text
Proxy
Unknown
Ambiguous
Unsupported
Evaluation timeout
Evaluation failure
Insufficient connection context
```

must default to:

```text
SURGE
```

They must NOT silently become DIRECT.

This behavior changes only when the global state machine has deliberately entered `DIRECT_FALLBACK`.

---

# 12. Surge Integration

Surge remains the source of truth.

Implement a dedicated adapter abstraction.

Conceptually:

```go
type SurgeAdapter interface {
    Health(ctx context.Context) HealthResult

    MatchRule(
        ctx context.Context,
        req RuleMatchRequest,
    ) (RuleMatchResult, error)

    ExplainRule(
        ctx context.Context,
        req RuleMatchRequest,
    ) (RuleExplainResult, error)

    ResolveFakeIP(
        ctx context.Context,
        question DNSQuestion,
    ) (*dns.Msg, error)

    CurrentProfile(
        ctx context.Context,
    ) (ProfileInfo, error)

    PolicyState(
        ctx context.Context,
    ) (PolicyState, error)
}
```

Use current Surge-supported interfaces where appropriate:

```text
surge-cli rule match
surge-cli rule explain
Surge HTTP API
Surge Fake-IP DNS
```

The implementation must use structured/raw CLI output when available rather than parsing decorative human-readable terminal output.

Never execute shell strings containing user-controlled input.

Use argument arrays.

Bad:

```text
sh -c "surge-cli rule match " + domain
```

Good:

```text
exec.Command(binary, args...)
```

---

# 13. Effective DIRECT Detection

Do not simply test whether the matched rule literally says:

```text
DIRECT
```

Policy groups may resolve through several layers.

For example:

```text
Matched rule
    ↓
Hidding
    ↓
GoHome
    ↓
DIRECT
```

The Controller cares about:

```text
effective final policy
```

not just:

```text
matched rule policy name
```

Only a confidently determined effective DIRECT result may bypass Surge.

---

# 14. Context-Dependent Surge Rules

DNS-time classification cannot necessarily know:

```text
URL
User-Agent
future destination port
future protocol
process
application
connection metadata
script result
```

Therefore a rule that cannot be safely evaluated from available DNS context must NOT result in DIRECT.

Use:

```text
insufficient context
      ↓
    SURGE
```

This is intentional.

It is better to send unnecessary traffic to Surge than to bypass Surge incorrectly.

---

# 15. DNS Query Types

A and AAAA are policy-classified.

Other DNS types normally use the Real Resolver unless special handling applies.

Special attention is required for:

```text
HTTPS
SVCB
```

because these may expose:

```text
ipv4hint
ipv6hint
```

and allow clients to bypass A/AAAA Fake-IP steering.

For a domain classified as SURGE:

```text
HTTPS/SVCB
→ suppress address hints / return appropriate NODATA behavior
→ cause client to use A/AAAA
```

For a domain confidently DIRECT:

```text
HTTPS/SVCB
→ normal upstream resolution
```

The exact RFC-correct response behavior must be tested.

---

# 16. Real DNS Resolver

The Controller must contain a production-quality forwarding resolver.

It should support:

```text
UDP DNS
TCP DNS
DoT
DoH
```

Multiple upstream resolvers must be configurable.

For each resolver expose:

```text
Enabled
Protocol
Address/URL
Health
Latency
Query count
Failure count
Last success
Last failure
```

The Controller should maintain an independent DNS response cache.

---

# 17. Original Resolver Preservation

For intercepted DNS requests, support two forwarding strategies.

## Preserve Original Destination

Example:

```text
Client originally queried:
8.8.8.8

Decision:
DIRECT

Controller forwards query to:
8.8.8.8
```

This should be the preferred/default transparent behavior.

## Configured Upstream

Alternatively:

```text
Client → arbitrary DNS destination
Controller → configured resolver pool
```

Expose in UI:

```text
DIRECT DNS Forwarding

○ Preserve original destination
○ Use configured upstream resolvers
```

---

# 18. DNS Forwarding Loop Prevention

This is a critical invariant.

Suppose:

```text
8.8.8.8/32
→ BGP
→ Controller
```

An intercepted request reaches Controller.

If Controller forwards it normally to:

```text
8.8.8.8
```

and its own outbound traffic follows the same interception route, this creates:

```text
Controller
→ Gateway
→ BGP route
→ Controller
→ ...
```

This must be impossible by design.

Implement a dedicated abstraction:

```go
type UpstreamTransport interface {
    Exchange(
        ctx context.Context,
        destination ResolverEndpoint,
        msg *dns.Msg,
    ) (*dns.Msg, error)
}
```

The Controller's own upstream DNS traffic must use an explicit egress-bypass mechanism.

The implementation must research the appropriate macOS routing/socket mechanism and document it.

Do not use a naive generic DNS exchange call if it can re-enter interception.

---

# 19. Fake-IP Resolver

Fake IPs must always be allocated by Surge.

The Controller must never generate its own Fake-IP mapping.

Flow:

```text
Domain
  ↓
Controller
  ↓
Surge Fake-IP DNS
  ↓
Surge allocates Fake-IP
  ↓
Controller returns Surge answer unchanged
```

Defaults may be:

```text
Surge Fake-IP DNS
198.18.0.2:53

Fake-IP network
198.18.0.0/15
```

but both must be configurable.

Do not hard-code these values into business logic.

---

# 20. Decision Cache

Do not invoke Surge CLI for every DNS packet.

Implement a decision cache.

Minimum key dimensions:

```go
type DecisionKey struct {
    Domain      string
    ClientGroup string
    QueryClass  string
}
```

Use Go `singleflight` or equivalent so concurrent identical misses result in one Surge evaluation.

Example configurable TTLs:

```text
DIRECT     5 minutes
SURGE      5 minutes
UNKNOWN    10 seconds
ERROR      1–2 seconds or uncached
```

All must be configurable.

Expose:

```text
Entry count
Hit count
Miss count
Hit rate
Evictions
```

Provide:

```text
Flush all
Flush domain
```

---

# 21. Cache Generation Invalidation

Maintain a global generation:

```text
Generation = 153
```

Each cache entry stores its generation.

When Surge state changes:

```text
153 → 154
```

Old entries become invalid without requiring an O(n) deletion.

Potential invalidation triggers:

```text
Surge profile change
Surge restart
Policy state change
Network environment change
Controller classification settings change
Manual cache flush
```

Use an atomic generation counter.

---

# 22. Rule Evaluation Concurrency

Do not allow unlimited `surge-cli` process creation.

Implement:

```text
bounded worker pool
+
singleflight
+
timeout
```

Initial configurable defaults may resemble:

```text
Max concurrent evaluations: 8
Queue size: 256
Evaluation timeout: 500 ms
```

If the evaluator fails or times out in AUTO/HEALTHY state:

```text
DecisionUnknown
→ SURGE
```

Do not fallback to DIRECT because of an evaluator failure.

---

# 23. BGP Engine

The Controller must include its own BGP speaker.

Use:

```text
GoBGP
```

Do not require ExaBGP as an external daemon.

Support:

```text
IPv4
IPv6
eBGP
iBGP
```

Peer configuration must include at least:

```text
Peer address
Peer ASN
Local ASN
Router ID
Hold time
Keepalive
Enabled
Description
```

Additional advanced settings may be added if useful.

---

# 24. BGP Route Classes

There are three route classes.

## 24.1 Managed Fake-IP Routes

Example:

```text
198.18.0.0/15
```

Purpose:

```text
Fake-IP traffic → Surge
```

Lifecycle:

```text
Health Binding = Surge
```

---

## 24.2 Managed DNS Interception Routes

Examples:

```text
8.8.8.8/32
8.8.4.4/32
1.1.1.1/32
1.0.0.1/32
```

Purpose:

```text
DNS traffic to configured resolver destinations
→ Controller
```

Lifecycle:

```text
Health Binding = Controller
```

These routes MUST NOT normally disappear merely because Surge fails.

---

## 24.3 Manual Routes

Users must be able to create arbitrary manual BGP routes.

Example:

```text
10.20.0.0/16
1.2.3.4/32
2001:db8::/32
```

Manual routes may contain:

```text
Prefix
Next hop
MED
Local preference where applicable
Communities
Enabled
Description
Health binding
```

Manual routes default to:

```text
Health Binding = None
```

Therefore they remain independent of Surge health unless the user explicitly changes the binding.

---

# 25. Health Binding Model

Use a generic model.

Conceptually:

```go
type HealthBinding string

const (
    BindingNone       HealthBinding = "none"
    BindingController HealthBinding = "controller"
    BindingSurge      HealthBinding = "surge"
    BindingDNS        HealthBinding = "dns"
)
```

A route conceptually resembles:

```go
type BGPRoute struct {
    Prefix netip.Prefix

    RouteType RouteType

    Enabled bool
    Managed bool

    HealthBinding HealthBinding

    MED uint32

    Communities []string

    Description string
}
```

This must be extensible.

---

# 26. BGP Route Safety Limit

The Controller exists partly to avoid exhausting the router with routes.

Therefore implement a HARD maximum advertised route count.

Default:

```text
128
```

Configurable by the user.

If an operation would exceed the limit:

```text
Reject advertisement
Generate CRITICAL event
Display UI warning
```

Do NOT merely log a warning and continue.

This applies to all automatically generated routes.

Manual routes must also be counted toward the configured global limit unless a separate explicitly configured limit model is introduced.

No code path may accidentally convert DNS cache entries into BGP `/32` routes.

---

# 27. DNS Interception Route Management

The UI must allow users to manage resolver prefixes.

Example:

```text
DNS Interception

Name             Prefix          Enabled    BGP
Google DNS       8.8.8.8/32      Yes        Announced
Google DNS       8.8.4.4/32      Yes        Announced
Cloudflare       1.1.1.1/32      Yes        Announced
Custom           9.9.9.9/32      Yes        Announced
```

Allow:

```text
Add
Edit
Enable
Disable
Delete
```

Do not require any specific public resolver.

All addresses are user-configurable.

Support IPv4 and IPv6 prefixes.

---

# 28. System State Machine

Implement explicit states:

```go
type SystemState int

const (
    StateStarting SystemState = iota
    StateRecovering
    StateHealthy
    StateSurgeDegraded
    StateDirectFallback
    StateStopping
)
```

Do not represent system health with a single boolean.

---

# 29. Normal Startup

Conceptually:

```text
STARTING
   ↓
Controller DNS ready
   ↓
Surge reachable
   ↓
Fake-IP DNS healthy
   ↓
BGP session healthy
   ↓
DNS interception routes active
   ↓
Fake-IP routes installed
   ↓
RECOVERING
   ↓
route stabilization delay
   ↓
allow Fake-IP issuance
   ↓
HEALTHY
```

The exact startup order must preserve all invariants.

---

# 30. Surge Failure

When Surge fails but Controller remains healthy:

```text
HEALTHY
   ↓
SURGE_DEGRADED
```

Immediately:

```text
Stop issuing NEW Fake-IP answers.
```

New DNS requests should transition to real resolution according to `DIRECT_FALLBACK` behavior.

DNS interception routes MUST remain advertised because the Controller is still alive.

Thus:

```text
Client
  ↓ arbitrary DNS destination
Gateway
  ↓ BGP
Controller
  ↓
Real DNS
```

The network continues operating without Surge.

---

# 31. Existing Fake-IP Grace Period

Existing clients may still possess recently issued Fake-IP answers.

Therefore do NOT immediately withdraw the Fake-IP route.

Failure sequence:

```text
Detect Surge failure
        ↓
Stop issuing new Fake IPs
        ↓
Enter SURGE_DEGRADED
        ↓
New DNS queries use Real DNS
        ↓
Wait configured Fake-IP grace period
        ↓
Withdraw routes bound to Surge
        ↓
Enter DIRECT_FALLBACK
```

Grace period must be configurable.

Initial default may be approximately:

```text
60 seconds
```

but must not be hard-coded as an immutable assumption.

---

# 32. Surge Recovery

Recovery order must be the reverse.

```text
Surge becomes healthy
        ↓
Fake-IP DNS probe succeeds
        ↓
BGP healthy
        ↓
Announce Surge-bound Fake-IP routes
        ↓
Confirm route installation / wait stabilization delay
        ↓
RECOVERING
        ↓
Enable Fake-IP issuance
        ↓
HEALTHY
```

Critical rule:

> Fake-IP routes must be ready before the Controller begins issuing new Fake-IP answers.

---

# 33. Controller or Mac Failure

DNS interception routes are bound to Controller health.

If the Controller process, Mac, network interface, or BGP session disappears:

```text
BGP session disappears
        ↓
DNS interception routes disappear
        ↓
Gateway uses normal routing
        ↓
Client reaches original configured DNS server
```

Example:

```text
Client thinks DNS = 8.8.8.8
```

Controller healthy:

```text
8.8.8.8/32
→ BGP
→ Controller
```

Controller dead:

```text
BGP route disappears

8.8.8.8
→ normal default route
→ actual 8.8.8.8
```

The client must not require:

```text
DHCP refresh
DNS setting change
secondary Controller
```

This BGP behavior is the primary DNS HA mechanism.

Do not design a separate mandatory DNS VIP architecture.

---

# 34. Two-Level Failure Model

The intended failure behavior is:

## Normal

```text
Client DNS
   ↓
Controller
   ↓
DIRECT / Surge
```

## Surge Failure

```text
Client DNS
   ↓
Controller
   ↓
Real DNS only
```

## Controller/Mac Failure

```text
Client DNS
   ↓
Original configured DNS server
```

Client DNS configuration remains unchanged in all three states.

---

# 35. Global Operating Modes

The UI must expose:

```text
AUTO
FORCE DIRECT
FORCE SURGE
```

## AUTO

Use normal classification and state machine.

## FORCE DIRECT

All relevant DNS queries use Real-IP resolution.

Surge-bound Fake-IP routes are withdrawn according to safe grace-period behavior.

DNS interception routes remain active while Controller remains healthy.

## FORCE SURGE

All eligible A/AAAA queries use Surge Fake-IP.

This mode is primarily for diagnostics.

The UI must prominently display:

```text
Manual override active
```

when not in AUTO.

---

# 36. Manual Overrides

Support a small Controller-level override system.

Examples:

```text
Force DIRECT
Force SURGE
Force REJECT
```

Possible matching:

```text
exact domain
domain suffix
```

Do not build a second full Surge rule language.

Priority:

```text
Controller safety rules
        ↓
Explicit Controller override
        ↓
Surge classification
```

Overrides must be manageable in the UI.

---

# 37. Health Monitoring

Monitor independently:

```text
Controller
DNS listener
Transparent DNS listener
Real DNS resolver
Surge process/API
Surge CLI
Surge Fake-IP DNS
BGP engine
Each BGP peer
Database
```

Each health component should expose:

```text
State
Last success
Last failure
Latency
Consecutive successes
Consecutive failures
Last error
```

Use hysteresis.

Example configurable values:

```text
Failures before DOWN: 3
Successes before UP: 3
```

Avoid route flapping.

---

# 38. Management UI Requirement

The Web UI is a first-class project requirement.

It is not an optional debug interface.

The application must be fully usable without editing configuration files.

The UI should feel like a native network-management application.

Desired characteristics:

```text
clean
dense but readable
desktop-first
responsive
dark/light theme
real-time state
clear health indicators
minimal unnecessary animation
```

A UniFi/Surge-like information density is appropriate, without copying proprietary visual assets.

---

# 39. UI Navigation

Use approximately:

```text
Dashboard

DNS
 ├─ Overview
 ├─ Queries
 ├─ Decision Cache
 └─ Interception

Rule Test

BGP
 ├─ Overview
 ├─ Peers
 └─ Routes

Health

Events

Settings
 ├─ General
 ├─ DNS
 ├─ Surge
 ├─ BGP
 ├─ Cache
 ├─ Logging
 ├─ Security
 └─ Advanced
```

---

# 40. Dashboard

The dashboard should expose at minimum:

```text
System state
Global mode

Controller health
DNS health
Surge health
BGP health
Fake-IP state

DNS queries/minute

DIRECT percentage
SURGE percentage
REJECT percentage

Cache hit rate

Intercepted vs directly addressed DNS

BGP peer state
Number of advertised routes

Recent important events
```

Example:

```text
┌─────────────────────────────────────────────────────────┐
│ Surge Controller                    AUTO ● HEALTHY      │
├─────────────────────────────────────────────────────────┤
│ DNS          Surge         BGP          Fake-IP         │
│ ● Healthy    ● Healthy     ● Established ● Active      │
│                                                         │
│ Queries/min  DIRECT        SURGE        Cache Hit       │
│ 2,104        72.1%         27.9%        96.8%           │
├─────────────────────────────────────────────────────────┤
│ DNS Ingress                                             │
│                                                         │
│ Intercepted       88%                                   │
│ Direct            12%                                   │
├─────────────────────────────────────────────────────────┤
│ BGP                                                     │
│ Peer             State          Advertised              │
│ 192.168.x.x      Established    6                       │
├─────────────────────────────────────────────────────────┤
│ Recent Events                                           │
│ 21:03 Policy state changed                              │
│ 21:03 Decision cache generation changed                 │
└─────────────────────────────────────────────────────────┘
```

---

# 41. DNS Query Inspector

Maintain an in-memory bounded query history.

Do not persist every DNS request by default.

Table:

```text
Time      Client          Destination     Domain          Type  Decision
21:13:01  192.168.x.51   8.8.8.8         google.com      A     SURGE
21:13:02  192.168.x.72   1.1.1.1         baidu.com       A     DIRECT
```

Filters:

```text
Domain
Client
Original resolver
Query type
DIRECT
SURGE
REJECT
Cache hit/miss
Intercepted/direct
```

Query detail should show:

```text
Client IP/port

Original destination IP/port

Directly addressed or intercepted

Transport

Domain
Query type

Decision
Confidence

Matched Surge rule
Policy
Effective final policy

Cache hit/miss

Resolver used

Returned answers

Total latency
Classification latency
Resolver latency
```

---

# 42. Rule Test UI

Provide an interactive tester.

Inputs:

```text
Domain
Client IP
Protocol
Port
Additional context where supported
```

Result:

```text
Matched Rule
      ↓
Policy
      ↓
Policy Group Chain
      ↓
Effective Final Policy
      ↓
Controller Decision
      ↓
DNS Behavior
```

Example:

```text
api.openai.com

Matched:
RULE-SET,OpenAI,LX_US

Policy:
LX_US
 ↓
US-Fallback
 ↓
Proxy-US-01

Controller Decision:
SURGE

DNS Behavior:
Fake-IP
```

This page should support an optional detailed `rule explain` view.

---

# 43. DNS Interception UI

Provide:

```text
DNS Interception
────────────────────────────────────────

Status                   ● Active

UDP/53                   Enabled
TCP/53                   Enabled

DIRECT Forwarding
Preserve Original Resolver

Resolver Prefixes

Name            Prefix          Enabled    BGP State
Google          8.8.8.8/32      Yes        Announced
Cloudflare      1.1.1.1/32      Yes        Announced
Custom          x.x.x.x/32      Yes        Announced

                            [+ Add]
```

Support:

```text
Add
Edit
Delete
Enable
Disable
```

---

# 44. BGP UI

Display:

```text
Local ASN
Router ID
Engine state
Advertised route count
Maximum route limit
```

Peers:

```text
Peer
ASN
State
Uptime
Prefixes
Last error
```

Routes:

```text
Prefix          Type                    State       Health Binding
198.18.0.0/15   Managed / Fake-IP       Announced   Surge
8.8.8.8/32      Managed / DNS           Announced   Controller
1.1.1.1/32      Managed / DNS           Announced   Controller
10.20.0.0/16    Manual                  Announced   None
```

---

# 45. Manual BGP Route Editor

Users must be able to manually create BGP entries.

Editor:

```text
Prefix
Next Hop
MED
Local Preference where applicable
Communities
Description
Enabled

Health Binding:
None
Controller
Surge
DNS
```

Manual routes must be persisted.

Manual routes default to:

```text
Health Binding = None
```

A user may deliberately configure:

```text
Health Binding = Surge
```

in which case the route follows Surge health.

---

# 46. Settings UI

All major parameters must be configurable through UI.

Categories:

## General

```text
Controller name
Web listen address
Global mode
Theme
```

## DNS

```text
DNS listener
Transparent interception
Real resolver configuration
Preserve original resolver
Timeouts
Response cache
```

## Surge

```text
CLI path
HTTP API endpoint
API credentials/token
Fake-IP DNS endpoint
Fake-IP prefixes
Health settings
```

## BGP

```text
Enabled
Local ASN
Router ID
Peers
Route limit
Timers
```

## Cache

```text
Decision TTLs
Maximum entries
DNS response cache settings
```

## Health

```text
Probe interval
Failure threshold
Recovery threshold
Grace period
Route stabilization delay
```

## Logging

```text
Level
Query logging
Retention
Persistence
```

## Security

```text
UI authentication
Session settings
Allowed management networks
```

## Advanced

Only expose settings that genuinely require advanced control.

---

# 47. Configuration Persistence

Use:

```text
SQLite
```

At minimum create logical tables for:

```text
settings
bgp_peers
bgp_routes
dns_interception_routes
overrides
events
schema_migrations
```

Decision cache remains memory-first.

DNS query history remains a bounded in-memory ring buffer by default.

Do not write every DNS query to SQLite.

Provide configuration:

```text
Export
Import
Backup
Restore
```

using JSON or another stable format.

---

# 48. REST API

The frontend must use the Controller API rather than directly manipulating backend objects.

Minimum API surface:

```text
GET  /api/v1/status
GET  /api/v1/health

GET  /api/v1/settings
PUT  /api/v1/settings

GET  /api/v1/dns/queries
GET  /api/v1/dns/cache
POST /api/v1/dns/cache/flush

GET    /api/v1/dns/interception
POST   /api/v1/dns/interception
PUT    /api/v1/dns/interception/:id
DELETE /api/v1/dns/interception/:id

POST /api/v1/test/domain

GET /api/v1/surge/status

GET    /api/v1/bgp/status
GET    /api/v1/bgp/peers
POST   /api/v1/bgp/peers
PUT    /api/v1/bgp/peers/:id
DELETE /api/v1/bgp/peers/:id

GET    /api/v1/bgp/routes
POST   /api/v1/bgp/routes
PUT    /api/v1/bgp/routes/:id
DELETE /api/v1/bgp/routes/:id

GET    /api/v1/overrides
POST   /api/v1/overrides
PUT    /api/v1/overrides/:id
DELETE /api/v1/overrides/:id

GET /api/v1/events

GET /api/v1/mode
PUT /api/v1/mode
```

Use appropriate HTTP semantics and validation.

---

# 49. Real-Time UI

Use SSE unless WebSocket provides a clear implementation advantage.

Real-time events should include:

```text
system state change
health change
Surge state change
BGP peer state
route announce/withdraw
mode change
critical warning
important configuration change
```

Do not stream every DNS packet through SSE by default.

DNS query tables may use bounded polling or an explicitly enabled live stream.

---

# 50. Event Journal

Maintain structured events with severity:

```text
INFO
WARNING
ERROR
CRITICAL
```

Example:

```text
21:04:13 Surge became unhealthy
21:04:13 Entering SURGE_DEGRADED
21:04:13 New Fake-IP issuance disabled
21:05:13 Fake-IP grace period expired
21:05:13 Withdrawing 198.18.0.0/15
21:05:14 Entered DIRECT_FALLBACK
```

Persist operationally important events.

Do not persist every DNS query as an event.

---

# 51. Metrics

Expose:

```text
/metrics
```

Prometheus format.

At minimum:

```text
dns_queries_total
dns_direct_total
dns_surge_total
dns_reject_total

dns_intercepted_total
dns_direct_addressed_total

decision_cache_hits_total
decision_cache_misses_total

surge_rule_match_duration_seconds
surge_rule_match_errors_total

fake_ip_queries_total
fake_ip_errors_total

real_dns_queries_total
real_dns_errors_total

bgp_session_up
bgp_routes_announced

controller_state
```

Add useful latency histograms.

Avoid labels that cause unbounded cardinality such as domain name.

---

# 52. Logging and Privacy

Use structured logging.

Example:

```json
{
  "event": "dns_decision",
  "client": "192.168.x.x",
  "domain": "example.com",
  "decision": "surge",
  "source": "cache",
  "latency_ms": 0.3
}
```

However, DNS queries contain sensitive browsing metadata.

Default behavior:

```text
Query history:
memory only

Persistent query log:
disabled

Bounded memory retention:
enabled
```

Allow users to disable domain logging entirely.

Never log Surge API secrets.

---

# 53. Security

The management UI must support optional authentication.

At minimum:

```text
Username/password
Secure password hashing
Session cookie
CSRF protection
```

Management listen address must be configurable.

Validate all:

```text
IP addresses
prefixes
ASNs
ports
communities
URLs
DNS endpoints
file paths where applicable
```

Never interpolate user-controlled values into shell commands.

Sensitive API tokens must:

```text
never be returned unnecessarily to frontend
never appear in logs
be stored appropriately
```

---

# 54. Graceful Shutdown

Handle:

```text
SIGINT
SIGTERM
```

Safe sequence:

```text
Enter STOPPING
        ↓
Stop accepting new DNS requests
        ↓
Finish/cancel in-flight work
        ↓
Withdraw managed BGP routes when possible
        ↓
Stop BGP engine
        ↓
Flush required persistent state
        ↓
Close database
        ↓
Exit
```

If the process crashes before withdrawal, BGP session loss must still remove its routes from peers.

---

# 55. macOS Service Integration

Provide a launchd service definition.

Requirements:

```text
start automatically
restart on unexpected failure
appropriate working directory
configurable logging
```

Document installation and removal.

Do not require the user to manually start the Controller after reboot.

---

# 56. Project Structure

Use approximately:

```text
surge-controller/
│
├── cmd/
│   └── controller/
│       └── main.go
│
├── internal/
│   ├── app/
│   ├── config/
│   ├── database/
│   ├── dnsserver/
│   │   ├── server.go
│   │   ├── dispatcher.go
│   │   ├── transparent.go
│   │   └── context.go
│   │
│   ├── classifier/
│   ├── cache/
│   │
│   ├── surge/
│   │   ├── adapter.go
│   │   ├── cli.go
│   │   ├── api.go
│   │   ├── fakeip.go
│   │   └── health.go
│   │
│   ├── resolver/
│   │   ├── real.go
│   │   ├── upstream_transport.go
│   │   └── cache.go
│   │
│   ├── bgp/
│   │   ├── manager.go
│   │   ├── peer.go
│   │   ├── route.go
│   │   └── gobgp.go
│   │
│   ├── state/
│   ├── health/
│   ├── events/
│   ├── metrics/
│   ├── security/
│   └── api/
│
├── web/
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx
│   │   │   ├── DNSOverview.tsx
│   │   │   ├── DNSQueries.tsx
│   │   │   ├── DNSInterception.tsx
│   │   │   ├── RuleTest.tsx
│   │   │   ├── BGPOverview.tsx
│   │   │   ├── BGPPeers.tsx
│   │   │   ├── BGPRoutes.tsx
│   │   │   ├── Health.tsx
│   │   │   ├── Events.tsx
│   │   │   └── Settings.tsx
│   │   ├── components/
│   │   ├── hooks/
│   │   └── api/
│   └── dist/
│
├── migrations/
│
├── tests/
│   ├── integration/
│   ├── fixtures/
│   └── mocks/
│
├── docs/
│   └── architecture.md
│
├── launchd/
├── scripts/
│
├── go.mod
├── package.json
├── Makefile
└── README.md
```

The exact structure may vary when justified, but maintain clear subsystem boundaries.

---

# 57. Core Interfaces

External dependencies must be abstracted.

Conceptually:

```go
type Classifier interface {
    Classify(
        ctx context.Context,
        query DNSQueryContext,
    ) (DecisionResult, error)
}
```

```go
type SurgeAdapter interface {
    MatchRule(
        ctx context.Context,
        req RuleMatchRequest,
    ) (RuleMatchResult, error)

    ResolveFakeIP(
        ctx context.Context,
        question DNSQuestion,
    ) (*dns.Msg, error)

    Health(
        ctx context.Context,
    ) HealthResult
}
```

```go
type Resolver interface {
    Resolve(
        ctx context.Context,
        req ResolveRequest,
    ) (*dns.Msg, error)
}
```

```go
type BGPManager interface {
    AddRoute(ctx context.Context, route BGPRoute) error
    RemoveRoute(ctx context.Context, id string) error

    Announce(ctx context.Context, id string) error
    Withdraw(ctx context.Context, id string) error

    Status() BGPStatus
}
```

```go
type StateManager interface {
    State() SystemState
    Mode() OperatingMode
}
```

Do not make the DNS server depend directly on GoBGP or Surge CLI implementation details.

---

# 58. Testability

Tests must not require:

```text
real Surge
real UCG/router
Internet access
real BGP peer
```

Provide mocks/fakes:

```text
MockSurge
MockResolver
MockUpstreamTransport
MockBGP
MockHealthMonitor
```

Use integration fixtures for DNS packets and state transitions.

---

# 59. Required Tests

At minimum implement tests for:

### DNS classification

```text
DIRECT domain → Real-IP

Proxy domain → Fake-IP

Unknown → Fake-IP

Ambiguous → Fake-IP

Rule evaluator timeout → not DIRECT

Rule evaluator error → not DIRECT
```

### Domain/IP ambiguity

```text
Domain A → DIRECT
Domain B → SURGE
Both resolve to same real IP

Result:
A remains Real-IP path
B remains Fake-IP path
```

### Cache

```text
Cache hit
Cache miss
singleflight
Generation invalidation
Domain flush
Global flush
```

### Transparent DNS

```text
Direct-addressed DNS request

Intercepted UDP/53 request

Intercepted TCP/53 request

Original destination preserved

Preserve-original-resolver forwarding

Configured-upstream forwarding
```

### Loop prevention

```text
Intercepted resolver route exists
Controller forwards upstream request
Controller upstream request must not re-enter interception
```

### Surge failure

```text
HEALTHY
→ SURGE_DEGRADED

New Fake-IP issuance stops immediately

DNS interception routes remain active

New queries resolve Real-IP

Fake-IP route remains during grace period

Fake-IP route withdraws after grace period

DIRECT_FALLBACK reached
```

### Recovery

Verify strict ordering:

```text
Surge healthy
→ Fake DNS healthy
→ Fake-IP route announced
→ stabilization
→ Fake-IP issuance enabled
→ HEALTHY
```

Fake-IP must never be issued before route readiness.

### BGP

```text
Peer creation
Peer update
Peer deletion
Session reconnect

Managed Fake-IP route

Managed DNS interception route

Manual route add/update/delete

Manual route with BindingNone survives Surge failure

Manual route with BindingSurge withdraws on Surge failure

Route limit enforced
```

### Modes

```text
AUTO

FORCE_DIRECT

FORCE_SURGE
```

### Persistence

```text
Settings survive restart
Peers survive restart
Routes survive restart
Interception prefixes survive restart
Overrides survive restart
```

---

# 60. Performance Targets

For cached DNS decisions, excluding external upstream latency:

```text
P50 < 1 ms
P99 < 5 ms
```

Target:

```text
>= 10,000 cached DNS QPS
```

Do not let Surge CLI performance determine cache-hit DNS performance.

Rule evaluation must use:

```text
bounded concurrency
singleflight
timeouts
```

No unbounded:

```text
goroutine creation
process creation
queue growth
memory growth
```

Use bounded in-memory query/event histories.

---

# 61. Development Phases

Implement incrementally.

## Phase 1 — Core DNS

Implement:

```text
DNS server
DNSQueryContext
Classifier interfaces
Surge adapter
Real resolver
Fake-IP resolver
Decision cache
singleflight
Mocks
Core tests
```

Acceptance:

```text
DIRECT → Real-IP
SURGE → Surge Fake-IP
UNKNOWN → Surge Fake-IP
```

---

## Phase 2 — Transparent DNS

Implement:

```text
Transparent DNS ingress
Original destination capture
UDP/53
TCP/53
Original-resolver preservation
Upstream egress bypass
Loop-prevention tests
```

Do not continue until transparent interception and loop prevention are proven.

---

## Phase 3 — Management Layer

Implement:

```text
SQLite
REST API
Embedded React UI

Dashboard
DNS query inspector
Decision cache
Rule tester
Settings
Events
```

The application should now be usable without editing configuration files.

---

## Phase 4 — BGP

Implement:

```text
GoBGP
Peer management
Managed Fake-IP routes
Managed DNS interception routes
Manual routes
Health bindings
Hard route limit
BGP UI
```

---

## Phase 5 — HA State Machine

Implement:

```text
Health monitor
STARTING
RECOVERING
HEALTHY
SURGE_DEGRADED
DIRECT_FALLBACK
STOPPING

Grace period
Recovery ordering
Global modes
Route health bindings
```

---

## Phase 6 — Production Hardening

Implement:

```text
Authentication
Metrics
Privacy controls
Configuration import/export
launchd
Performance testing
Stress testing
Failure injection
Documentation
Packaging
```

After every phase:

```text
make test
frontend tests
frontend build
Go production build
```

`make test` builds the embedded frontend assets before compiling Go packages.
`web/dist` is generated during builds and is not tracked in source control.

The project must remain runnable throughout development.

---

# 62. Architectural Invariants

These are mandatory.

## Invariant 1 — DIRECT requires confidence

Only a confidently established effective DIRECT result may return Real-IP in normal AUTO operation.

---

## Invariant 2 — uncertainty stays with Surge

```text
UNKNOWN
ERROR
AMBIGUOUS
UNSUPPORTED
INSUFFICIENT CONTEXT
```

must not silently become DIRECT during normal operation.

---

## Invariant 3 — Surge owns Fake-IP

The Controller must never independently allocate Fake IPs.

Fake-IP answers must originate from Surge.

---

## Invariant 4 — route before Fake-IP

The required Fake-IP route must be ready before new Fake-IP answers are issued.

---

## Invariant 5 — stop Fake-IP before route withdrawal

During failure:

```text
Stop issuing new Fake IPs
        ↓
Grace period
        ↓
Withdraw Fake-IP route
```

Never reverse this order.

---

## Invariant 6 — manual BGP routes are independent by default

Manual routes default to:

```text
Health Binding = None
```

Surge failure must not withdraw them unless explicitly configured.

---

## Invariant 7 — hard BGP route limit

The Controller must enforce a hard advertised-route limit.

No subsystem may bypass it.

---

## Invariant 8 — Surge remains policy source of truth

Do not implement a competing full Surge rule engine.

---

## Invariant 9 — transparent classic DNS

The Controller must be able to process configured intercepted UDP/TCP port 53 traffic whose original destination is not a local Controller IP.

---

## Invariant 10 — preserve original destination

Transparent DNS processing must preserve:

```text
Client address
Original destination address
Transport
DNS question
```

---

## Invariant 11 — upstream loop prevention

Controller-generated upstream DNS traffic must not be intercepted back into the Controller.

A DNS forwarding loop must be impossible by design.

---

## Invariant 12 — different BGP health lifecycles

Fake-IP routes:

```text
Health Binding = Surge
```

DNS interception routes:

```text
Health Binding = Controller
```

Do not conflate them.

---

## Invariant 13 — Surge failure does not remove DNS interception

If Surge fails but Controller remains healthy:

```text
Keep DNS interception routes
Enter DIRECT_FALLBACK
Resolve new requests using real DNS
```

---

## Invariant 14 — Controller failure restores original DNS path

Loss of Controller/Mac/BGP must cause DNS interception routes to disappear through BGP session loss so clients can reach their originally configured DNS servers through normal routing.

---

## Invariant 15 — no DNS-to-BGP explosion

DNS observations, DNS cache entries, Real-IP answers, and Fake-IP mappings must never automatically generate arbitrary per-address BGP routes.

---

# 63. Final Intended Network Behavior

## Healthy

```text
Client
  │
  │ DNS → configured resolver
  ▼
Gateway
  │
  │ BGP DNS interception
  ▼
Controller
  │
  ├── confidently DIRECT
  │         ↓
  │      Real DNS
  │         ↓
  │      Real-IP
  │         ↓
  │ Client data → normal Internet path
  │
  └── everything else
            ↓
         Surge DNS
            ↓
          Fake-IP
            ↓
      Client data → Surge
```

## Surge fails

```text
Client
  │
  │ DNS
  ▼
Controller
  │
  │ DIRECT_FALLBACK
  ▼
Real DNS
  │
  ▼
Real-IP
```

DNS interception remains operational.

After grace period, Surge-bound Fake-IP routes are withdrawn.

## Controller/Mac fails

```text
BGP session disappears
        ↓
DNS interception routes disappear
        ↓
Client DNS packets follow normal default route
        ↓
Client reaches its originally configured DNS resolver
```

No client DNS configuration change is required.

---

# 64. Definition of Done

The project is complete when:

1. It builds into a production macOS executable.
2. The Web UI is embedded and provides complete configuration/status management.
3. It can serve directly addressed DNS.
4. It can process transparently intercepted UDP/TCP DNS to configured non-local resolver destinations.
5. It preserves original DNS destination information.
6. It can classify DNS requests using the active Surge policy engine.
7. It returns Real-IP only for confidently DIRECT traffic.
8. It obtains Fake-IP exclusively from Surge.
9. It prevents its own upstream DNS traffic from looping through interception.
10. It runs an integrated BGP speaker.
11. It manages Fake-IP routes separately from DNS interception routes.
12. Users can manually create/edit/delete BGP routes from the UI.
13. BGP routes support health bindings.
14. It enforces a hard route limit.
15. Surge failure results in Controller-based Real-DNS fallback without removing DNS interception routes.
16. Controller/Mac failure naturally restores access to clients' original DNS servers through BGP withdrawal.
17. The UI exposes DNS decisions, original resolver destinations, BGP state, Surge state, health, events, settings and testing tools.
18. Configuration persists in SQLite.
19. All critical components can be tested without Surge or network hardware.
20. All required unit/integration tests pass.
21. The fourteen core behavioral invariants plus the no-route-explosion invariant are covered by automated tests.
22. `docs/architecture.md` explains the chosen macOS transparent-ingress and upstream-egress-bypass implementations.
23. `README.md` contains build, installation, launchd, initial setup, development and troubleshooting documentation.

Do not declare the project complete with placeholder UI, stub BGP, mocked production paths, TODO implementations, or unimplemented failure behavior.

The finished result should be usable as the actual Controller, not merely as a proof of concept.