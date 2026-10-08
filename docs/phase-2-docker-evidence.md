# Docker transparent DNS acceptance — 2026-10-08

IPv4 Phase 2 gate passed using actual macOS PF, not a mock destination lookup.
The tested mode was explicit FORCE_DIRECT; this proves transport and egress,
not BGP, HA, policy classification or production readiness.

## Test path

Docker macvlan client `172.30.31.130` → Mac `bridge101/vmenet1`, gateway
`172.30.31.3` → PF rdr → Controller `192.168.20.2:1054` → pinned `en0` source
`192.168.20.2:20000..20999` → physical gateway `192.168.20.1` → original resolver
`8.8.8.8:53`. The user's gateway source exception remained in place.

A default OrbStack bridge probe returned answers but produced zero PF ingress
hits and zero Controller journal records. It was rejected as acceptance evidence.
A dedicated macvlan forced the real Mac kernel path without a LAN test device.

## Correlated positive test

Run ID `dlz2hon99a34`, private journal `/private/tmp/surge-phase2.Q3si8C/journal.jsonl`.
Client output `/private/tmp/routepilot-docker-probe.jsonl`.

| Transport | Client ID | Source port | Original destination | RCODE | Answers |
|---|---:|---:|---|---|---:|
| UDP | 43952 | 36822 | 8.8.8.8:53 | NOERROR | 2 |
| TCP | 44996 | 34176 | 8.8.8.8:53 | NOERROR | 2 |

Both journal records contain `intercepted=true` and the actual Docker client IP.
`scripts/check_phase2.py` correlated both transports successfully. The bridge
capture shows replies from `8.8.8.8:53` with the matching IDs, demonstrating reverse
translation. The `en0` capture shows UDP upstream ID 31840 from source port 20000
and TCP upstream ID 43590 from source port 20001, directed to the physical gateway
MAC, with replies arriving from that gateway. No upstream request re-entered the
Controller. IDs differ upstream by design; client IDs are restored.

The first macvlan attempt exposed a real UDP NATLOOK bug: Darwin uses protocol
variant `PF_EXTFILTER_APD=1` for UDP state keys. Variant zero failed lookup while
TCP worked. The fix and a Darwin regression test are included; the table above
records the successful post-fix run. Apple source:
[UDP state-key construction](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/net/pf.c)
and [NATLOOK](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/net/pf_ioctl.c).

## Loop-prevention failure injection

The isolated Docker generator injected exactly one UDP query (ID 62001) and one
TCP SYN with the Controller's own source IP `192.168.20.2`, reserved ports 20050
and 20051, destination `8.8.8.8:53`. Bridge capture confirms both arrived.
Actual PF `block drop in quick` counters increased by exactly one UDP packet
and one TCP packet. The journal remained exactly two successful client queries;
there was no additional upstream exchange. This demonstrates bounded rejection
of self-origin traffic even if an upstream request is sent back to the Mac.
It does not claim that the router source exception was actually disabled.

## Scope and cleanup

Mac native race tests and all six Linux offline test suites also passed. Docker
offline suites ran with `--network=none` and require no real Surge/router.
IPv6 transparent deployment has not been verified and remains disabled.
The test process, captures, own PF filter/rdr rules and enable reference were
stopped/released; the temporary Docker macvlan was removed. OrbStack remains
running for further tests. The gateway test route is user-owned and must be
removed manually when no longer needed. Private captures are retained in the
temporary directory for review; no raw packet/query journal is stored in this repo.
