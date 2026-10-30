# Acceptance evidence — 2026-10-08

## Stable v1.0.4 release gate — 2026-10-16

This release consolidates the final behavior from beta.1–28 since stable
v1.0.3 (`5ca0fd97496776d9949dbe3218d57f982dc0e04f`). The full user-facing
change record is [.gitea/release-notes/v1.0.4.md](../.gitea/release-notes/v1.0.4.md).
Source identity is `1.0.4 / build 111`; the skill installer pins the new stable
tag. Worktree baseline is `de2d7a3855fece6d99868b5d31b70483caad8d41` on both forges.

Local `env GOCACHE=/private/tmp/routepilot-next-hop-go-build make verify-release`
and `make fuzz` passed: 36 frontend files / 193 tests, all Go race / vet, native
core / WKWebView / multiple windows, 17 pages / 14 settings categories, nine
cache UI checks and full package validation. Fuzz executed 217,336 cases. Local
App/backend identity is `1.0.4 / build 111`; reports are `local-verification.log`,
`local-fuzz.log` and `local-package-verification.json`. CI regression/full release,
publication and dual-forge artifact verification have now passed:

- Tag `v1.0.4` resolves to `6d336a770028a18690914eeaf2733cdd42e86ed3`, annotated
  object `cd460ac01058ff470afdd5fb9762a6f56e208219`; both remotes agree.
- [GitHub regression](https://github.com/KnotDing/routepilot/actions/runs/38844597375)
  and the complete [stable Release workflow](https://github.com/KnotDing/routepilot/actions/runs/38844625516)
  succeeded, including their required native/browser checks and fuzz.
- [Gitea Release](https://gitea.knotding.com/knot/routepilot/actions/runs/2109)
  succeeded by mirroring GitHub; fallback build skipped.
- [GitHub Release](https://github.com/KnotDing/routepilot/releases/tag/v1.0.4)
  and [Gitea mirror](https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4)
  are non-draft stable releases (`prerelease=false`), each with only DMG, ZIP
  and SHA256SUMS. All six attachments downloaded, with identical hashes across
  forges. ZIP, signatures, DMG integrity/layout, binary identity and actual
  **App/backend 1.0.4 / build 37** passed; CI build differs from local build 111.
  GitHub latest also points to v1.0.4.
- All three public files referenced by the stable skill installer downloaded
  and matched tag contents; no skill was installed.

Evidence is under `artifacts/releases/v1.0.4/`, including
`publication-verification.json` and `skill-source/verification.json`. Prior
packages and bin files were preserved under
`artifacts/pre-v1.0.4-backup-20261016/`. The final handoff is a later docs-only
commit; the release tag remains unchanged. No live installation/restart was
performed, and no new live gateway/LAN/IPv6 or notarization acceptance is claimed.

## Automatic Surge VM IPv4 next hop — 2026-10-16

`surge.next_hop4` is optional for IPv4 Fake-IP routes. Explicit values take
priority; blank values use the connected backend's CLI VMNET `self-ip`.
The saved value remains blank. Runtime state exposes the detected/effective
address, pending transition and read error through the existing Surge/routing
status endpoints. IPv6 keeps its previous explicit-address requirements.

The existing monitor probes at most every 30 seconds, with a two-second timeout.
Failures pause issuance and preserve old paths for issued TTLs/grace. Address
changes drain old answers before updating paths, retaining the DNS runtime and
BGP speaker. Static confirmation binds to the detected address and requires
explicit confirmation again after an address change, including after restart.

Executed for this change:

- `npm --prefix web test -- --reporter=dot`: 36 files / 193 tests passed.
  The final focused four-file rerun passed all 18 tests after a static-confirmation
  display refinement. Existing monitoring fixture duplicate-key warnings remain.
- `npm --prefix web run build`: management UI and offline notes built; the existing
  bundle-size advisory remains.
- `env GOCACHE=/private/tmp/routepilot-next-hop-go-build go test -race
  ./internal/surge ./internal/routing ./internal/config ./internal/app`: passed.
  Coverage includes strict VMNET parsing, manual priority, bounded probing,
  Fake-IP TTL drain/recovery, independent routes, unsafe detected addresses,
  persistence and address-bound static confirmation. Tests use mock Surge,
  temporary private databases and random loopback ports; initial sandbox bind
  restrictions were resolved by running isolated integration tests outside it.
- `go vet` for the same four packages and `go build ./...` with the same cache:
  passed, with frontend assets built before Go embedding.
- `node scripts/ui_forms_check.cjs`: 17 pages / 14 categories at widths 1440,
  1120 and 390 passed; the optional IPv4 field and detected-address hint were
  checked at each width. Mobile screenshot visually inspected.
- A temporary read-only program using the new `surge.ReadVMNextHop4` with the
  installed Surge CLI returned **192.168.20.141**. It only read VMNET status;
  it did not change configuration or restart services. Probe source and UI
  screenshots remain under ignored `artifacts/`.

The implementation-stage checks above preceded beta.28 publication; final release
evidence is recorded below. Nothing was installed or applied to the live backend.
Live BGP gateway RIB/FIB, DHCP address changes and LAN packet paths were not
exercised; isolated state-machine checks do not establish those outcomes.

Follow-up: the Fake-IP prefix editor now uses a single-line comma-separated
field (ASCII or Chinese commas), retaining the configuration array format.
The three relevant frontend files passed all 16 tests and the production build
passed. An isolated browser with a mock API verified typing a trailing comma,
adding an IPv6 prefix, the submitted two-element array, and no horizontal
overflow at width 390. The screenshot is under
`artifacts/surge-next-hop-2026-10-16/prefix-single-line-390.png`.

## Beta.28 release gate — 2026-10-16

Local `env GOCACHE=/private/tmp/routepilot-next-hop-go-build make verify-release`
passed: 36 frontend files / 193 tests, all Go race tests and vet, native core /
WKWebView / multi-window checks, 17 pages / 14 settings categories, nine cache UI
checks, full App packaging and ZIP / signatures / DMG layout validation. Source
and local package identity is `1.0.4-beta.28 / build 110`. Evidence is under
`artifacts/releases/v1.0.4-beta.28/` (`local-verification.log` and
`local-package-verification.json`). Publication is verified: tag `v1.0.4-beta.28` resolves to
`bfb4501848212838f819dc1158b193e6ca859bea`, annotated tag object
`2e9345739823347c8091e83ba9f57b5d844e0ca2`, identical on GitHub and Gitea.

- [GitHub regression](https://github.com/KnotDing/routepilot/actions/runs/38834865752)
  and [Release](https://github.com/KnotDing/routepilot/actions/runs/38834901914)
  succeeded. Regression includes version-parser fuzz.
- [Gitea Release](https://gitea.knotding.com/knot/routepilot/actions/runs/2108)
  succeeded by mirroring GitHub; fallback build skipped.
- [GitHub Release](https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.28)
  and [Gitea mirror](https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.28)
  are non-draft prereleases, each containing only DMG, ZIP and SHA256SUMS.
  All six attachments downloaded nonempty, with identical SHA-256 across forges.
  ZIP integrity, signatures, DMG integrity/layout and binary identity passed.
  Actual App/backend identity is **1.0.4-beta.28 / build 36**, independent of
  local source build 110. See `publication-verification.json` in the evidence directory.

GitHub SSH port 22 closed connections and the local HTTPS credential could not
write (403); pushing via the official SSH 443 endpoint with the existing SSH
identity completed synchronization. No global Git/SSH configuration changed.
No live installation, restart, DHCP transition or gateway path acceptance.

## LAN resource profiling — 2026-10-13

See [CPU and memory evidence](performance-2026-10-13.md) for reproducible
benchmarks, opt-in UDP stress, paced LAN-rate resource samples and read-only
observation of the installed App/backend. The isolated 30-second full cached
UDP run completed 3,191,767 requests with no failures or server drops.

Implemented optimizations skip disabled history formatting, format history
answers once while preserving independent public slices, use cumulative-only
cache counters for anomaly sampling, avoid client-list snapshots for unrelated
views, and skip redundant native Chinese translation work / regex compilation.
Focused history and cumulative-counter checks, the four-package Go race run
(`dnsserver`, `notifications`, `app`, `tests/integration`), Go vet/build,
`make native-test` and complete production Swift type checking passed. Existing
WebKit concurrency warnings remain. Native translation and allocation benchmarks
show component savings; they do not establish a reduction in the installed
App's overall CPU. That earlier profiling session did not perform a release, installation or service restart.

## DNS monitoring addition — 2026-10-12

The management UI now includes **大屏监控 / Monitoring** for the selected backend.
It uses completed-request snapshots, with one ingress/path, decision and policy
path per request. Ribbon widths and percentages share a common denominator;
policy DNS overrides, cache hits and fallback paths remain distinct from policy
decisions. See [monitoring semantics](monitoring.md).

Executed for this change:

- `npm --prefix web run build`: management UI and offline native notes built.
- `npm --prefix web test`: all 25 files / 116 tests passed before the final
  focused adjustments; the final `npm --prefix web test -- src/monitoring.test.jsx
  src/full-i18n.test.jsx` passed all 13 tests, including two added regressions for
  stale/gapped samples and exiting large-screen mode after data loss.
- `env GOCACHE=/tmp/routepilot-go-build-monitoring go test -race
  ./internal/dnsserver ./internal/app`: both packages passed. Coverage includes
  concurrent request conservation, bounded policy detail, UNKNOWN, non-address
  queries, local records, refusal/empty answers, Fake-IP gating, policy DNS
  fallback and an actual resolver response cache.
- Final `env GOCACHE=/tmp/routepilot-go-build-monitoring go test -race
  ./internal/app -run TestDNSMonitoring`: passed after strengthening the test to
  force an actual DNS Runtime rebuild. It verifies shared cumulative statistics,
  shutdown/disk restart, the optional status field and metrics.
- `env GOCACHE=/tmp/routepilot-go-build-monitoring go build ./...`: passed with
  the final embedded frontend resources.
- `node scripts/ui_forms_check.cjs`: 17 pages / 13 settings categories passed
  at 1440, 1120 and 390 px with no page errors or document overflow. Monitoring
  additionally passed 3840 × 2160 light/dark screenshots, hidden navigation in
  large-screen mode and Escape restoration. Screenshots under
  `artifacts/ui-forms/monitoring-*.png` use synthetic API fixtures.

Sandbox execution initially blocked localhost UDP test listeners and Chromium
process communication; the checks above passed after granting execution access.
The existing lockfile dependencies were restored to run the frontend checks;
no dependency or lockfile change was introduced.

This verifies implementation, fixture UI layout and local test persistence.
It does not establish live Surge/gateway traffic commissioning, installed native
WKWebView behavior, or package/install/update acceptance. No release, deployment,
service restart or user configuration change was performed during the implementation checks.

## v1.0.4-beta.1 packaging checks — 2026-10-12

- Source identity: `1.0.4-beta.1`, build `83`. CI sets the package build from
  its Actions run number; downloaded release identities are checked separately.
- `npm --prefix web test`: all 25 files / 118 tests passed. The multi-page
  localization test exceeded its default five-second limit while packaging
  competed for CPU; its scoped timeout is now 15 seconds, with assertions intact.
- `env GOCACHE=/tmp/routepilot-go-build-monitoring make package package-check`:
  passed App/backend version and resource checks, ZIP/App equality, checksums,
  ad-hoc signatures, DMG integrity, installer layout and vector background.
- `env GOCACHE=/tmp/routepilot-go-build-monitoring make native-test`: passed
  isolated installer, localization, core and real WKWebView UI checks.
- Previous canonical artifacts and binaries were preserved under
  `/tmp/routepilot-pre-v1.0.4-beta.1` before packaging. Local checks did not install
  or restart the user's deployed application or network service.

Beta tags perform packaging, while the corresponding main push must pass the
GitHub regression gate. Publication requires both release workflows and the
three downloadable, checksum-verified assets; package integrity does not prove
live upgrade or network commissioning.

This document separates verified implementation from site commissioning.
The IPv4 Phase 2 prerequisite passed before SQLite/UI/BGP/state implementation
continued. There are no simulated BGP or UI paths in the normal executable.
Production DNS, CLI, PF, SQLite and GoBGP use real implementations; mocks are
restricted to automated tests and the independently described acceptance scope.

## Executed checks

| Check | Result and scope |
|---|---|
| `make verify` | Frontend tests, all Go race tests, `go vet`, embedded-UI macOS build pass |
| Docker offline suite | 14 Linux test executables, `--network=none`, unprivileged container user; all pass |
| Native GoBGP exchange | Two actual in-process speakers on local high TCP ports: session establishment, IPv4 route/next-hop advertisement, explicit withdrawal, imported transit rejection pass |
| BGP session loss | Actual IPv4 Fake-IP/interception and IPv6 Fake-IP NLRI received; shutting down the originating speaker removes all three peer paths |
| Application runtime | Actual Darwin pinned loopback UDP DNS, native GoBGP, local test CLI/Fake-IP DNS: 100 unique DNS answers leave the fixed manual route set unchanged; removed CLI causes independent real-DNS fallback without withdrawing manual None |
| Actual transparent network | Docker macvlan + real macOS PF NATLOOK, UDP/TCP destination 8.8.8.8:53, preserved client/destination, physical gateway egress and reverse-translated replies pass |
| Actual re-entry defense | Raw UDP/TCP self-source injection at Docker ingress increases PF drop counters; no extra Controller query/upstream exchange |
| Browser acceptance | Embedded production UI: 15 pages; BGP/Static/Hybrid switching, disabled interception and local DNS entry create/edit/delete through actual API; masked editable credentials survive polling; zero page errors |
| Release assets | Native status-bar APP, macOS arm64 executables, embedded assets, SHA256 manifest, APP ZIP and CLI archive; shell syntax and launchd plist lint pass |
| Evidence parser | 3 Python parser regression tests pass |

Network evidence: [phase-2-docker-evidence.md](phase-2-docker-evidence.md).
The raw packet/journal files are private temporary artifacts; the dated document
retains the correlated transaction IDs, addresses and observation boundaries.
UI screenshots are generated under `artifacts/ui/` by `scripts/ui_acceptance.cjs`.
Final build/test logs are stored under `artifacts/` in this workspace.

## Fifteen architectural invariants

| # | Invariant | Automated coverage / actual evidence |
|---|---|---|
| 1 | Confident DIRECT only | `classifier.TestConservativeDecisions`, `integration.TestClassificationPaths`; unsafe metadata DIRECT is UNKNOWN |
| 2 | Uncertainty stays with Surge | Same cases plus errors/deadlines; unknown A/AAAA never become real-DNS bypass unless explicit health fallback/manual mode |
| 3 | Surge owns Fake-IP | Dispatcher consumes adapter answers only; `TestClassificationPaths`, `TestIPv6SurgeFakeAnswerTTLAndDNSSEC`; fake range validation, no allocator |
| 4 | Route before Fake-IP | `TestServiceBindingAndRouteGates`, `state.TestRouteBeforeFakeGraceAndRecovery`, selected routing abstraction readiness: GoBGP peer/Adj-RIB-Out or explicit external static declaration; never automatic static installation |
| 5 | Stop issuance before withdrawal | In-flight gate invalidation, observed TTL/grace, `TestWithdrawalWaitsForResponseWrite`; actual socket writes hold the barrier |
| 6 | Manual routes independent by default | API CRUD default None; manager lifecycle and actual runtime Surge-failure test |
| 7 | Global hard route limit | Pre-persistence rejection/CRITICAL event, rejected-update atomicity, `bgp.TestBindingsHardLimitAndNoDNSCoupling`; runtime add limit |
| 8 | Surge remains policy authority | Raw CLI argument/explanation tests, active inventory context audit; overrides are explicit UI settings |
| 9 | Transparent UDP/TCP non-local DNS | `TestDirectAndTransparentSockets` plus actual Docker/PF two-transport proof |
| 10 | Preserve query context and resolver | Original destination/client/transport tests, resolver cache isolation and correlated journal/wire packets |
| 11 | No upstream forwarding loop | `TestSelfOriginatedLoopRejected`, pinned source/ports and PF rule tests; actual self-source PF drops; real answers/hints in Fake-IP ranges rejected |
| 12 | Distinct route health lifecycles | `bgp.TestBindingsHardLimitAndNoDNSCoupling`: fake=Surge, interception=Controller, manual=None by default |
| 13 | Surge failure retains interception | State real-DNS fallback + manager leaves interception/None; actual app runtime fallback validated separately |
| 14 | BGP loss restores original path; static limitations explicit | Actual local BGP session-loss test removes interception/Fake-IP peer paths; full gateway FIB/client fallback remains site commissioning; static routes require external removal/failover |
| 15 | No DNS-to-BGP route explosion | Actual application processes unique DNS answers while native GoBGP route count/prefix stay fixed; static desired routes depend only on configured prefixes, never DNS history/cache |

Other regression coverage includes LRU/generation invalidation/singleflight,
per-client isolation, UDP truncation→TCP, DoT/DoH certificate validation, negative
TTL/cache flags, Fake-IP TTL and AD stripping, HTTPS/SVCB NODATA/hint guards,
SQLite encryption/transactions/retention, sessions/CSRF/rebinding and independent
health hysteresis/mode/shutdown/recovery behavior, including configured probe domains without a trailing DNS dot.

## Cached-DNS performance

The load test sends 20,000 queries through real UDP sockets, using 16 concurrent
clients, cached classification and cached real-DNS answers. The test asserts
exactly one upstream exchange and one external rule evaluation. Its local
fixtures do not measure the latency of Surge CLI misses or external resolvers.

| Run | QPS | P50 | P99 |
|---|---:|---:|---:|
| Native macOS, non-race run at 10:23 | 105,399 | 0.142 ms | 0.379 ms |
| Docker offline run at 10:42 | 97,769 | 0.061 ms | 3.271 ms |
| Prior Docker regression | 104,181 | 0.053 ms | 3.163 ms |
| APP/Hosts Docker regression | 116,359 | 0.049 ms | 3.040 ms |
| Routing abstraction final Docker regression | 105,586 | 0.051 ms | 2.993 ms |

These samples meet the specification's cached-path targets of ≥10,000 QPS,
P50 <1 ms and P99 <5 ms. Race instrumentation is for correctness, not capacity.
Run the same load test on the deployed machine to establish its own sustained
capacity; results above are measured samples rather than a universal guarantee.

## Deployment boundaries (automated and isolated evidence, 2026-10-08)

- At the time of this isolated preview, BGP was disabled and no live router
  configuration or announcement was made. This is a dated test boundary, not
  the current state of the deployed local instance; see the field-validation
  update below.
- IPv6 NLRI, DNS answers, listener/rule construction and Darwin NATLOOK layout
  have automated coverage. Actual IPv6 PF ingress and independent IPv6 gateway
  WAN bypass are unverified, so no IPv6 interception is enabled by default.
- Readiness uses ESTABLISHED plus Adj-RIB-Out and stabilization, not a fictitious
  router installation acknowledgement. Confirm next hops and gateway RIB/FIB,
  then test real Surge loss and Controller/Mac loss with Docker packet sources.
- The production PF lease loader/ownership guard and launchd installer have not
  been installed as a system daemon in this development environment. The actual
  PF network proof uses the independently documented diagnostic harness.
- Gateway bypass failure was simulated by injecting self-sourced ingress
  packets; the user-owned gateway policy was not removed during testing.
- The user configured a temporary gateway route for 8.8.8.8/32. It was not changed
  by Controller; the operator should remove it when the isolated test is done.
- On the inspected Surge inventory, unexpanded rule sets and dynamic groups
  conservatively prevent certain DIRECT. CLI polling has bounded consistency,
  not an atomic revision handshake with external Surge profile changes.
- macOS BGP active-connection TTL tuning reports an unsupported-platform warning;
  normal eBGP/iBGP exchanges pass. No unverified MD5/GTSM feature is claimed.

The checks above describe the automated and isolated acceptance scope as of
2026-10-08. Subsequent user-reported field validation is recorded separately
below; unverified details are not inferred from these earlier checks.

## Subsequent field validation (2026-10-10)

The user confirms that BGP is enabled on the local RoutePilot instance and has
been exercised in actual use. The user also confirms repeated real-world
validation of both local App upgrades and remote backend upgrades. These reports
supersede the earlier statements that BGP was disabled in the local preview and
that local/remote upgrades had only been tested with mocks.

Detailed run-by-run logs, tested version pairs, route matrices, and upgrade
failure-injection cases were not added to this repository in this update.
Accordingly, this record does not claim an independently audited external
gateway RIB/FIB matrix, every BGP failure mode, or every upgrade rollback case.
IPv6 transparent PF/WAN validation remains outstanding and IPv6 interception
remains disabled by default.

## Remaining field-validation status

For the v1.0.0 release, IPv6 transparent DNS is explicitly marked **unverified**
and remains disabled by default. IPv6 support in configuration, route families,
and automated tests does not imply validation of the real PF ingress or gateway
WAN/return path.

| Area | Current status | Evidence boundary |
|---|---|---|
| IPv4 transparent DNS | Passed for the documented Docker macvlan/macOS PF path on 2026-10-08 | See [packet evidence](phase-2-docker-evidence.md); gateway policy was not disabled, and production cutover is deployment-specific |
| IPv6 transparent DNS | Not yet validated on a real PF/WAN path; disabled by default | Automated IPv6 construction/tests do not prove live ingress, routing or return path |
| BGP | Enabled and used on the local instance, confirmed by the user on 2026-10-10 | Independent external gateway RIB/FIB matrix and detailed run logs are not archived here |
| Local App upgrade | Repeated actual upgrades confirmed by the user on 2026-10-10 | Exact source/target versions and scenario-by-scenario logs are not archived here |
| Remote backend upgrade | Repeated actual upgrades confirmed by the user on 2026-10-10 | Exact backend versions and failure-injection coverage are not archived here |
| Developer ID signing/notarization | Not provided; release packages use ad-hoc signing | Affects Gatekeeper trust and first-run experience, not the reported upgrade test outcome |

## macOS APP and local DNS additions (2026-10-08)

`artifacts/RoutePilot.app` is a native Swift/AppKit accessory application with a
real NSStatusItem, an embedded WKWebView and a bundled Go Controller. The isolated
APP smoke run (`artifacts/app-smoke.json`) confirms menu creation, real API mode
change/restoration, cache flush, local DNS CRUD, effective Surge Host import,
rendered management UI, continued background operation after the window closes,
and graceful owned-child shutdown. Administrator installation and login-item
registration are exposed through native controls but were not enabled on this
Mac during acceptance. The APP uses ad-hoc signing, without Developer ID notarization.

Static `[Host]` import was tested against the active Surge instance: 4 static
mappings imported, 3 unsupported entries skipped with generic reasons and line
numbers. Other profile sections, raw lines, script content and remote URLs are
excluded from the API and database. Manual entries override imported mappings;
Surge entries preserve first-match order. Unsupported earlier mappings
conservatively suppress potentially shadowed later static entries. Reimport
replaces only the Surge snapshot. Fake-IP mappings are rejected.

Native and Docker regressions cover manual/imported precedence, glob/apex
semantics, A/AAAA families, HTTPS/SVCB and missing-family NODATA, immutable
configuration copies, live local-DNS updates without BGP changes, idempotent
import and SQLite schema v1→v2 migration. An APP startup check also covers a
configuration applied before the first management listener and explicit port
release during shutdown. Browser acceptance now exercises 15 pages and local
DNS create/edit/delete, including multiline IPv4/IPv6 and wildcard selection.

The active Surge configuration still hijacks all DNS via `hijack-dns = *:53`.
No Surge or gateway configuration was changed by this implementation. Production
cutover must remove full DNS hijacking and route client resolver traffic through
the normal gateway; retain Surge's original upstream DNS and internal Fake-IP DNS.


## Routing abstraction additions (2026-10-08)

All five frontend tests, full Go race suite, vet and embedded build pass. The
final Docker offline suite runs 14 package executables (74 tests), including
common routing/state, both Hybrid directions, static installation truthfulness,
scoped TCP-probe evidence and explicit static-interception opt-in. The actual
Darwin application test returns Real-IP and Fake-IP with no BGP speaker,
preserves its DNS listener during routing-only edits, and enters DIRECT_FALLBACK
on CLI loss while retaining an externally managed, installation-unknown static
route declaration. Tests use local fixtures/operator declarations and do not
prove an external gateway installed a static route.

The native APP smoke additionally switches Static/Hybrid/BGP through the real
API, checks `installed=null`/`verified=false` on static rows and restores its
original mode. Its live Surge import at 14:20 saw 44 Host items: 4 supported
static mappings imported; 40 skipped, including mappings potentially shadowed
by unsupported earlier remote rule sets. This supersedes the earlier 7-item
inventory observation; no remote URLs were downloaded or script mappings run.
The optional TCP probe is unit-tested with controlled transports; no external
static-route or real WAN connectivity verification is claimed.

## v1.0.1 release preparation — 2026-10-10

The local `make verify-release` and `make fuzz` gates passed for v1.0.1:

- 23 frontend test files, 108 tests; all Go packages under `-race -count=1`,
  `go vet`, and embedded frontend/controller/CLI builds.
- Isolated native installer, localization, core and actual AppKit/WebKit UI
  checks; no production service upgrade or credential change performed.
- Fixture-backed browser checks for 16 pages and 13 settings categories at
  1440, 1120 and 390 pixels, with no page errors or viewport overflow.
- DMG/ZIP contents, release identity, bundled resources, SHA256 manifests,
  ad-hoc signatures and DMG integrity checks.
- Release-version fuzzing: 229,528 executions over five seconds, passed.

These checks cover update configuration, persistent scheduling/skips, release
channels, unsaved preferences and remote version notices. They do not extend
the existing IPv6 ingress or gateway commissioning evidence. Release publication
is performed separately by the stable Gitea workflow.


The v1.0.1 tag was **not released**: CI fuzzing found an outdated test invariant
that incorrectly treated an accepted numbered Beta tag as a stable-only tag.
The minimized `0.0.0-beta.1` case was reproduced locally, then added alongside
`v1.0.1-beta.7` and an invalid leading-zero Beta seed. The assertion now checks
canonical numbered Beta suffixes independently. Production version validation
was unchanged. The existing tag remains immutable; the next candidate is v1.0.2.

### v1.0.2 candidate verification

After correcting the fuzz assertion, `make verify-release` passed again for
v1.0.2 with the same 108 frontend tests, full Go race/vet, native UI, responsive
browser checks and DMG/ZIP package validation. `make fuzz` also passed with all
regression seeds and 203,435 randomized executions over five seconds.

## v1.0.3 candidate test coverage audit — 2026-10-11

The feature audit added direct regressions for the user-visible and security
boundaries introduced since v1.0.2:

- GitHub token controls appear only for the GitHub source; save and clear actions
  use the native bridge. Native request construction checks GitHub bearer auth,
  no token on Gitea, source URLs, and invalid source rejection.
- Scoped configuration backup/restore checks selected sections, preservation of
  omitted sections and existing secrets, and invalid section selections.
- Trusted-proxy tests check the configured client-IP header, trusted-chain
  traversal, malformed values, forwarded allowlist identity, and rejection of
  bypass through an unselected header.
- BGP detail rendering checks deterministic remote capability ordering while
  retaining local order. GoBGP detail serialization checks both credential
  fields are redacted without mutating the live snapshot.
- Optimistic cache tests check request-level hit/comparison/mismatch accounting,
  maintenance reads, manual reset, and persistence across service restart.
  Response fingerprints ignore wire ID, TTL, record ordering, and domain case,
  while preserving opaque TXT changes.
- Automation preference tests cover legacy `skillTarget` migration and valid,
  duplicate, and unknown agent targets.

Remaining automated-coverage gaps: native automation's actual filesystem
conflict, backup/restore, and rollback transaction is not exercised end-to-end;
GitHub token Keychain save/read/delete is tested through the bridge/request
contract, not against a disposable Keychain; the new BGP UI test focuses on
capability ordering rather than every peer-detail field. These checks do not
replace live gateway commissioning.

Final local release gates passed after these changes:

- `make verify-release`: 24 frontend files / 111 tests, all Go packages with
  `-race -count=1`, `go vet ./...`, native core/UI, 16-page responsive UI,
  and DMG/ZIP version, resource, signature, checksum, and layout validation.
- `make fuzz`: 230,438 version-parser executions over five seconds, passed.

## v1.0.4-beta.2 monitoring verification — 2026-10-12

The monitoring dashboard now persists five-second observations as minute
history retained for seven days, and filters live and historical DNS flows,
policy hits and response codes by client source IP. Source shares use the
selected scope's request counts. Legacy totals without source information,
unidentified clients and detail-capacity overflow remain explicitly separate.

Local checks passed for this candidate:

- `npm --prefix web test`: 26 files / 122 tests, including source switching,
  late-response isolation, invalid history ranges and visible query failures.
- `go test -race ./internal/database ./internal/dnsserver ./internal/api
  ./internal/app ./internal/remote`: persistence transactions and checkpoints,
  disk reopen, retention, sampling gaps, source normalization and caps,
  runtime rebuild/restart, history API validation and remote GET allowlisting.
- `go vet` for the same five packages, and `npm --prefix web run build`.
- `node scripts/ui_forms_check.cjs`: 17 pages / 13 settings categories at
  1440, 1120 and 390 pixels; source-specific history, 4K light/dark monitoring,
  fullscreen focus and Escape checks passed.
- `make package package-check`: complete App, embedded frontend and backend,
  source version 1.0.4-beta.2 / build 84, DMG/ZIP resources, signatures,
  layout, integrity and SHA256 manifest checks passed.

History starts with observations collected by the new backend; earlier
versions' missing samples and source attribution are not reconstructed.
Restart boundaries and sampling gaps do not claim continuous request rates.
These fixture and isolated-runtime checks do not establish production
installation, live LAN traffic correctness or capacity at the source limit.
GitHub regression and both publication workflows are separate release gates.

## v1.0.4-beta.3 source flow layout — 2026-10-12

Source IPs now form the ingress nodes of the DNS flow diagram. The source
share table sits inside the flow card below the diagram; the standalone
source statistics card was removed. Both live and historical lightweight
source lists include actual flow counts without per-source policy details.
Smaller diagram nodes are grouped using observed destinations, never inferred
by multiplying source and destination shares. The share table uses received
requests, while ribbons use completed requests in the selected scope.

Local verification passed: 26 frontend files / 123 tests; race tests and vet
for database, dnsserver, app and api; frontend build; 17-page browser checks
at 1440/1120/390 and 4K light/dark; complete App packaging and package checks
for source version 1.0.4-beta.3 / build 85. Added checks cover source-flow
conservation, small-source aggregation, missing-detail fallback, embedded
table placement and persisted history flow counts. No production service was
installed or restarted during these checks.

## v1.0.4-beta.4 source presentation — 2026-10-12

The source share table is now an independent card below the DNS flow and
policy cards. Pre-collection source entries are hidden from live and historical
selectors, diagrams and source tables, while stored totals and the request
denominators remain unchanged. Visible source flows continue to use their
observed destinations; omitted old requests are not assigned to other clients.

Local checks passed: 26 frontend files / 124 tests, production frontend build,
17-page browser checks at 1440/1120/390 and 4K light/dark, and full App package
validation for source version 1.0.4-beta.4 / build 86. The history regression
checks hidden entries, retained totals and correct source shares in both modes;
browser checks assert the share card is outside the flow card. Backend
statistics and persistence behavior were not changed. No installed service was
upgraded or restarted.

## v1.0.4-beta.5 update and monitoring fixes — 2026-10-12

- Reproduced the reported history-save failure with a source-less persisted
  snapshot upgraded into a legacy source. New requests mutated shared decision
  and response-code maps without changing that source's completed count. The
  restore path now owns independent maps; the same test passes with three
  subsequent saves and a source-filtered history query.
- Source-flow and source-share denominators exclude hidden pre-collection
  requests. The live/history regression verifies 40% + 60% visible source
  shares while stored cumulative totals remain intact.
- Resolver traces distinguish ordinary cache hits from actual optimistic stale
  hits. Dispatcher tests cover both destinations and resolver tests verify a
  valid cache hit remains ordinary even when optimistic responses are enabled.
- Native checks read metadata without pre-downloading the package. Installation
  remains owned by the helper. Go download tests cover known/unknown sizes,
  status-write failures, transferred bytes and duplicate-install rejection;
  frontend tests cover native and remote progress and disabled duplicate actions.
  Native status decoding checks both progress percentages and old-helper fallback.

Local gates passed: 27 frontend files / 127 tests; race tests and vet for
softwareupdate, database, dnsserver, resolver, app and api; production frontend
build; isolated native core/UI checks; 17-page browser checks at 1440/1120/390
and 4K light/dark; full App packaging and package validation for source version
1.0.4-beta.5 / build 87. These checks did not start a privileged update helper,
replace an installed App, restart production services or measure download
progress against a live installed environment. Old history does not contain
enough information to retrospectively split optimistic cache hits.

## v1.0.4-beta.6 client configuration — 2026-10-12

Client settings support concrete IPv4/IPv6 addresses, remark names and
Automatic/DIRECT/SURGE/REJECT policies on the connected backend. Matching
normalizes IPv4-mapped addresses and runs before ordinary DNS policy branches,
while retaining query validation, ANY refusal, loop prevention and Fake-IP
health/issuance gates. Names affect display only; monitoring still queries IPs.

Local gates passed: 28 frontend files / 132 tests; race tests and vet for config,
api, app, dnsserver, database and remote; frontend build; 17-page responsive
browser checks plus client editor checks at 1440/1120/390 and 4K monitoring;
full App/package verification for source version 1.0.4-beta.6 / build 88.
Regressions cover configuration cloning and invalid/duplicate addresses,
dedicated CRUD and durable acknowledgement, old-settings-form preservation,
IPv4-mapped/IPv6 matches, A/TXT policies, non-target-client isolation, ANY and
unready-Surge refusal, runtime activation and actual disk reopen. UI tests cover
remarks, Automatic restoration, failed saves, unsupported backends and names
with unchanged source-IP request identities. These checks do not install the
release or commission actual LAN/NAT/IPv6 client traffic.

The client-to-policy chart shares the DNS flow chart layout and is displayed
beside it on wide screens. Bounded backend aggregation uses observed source and
policy pairs, excludes pre-collection legacy sources, preserves every count
when grouping smaller nodes, and never multiplies marginal percentages.
Additional race tests cover pair grouping and global/source-filtered persisted
history. Frontend regressions assert real pair identities and shared history
scope; browser checks include both charts in light/dark 4K and responsive views.

## v1.0.4-beta.7 update settings layout — 2026-10-12

Removed the requested update-target explanation and moved the manual check
button beside Save settings. The existing updater remains mounted: only its
check button is rendered into the settings action row. Native actions, direct
browser requests and remote relay requests keep their existing ownership,
progress and confirmation behavior. Existing functional tests assert the shared
action row, removed text and correct target for all three connection modes.
Responsive browser checks cover the action row at 1440/1120/390; no installed
App or production service is upgraded as part of this verification.

Local beta.7 gates passed: 28 frontend files / 132 tests, production frontend
build, 17-page responsive browser checks and full App/package validation for
source version 1.0.4-beta.7 / build 89. Backend DNS code is unchanged from the
validated beta.6 release.

## v1.0.4-beta.8 small-share flow labels — 2026-10-12

Flow node gaps now reserve both name and count/share lines even when adjacent
nodes carry a fraction of a percent. Node and band sizes remain proportional;
no artificial minimum thickness is added. A layout regression checks tiny
clients and eleven policies, and browser checks compare actual SVG text bounds
for the reported crowded client/policy pattern at 1440 and 3840 pixels.
Software update settings remove the unused divider while retaining the check
button beside Save; responsive checks assert its computed border is absent.

Local beta.8 gates passed: 28 frontend files / 133 tests, production frontend
build, 17-page responsive UI checks plus actual SVG label-boundary checks at
1440/3840, and full App/package validation for 1.0.4-beta.8 / build 90.

Published beta.8: commit f1d3eefb43b6528dcb56ed927e36addc33a5b076;
GitHub regression #27 (38214203645), release #16 (38214203939), and Gitea
release #110 (2081) succeeded. Gitea mirrored GitHub assets without fallback.
Both prerelease pages contain exactly DMG, ZIP and SHA256SUMS; downloaded
checksums match, App/backend identity is 1.0.4-beta.8 / build 16, and archive,
resource, code signature and DMG checks passed. Evidence is retained under
artifacts/releases/v1.0.4-beta.8/. No live installation was performed.

## v1.0.4-beta.9 client-list editing and persistence — 2026-10-12

Client settings are edited from the client list with a fixed source IP; the
separate configuration card is removed. Configured clients absent from the
current bounded statistics list remain editable with unknown counts, rather
than fabricated zeros. Clear settings preserves the statistics identity.
Existing frontend regressions cover POST/PUT/DELETE from list rows, failed
saves, old-backend fallback, and configured clients without activity.

Client statistics now share the App lifetime across runtime rebuilds, checkpoint
to SQLite every five seconds when changed, and save after DNS workers drain on
normal shutdown. Disk reopen tests cover periodic writes, final unsampled
requests, IPv4-mapped identities, IPv6, counts and original last activity.
Restore validates before mutation; bounded list eviction and aggregate counts
survive restore. An injected SQLite write error is visible in the client API
and metrics; retry clears the error without losing counts. Older unsaved client
statistics are not reconstructed; abrupt exit can lose the last unsaved window.

Local beta.9 gates passed: 28 frontend files / 133 tests; related Go race
tests and vet; final client-restore race tests including scoped IPv6; production
frontend build; 17-page responsive UI checks; full App/package validation for
source version 1.0.4-beta.9 / build 91. No live upgrade was performed.

## v1.0.4-beta.10 update action row — 2026-10-12

The persistent updater renders both check and install/upgrade actions into the
settings footer. Existing tests now assert the shared row after an update
becomes available for native, browser and remote connections, including native
install dispatch and remote relay ownership. Responsive checks compare actual
button positions at 1440/1120/390. Update progress and confirmation use the
existing components; no real installation is triggered by these tests.

Published beta.9: GitHub regression #29 (38216768389), release #17
(38216768399), Gitea mirror #111 (2082) succeeded. Both providers retain
exactly three matching assets; downloaded App/backend identity is
1.0.4-beta.9 / build 17. Full integrity evidence is under
artifacts/releases/v1.0.4-beta.9/.

Local beta.10 gates passed: 28 frontend files / 133 tests, production frontend
build, 17-page responsive checks including same-row check/install buttons at
1440/1120/390, and full App/package validation for 1.0.4-beta.10 / build 92.

## v1.0.4-beta.11 three-stage flows and navigation — 2026-10-12

Source/policy/final-policy triples are aggregated from observed per-source policy
records, bounded independently per node column and kept intact in live/history
responses. Each segment uses the same request scale and middle-node geometry.
Old policy-only backends show an explicit two-stage fallback. Names alone toggle
to IPs without changing statistics filters, and full wrapped labels reserve
extra vertical space. Request source shares use ranked bars, with the existing
legacy exclusion and denominator preserved.

Sidebar expanded/icon/hidden states persist in the management UI. Hidden
navigation overlays the page on edge hover or keyboard access and can be
pinned back. Tests retain DNS mode identity and menu accessibility; browser
checks cover actual icon width, hidden page margin, preference reload, and
edge-to-page mouse leave. Mobile navigation remains a separate drawer.

Local beta.11 gates passed: 29 frontend files / 138 tests, related Go race
tests and vet, bounded 1331-triple aggregation regression, production frontend
build, 17-page responsive checks plus full name/IP toggles and sidebar hover
state checks, and full App/package validation for 1.0.4-beta.11 / build 93.

Published tag `v1.0.4-beta.11` points to
`ff9eec65b4f1620788d59b5977caec4290d64927`. GitHub regression #31
(`38222205627`) and release #19 (`38222205910`) passed; Gitea release #113
(`2084`) mirrored the GitHub artifacts successfully, with fallback skipped.
Both pages are non-draft prereleases with exactly DMG, ZIP and SHA256SUMS.
Downloaded attachments are nonempty, match metadata sizes and checksums,
and have identical hashes across providers. Actual App and bundled backend
are `1.0.4-beta.11 / build 19`; ZIP resources, signatures and DMG integrity
passed. Evidence: `artifacts/releases/v1.0.4-beta.11/publication-verification.json`.
No deployed installation, live upgrade or real LAN client-flow commissioning
was performed.

## v1.0.4-beta.12 display preferences and ribbon alignment — 2026-10-12

Settings → Monitoring controls six individual summary cards and seven chart /
ranking cards. App preferences use the acknowledged native bridge and
UserDefaults because the management WKWebView is nonpersistent; browsers use
site storage. Old App bridges report unsupported persistence. Failed preference
reads are visible and use protected labels; failed writes do not report success.

Protection replaces IPv4/IPv6 client addresses, remarks, matched/final policy
names and backend identifiers in visible text, SVG titles, detail tables,
accessibility labels and source selector values. Anonymous source selection
still sends the original source identity to the backend. Live/history tests
preserve counts and aliases; enabling protection clears an existing raw policy
filter. This is display protection, not API access control or data anonymization
in the persistent database.

Three-stage layout now deducts node gaps using the maximum of all three
columns, so both segments share the same scale and middle-node geometry even
when the final column has more nodes. Browser checks inspect every actual SVG
ribbon's thickness/count ratio and both ends' coverage of each middle node.

Local gates passed: 30 frontend files / 146 tests; production frontend build;
`make native-test`, including preference migration/round-trip/validation and
real WKWebView checks; `node scripts/ui_forms_check.cjs` with 17 pages and
14 settings categories at 1440/1120/390 plus 4K flow checks; and
`make package package-check` for 1.0.4-beta.12 / source build 94. Privacy browser
checks reopen the page, retain hidden cards, select an anonymous source, and
inspect the rendered markup for raw identifiers. No actual environment was
installed or restarted.

Beta.12 publication verified: tag points to
`6c6724b6e0f111f21c48a915163ba63e9af1c6a8`; GitHub regression #33
(`38225857313`) and release #20 (`38225857525`) passed, and Gitea #114
(`2085`) mirrored GitHub successfully with fallback skipped. Both releases are
non-draft prereleases with exactly three downloadable nonempty attachments.
Downloaded hashes match across providers; actual App and backend are
`1.0.4-beta.12 / build 20`, with signatures, ZIP resources and DMG integrity
verified. Evidence: `artifacts/releases/v1.0.4-beta.12/publication-verification.json`.

## v1.0.4-beta.13 final-policy chart scope — 2026-10-12

Client policy flows filter complete triples whose final kind is `unnamed`
before building any column, ribbon, total or detail row. All percentages use
the remaining completed requests; cumulative metrics, DNS flows and standalone
policy rankings are unchanged. Recorded `rest` / capacity aggregates remain,
and old two-stage rows keep their explicit compatibility fallback. If every
request lacks a final policy, the card states that no recorded final-policy
requests are available.

Local checks passed: 30 frontend files / 147 tests, production build, full
17-page / 14-category browser checks at 1440/1120/390 and 4K, and
`make package package-check` for 1.0.4-beta.13 / source build 95. The crowded
browser fixture excludes 1492 unrecorded-final requests from 4560, confirms
3068 in the exact detail rows, verifies displayed shares sum to 100% within
rounding, and repeats actual ribbon-scale / middle-node alignment checks.

## v1.0.4-beta.14 capacity-grouped unrecorded kinds — 2026-10-12

Missing-policy and capacity-overflow observations stay distinct types when
flow details are grouped under pressure, so small unrecorded flows are never
folded into the remaining policies or their percentage denominators. The
client policy chart keeps excluding requests without a recorded final policy
from every column, ribbon, exact detail row and denominator.

Local checks passed: 30 frontend files / 147 tests, `go vet ./...` and race
tests for `internal/dnsserver`, and `make package package-check` for
1.0.4-beta.14 / source build 96.

Beta.14 publication verified: tag points to
`1e352790cfa1187747fe2b18155766a4a119f126`; GitHub regression (`38229480681`)
and release (`38229480688`) passed, and Gitea run `2087` mirrored GitHub with
the fallback build skipped. Both releases are non-draft prereleases with
exactly three downloadable nonempty attachments; downloaded hashes match
across providers and the actual App and bundled backend are
`1.0.4-beta.14 / build 22`.

## v1.0.4-beta.15 flow ribbon selection — 2026-10-12

Ribbons in the DNS request flow and client policy flow charts accept clicks
(Enter/Space on keyboard focus as well). Selecting one ribbon highlights it,
dims the rest and pins the flow's exact request count and share below the
chart; clicking again clears the selection and clicking another ribbon
replaces it. The selected flow is tracked by its stage/source/target identity,
so live refreshes update the pinned numbers instead of freezing stale values.
Sensitive-display protection keeps applying to the pinned labels.

Local checks passed: 30 frontend files / 149 tests including the new ribbon
selection cases, production build, and 17-page / 14-category browser checks at
1440/1120/390.

Beta.15 publication verified: tag points to
`63b1487191cc4a694f601177740fd1358ed977f9`; GitHub regression (`38232084051`)
and release (`38232084606`) passed, and Gitea run `2088` mirrored GitHub with
the fallback build skipped. Both releases are non-draft prereleases with
exactly three downloadable nonempty attachments; downloaded hashes match
across providers and the actual App and bundled backend are
`1.0.4-beta.15 / build 23`. Evidence:
`artifacts/releases/v1.0.4-beta.15/publication-verification.json`.

## v1.0.4-beta.16 DNS interception gateway confirmation — 2026-10-13

The static DNS gateway declaration moves from routing settings to the DNS
interception page. Clicking “Confirm gateway routes are configured” saves
immediately through the connected backend's existing settings API. An existing
declaration displays a gray, disabled “✓ Gateway routes confirmed” button;
backend invalidation after relevant configuration changes enables confirmation
again. Persistent static interception permission remains in routing settings.
The declaration does not install or verify external gateway routes.

Local checks passed: `npm --prefix web test` (31 files / 154 tests),
`make native-test ui-test` (isolated native checks and 17 pages / 14 settings
categories at 1440/1120/390), and
`env GOCACHE=/tmp/routepilot-go-build-beta16 make package package-check`
(1.0.4-beta.16 / source build 98). Confirmation cases cover connected remote
requests, preservation of freshly read settings, failure reporting, static
permission, changed backend mode, and confirmed/invalidated button states.
No actual installation, upgrade, backend restart, or gateway/LAN/IPv6 path
acceptance was performed. Configuration runtime application is unchanged.

## 2026-10-13：v1.0.4-beta.17 乐观缓存分项设置

- 实现：按 DIRECT / SURGE / REJECT 决策与实际 DNS 上游设置乐观缓存；总开关勾选后展开分项，取消后收起并保留配置。普通缓存保持原有行为，UNKNOWN 沿用短期普通缓存。
- 分项统计包含条目、命中、未命中、淘汰、待刷新、旧值命中、已核对、不一致及不一致率；同步页面、API 与 /metrics。核对按实际返回的旧决策 / 旧上游归属，跨配置重建和进程重启持久化，手动清理重置。
- 验证：隔离发布目录的 `env GOCACHE=/tmp/routepilot-go-build make verify-release` 通过：前端 32 文件 / 159 测试、Go 全包 race / vet、原生核心与真实 WKWebView、17 页 / 14 分类响应式检查、九项分组缓存检查、完整 App 打包与版本 / 资源 / 签名 / DMG / ZIP / SHA256 校验（源码 1.0.4-beta.17 / build 99）。`make fuzz` 通过。GitHub 回归 run `38353290349` 和 Release run `38353296930` 成功；Gitea run `2092` 的 Ubuntu runner 拉取镜像时 TLS 握手超时，回退任务生成制品，但整体工作流失败，该标签不记为完整核验发布。
- 未授权安装或升级现用环境，未验收真实 LAN / NAT / IPv6 流量。前序告警、性能、导航与偏好等未提交改动留在原工作区，不纳入此版本。
- 说明：[分项缓存说明](cache-policies.md)。

## 2026-10-13：Surge 乐观响应与本机通知菜单（beta.18）

- Surge 解析结果作为独立乐观响应来源；覆盖过期响应、刷新不一致、客户端隔离、健康 / 路由 generation 失效拒绝写回、Fake-IP 网段校验和 TTL 发行上限。可靠性计数参与状态 API、缓存 API、metrics 与持久化；Fake-IP 内容不跨运行实例恢复。
- 本机通知设置迁移到 App 状态栏菜单，保持本机偏好保存、通知权限请求和声音选项；移除管理卡片，后端通知表单仍可编辑。
- 定向测试及隔离目录的 `env GOCACHE=/tmp/routepilot-go-build make verify-release` 通过：前端 32 文件 / 160 测试、Go 全包 race / vet、原生核心与真实 WKWebView、17 页 / 14 分类响应式检查、九项分组缓存检查（含 Surge 开关保存及通知卡片移除）、完整 App 打包与版本 / 资源 / 签名 / DMG / ZIP / SHA256 校验（源码 1.0.4-beta.18 / build 100）。GitHub 回归 `38355720774` 与发布 `38355781983` 成功；Gitea `2093` 再次在 runner 拉取 Docker Hub 镜像时 TLS 握手超时，回退任务成功但整体工作流失败，不记为完整核验发布。两仓库 main 与注释标签均指向 `917a8919dc92b30728f12c1e5acc4ae63175f256`；发布页均为非草稿测试版、各有 DMG / ZIP / SHA256SUMS 三附件。下载哈希、DMG 完整性及 App / 后台版本已核对，GitHub 为 `1.0.4-beta.18 / build 26`、Gitea 回退为 `1.0.4-beta.18 / build 120`，两边分别校验且哈希不同。证据见 `artifacts/releases/v1.0.4-beta.18/publication-verification.json`；runner 缓存说明见 [Gitea runner 镜像缓存](gitea-runner-cache.md)。
- 不安装或升级现用环境，未在真实 Surge / 网关环境验收缓存或实际 macOS 通知授权。

## 2026-10-13：beta.19 发布重试

- 功能包含 beta.18 的 Surge 缓存与通知菜单，并补上旧远端缺少 Surge 能力时的局部降级提示。用户已修改 Gitea runner 设置；使用新的递增标签复核镜像缓存与双仓库发布，不修改已发布标签。本轮按用户授权纳入此前全部未提交的告警、低资源优化、页面细节与基准 / 文档；审查修复告警采样遗漏 Surge 响应池，并增加该池计数回归。
- Gitea 获取作业设为可恢复失败，三附件发布完成才输出成功；否则执行必须成功的回退作业。覆盖获取成功、获取失败且回退成功、两条均失败的结果语义。
- 完整候选的 `env GOCACHE=/tmp/routepilot-go-build make verify-release` 通过：前端 33 文件 / 165 测试、Go 全包 race / vet（含五种发布分支夹具）、原生核心与真实 WKWebView、17 页 / 14 分类响应式页面、九项缓存界面检查、完整包身份 / 资源 / 签名 / DMG / ZIP / SHA256 校验（源码 `1.0.4-beta.19 / build 101`）。`make fuzz` 通过。GitHub Release `38358819638` 成功，但回归 `38358861961` 在发布脚本夹具失败：macOS runner 没有 GNU `sha256sum`，因此该版本不能记为完整门禁通过。Gitea `2094` 整体成功，Ubuntu 镜像日志确认 `forcePull=false` / `Image exists? true`，随后 checkout 网络超时，macOS 回退发布成功。
- 仍未安装或升级现用环境，未进行真实 Surge / 网关或 macOS 通知授权验收。

## 2026-10-13：beta.20 流向图固定字号与纵向尺寸

- 两张图使用与实际像素尺寸一致的 SVG viewBox；ResizeObserver 仅调整横向几何。标签固定 14px、数值 12px，固定换行宽度，按相邻标签所需高度留间距；节点更多时自然扩展，不设图内纵向滚动。三列共享请求比例与中间节点坐标，卡片不随邻卡高度拉伸。
- 浏览器固定夹具覆盖 1440 / 3840 宽度：同一数据的图高、节点 y / height、文字 font-size / y / screenCTM 不变，仅宽度变化；十个短标签节点图高小于 600px，拥挤标签无重叠，长备注切换仍完整。新增长标签仅影响相邻间距的回归。
- 发布脚本夹具显式提供测试用 SHA256 适配命令，不要求 macOS runner 安装 GNU coreutils；以仅系统 PATH 运行五种获取 / 发布 / 回退分支通过。生产 Gitea 获取脚本保持 Ubuntu 原有命令。
- `env GOCACHE=/tmp/routepilot-go-build make verify-release` 通过：前端 33 文件 / 167 测试、Go 全包 race / vet、原生核心与 WKWebView、17 页 / 14 分类响应式检查、缓存界面九项检查、源码 `1.0.4-beta.20 / build 102` 完整打包与包核验。CI / 下载包结果在发布后记录；未安装或升级现用环境。

## 2026-10-13：并列流向图等高（纳入 beta.21）

- 两张图实际并列时，使用双方自然图高的最大值；较矮图只将新增空间分配到节点间距，单节点居中。保持标签 14px / 数值 12px 与请求比例尺，不改变节点和流带粗细。上下排列、单图、另一图无数据时恢复自然高度；读取自然图高而非展示图高，允许数据减少后收回空间。
- `npm --prefix web test`：33 文件 / 169 测试通过，包含间距扩展、单节点居中、请求粗细不变、三列中间节点与左右流带对齐回归。`npm --prefix web run build` 通过。
- `node scripts/ui_forms_check.cjs` 通过：17 页面 / 14 设置分类的 1440 / 1120 / 390 响应式检查；流向图另覆盖 1440 → 3840 → 2100 → 1440，验证并列等高、相同排列方式内纵向坐标固定、字号与 SVG screenCTM 不变、无纵向滚动与拥挤标签重叠。数据从十策略节点减少后共享图高能缩回；隐藏一张图后另一张恢复自然高度。截图与记录见 `artifacts/ui-forms/monitoring-equal-heights.png` 与 `verification.json`。
- 仅本次前端改动与生产构建验收；未提交、未发布、未安装或重启现用环境。

## 2026-10-13：域名占比与六维组合筛选（纳入 beta.21）

- 新增与来源占比一致的请求域名排行、比例条、计数和百分比，点击域名联动页面；两份排行使用紧凑行距。标签在所有筛选框前方，包括来源、处理去向、策略 / 策略组、最终策略、域名、响应码及历史时间输入。
- Dispatcher 按真实完成请求记录有界联合维度；所有新增条件取同一请求的交集，统一用于实时指标、两张流向图、各排行 / 占比及历史趋势。关联不从查询记录推算，最多 4096 条组合，溢出独立累计。原有来源条件保留流入 / 处理中口径；使用新增完成条件时显示「筛选完成请求」。域名及关联跨配置重建 / 重启保留，参与 SQLite 检查点、七天历史、详情 API 与域名 metrics。
- 旧历史不补造域名 / 响应码关联；整个范围缺少维度时返回明确的 422 不可用，混合范围只纳入已记录数据。历史错误不会清空仍然有效的实时结果。旧后端局部禁用新增选项；六个字段复用远程允许列表中的现有 GET 接口，Query 原样转发。保护模式对域名 / 策略 / 最终策略的选框值、行标题和悬停同样匿名；本机 App 新卡片偏好已同步。
- `npm --prefix web test`：34 文件 / 172 测试通过；`npm --prefix web run build` 与 `go build ./...` 通过。`env GOCACHE=/tmp/routepilot-go-build go test -race ./internal/dnsserver ./internal/database ./internal/api ./internal/app ./internal/remote` 与相同五包 `go vet` 通过，覆盖真实 Dispatcher 去向、联合条件交集 / 零结果、响应错误、规范化、容量溢出、检查点 / 历史趋势 / 重启、状态能力与域名 metrics。隔离 Swift 偏好 Codable 检查通过新增 domains 卡片保存校验。
- `node scripts/ui_forms_check.cjs`：17 页面 / 14 分类响应式检查通过，新增 1440 / 390 下筛选名称与选框水平对齐、来源 / 域名行步长不超过 32px、六项条件同时传入及历史统计一致检查。前轮并列等高 / 固定字号 / 纵向间距检查仍通过。截图及 JSON 位于 `artifacts/ui-forms/`；本轮原始命令日志备份在 `artifacts/monitoring-filters-2026-10-13/`。
- 保留前轮未提交的流向图等高工作；本轮未提交、未发布、未安装或重启现用环境。未做真实 LAN / Surge / NAT / IPv6 流量或长时资源占用验收。

## 2026-10-13：beta.21 直接推送发布

用户明确要求跳过相关检查、直接推送。本次不重新运行本地测试、编译、打包或下载包校验；上文记录的是实现阶段的既有证据，不能当作本次发布核验。将域名占比、六维筛选、紧凑排行与并列流向图等高纳入递增标签 `v1.0.4-beta.21`（源码 build 103），由现有双仓库发布流水线生成制品。未授权安装、升级或重启现用环境。

## 2026-10-15：beta.22 缓存开关、域名独立累计与会话恢复

- 决策缓存 / 响应缓存新增总开关与分项开关；普通缓存关闭时禁用对应乐观缓存，后端校验同样拒绝冲突配置。覆盖禁用项读写 / 恢复、刷新转入禁用项后的旧请求核对、配置克隆 / 保存 / 重建及累计可靠性保留。Surge 普通响应缓存与乐观保留分别控制，继续复核客户端分区、健康 / 路由 generation、Fake-IP 范围与发行 TTL。
- 无组合条件的域名排行改为独立来源 / 域名累计，使用有界待写批次、事务内批次去重、SQLite 累计与七天分钟表；只返回前一百名和精确余量。压力阈值提前保存，无变化时复用最近排行。覆盖超过 4096 种组合后新高频域名进入排行、实际分母、按源 / 历史、提交失败与重试、并发保存 / 读取、重启恢复、缓冲溢出的显式未记录计数。组合条件仍仅匹配真实保留明细，并显示覆盖请求数；独立域名采集显示自身起点，旧丢失明细不倒填。
- App 已打开管理页面在本机后台重启后重新使用原生存储中的凭证登录，同步更新页面 CSRF，短暂失败重试；显式退出保持退出，凭证缺失 / 失效仍要求登录。测试模拟连续会话失效、写请求使用新 CSRF、临时恢复失败、重试取消和本机中继重新连接；密码不经恢复结果返回页面。
- 隐藏 / 固定侧边栏按钮移到左下角，与收起 / 展开并排，验证展开、仅图标、悬浮及移动导航交互。
- 完整本地 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 通过：前端 35 文件 / 181 测试、Go 全包 race / vet、原生核心与真实 WKWebView、17 页面 / 14 设置分类响应式检查、九项分组缓存检查、完整 App 打包及版本 / 资源 / 签名 / DMG / ZIP / SHA256 校验。源码身份为 `1.0.4-beta.22 / build 104`；旧产物已备份。相同 GOCACHE 的 `make fuzz` 通过，执行 203,978 次。
- 本地原始日志：`artifacts/releases/v1.0.4-beta.22/local-verification.log`、`fuzz.log`；双仓库 Actions、Release 类型、附件下载 / 哈希和实际包版本的发布后证据保存至该目录的 `publication-verification.json`，CI build 按对应 Actions run number 核验，不要求与源码 build 相同。
- 未安装、升级或重启现用环境；会话恢复使用模拟原生桥接与隔离后端验证，未做真实运行中的 App / 后台重启验收。未做真实 LAN / Surge / NAT / IPv6 流量及长期资源占用验收。

## 2026-10-15：远程 App 强制重启与轻量提示（beta.23 发布验证）

- 远程后端升级完成、仅图形 App 尚未重启时，管理页面改用普通灰色提示，说明可继续管理；不再显示黄色升级告警，App 重启失败也不作为网络后台失败报告。等待确认 / 稍后 / 失败状态每 30 秒查询一次，实际重启时每 2 秒查询。
- 支持此能力的后端在等待远端确认、稍后或失败状态提供“强制重启远程 App”。本端二次确认后，固定版本请求异步执行 GUI 交接；取消并等待旧助手退出，拒绝重复操作和与安装并发，保留按用户会话降权、固定 App 路径、代码签名与启动时间边界。已手动重开的新进程不被终止，原本关闭的 App 不会被启动。强制操作先正常退出，无响应时终止匹配的旧 App；网络服务不再次重启。请求持久化失败不取消旧助手，超时明确失败并允许重试。
- `npm --prefix web test`：35 文件 / 185 测试通过；最终文案微调后 `npm --prefix web test -- src/backend-update.test.jsx` 17 项通过。`npm --prefix web run build` 与 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build go build ./...` 通过。相同 GOCACHE 的 `go test -race ./internal/softwareupdate ./internal/api ./internal/remote -timeout 120s` 和三包 `go vet` 通过，覆盖取消旧助手 / 旧输出隔离、重复操作、安装互斥、状态与版本校验、持久化失败、子进程组退出、认证及中继 POST 限制。
- `sh scripts/test_native.sh` 通过，包括精确进程路径 / 启动时间 / 助手排除，以及主线程被阻塞时仍收到管道取消；完整生产 Swift 源码 `xcrun swiftc -typecheck ...` 通过。现有 `update_dialog.swift` 的 WebKit 并发警告仍存在。
- `node scripts/ui_forms_check.cjs` 通过 17 页面 / 14 设置分类及 1440、1120、390 px 检查；新增远程轻提示的透明背景 / 无告警边框、手机确认框、取消默认焦点及准确单次提交验证。截图与结果位于 `artifacts/ui-forms/remote-app-reminder-*.png`、`force-app-restart-*.png` 和 `verification.json`；使用模拟 API，不连接实际后端。
- 发布前 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make package package-check` 通过，覆盖完整 App / 后台 / CLI、版本、资源、签名、DMG 布局、ZIP 和 SHA256SUMS；旧产物与二进制已备份至 `artifacts/pre-beta23-backup-20261015/`。版本更新后前端升级 / 版本提示 / 全量翻译三文件共 39 项检查通过；包日志为 `artifacts/releases/v1.0.4-beta.23/package.log`。
- 实现阶段未提交、未发布或重启现用 App / 后台；本轮获授权发布测试版 `v1.0.4-beta.23`（源码 build 105），发布后工作流与双仓库附件证据保存至 `artifacts/releases/v1.0.4-beta.23/`。未授权安装升级现用环境。自动化验证不代替真实多用户远程桌面下的关闭、重开及通知验收；旧版后端不提供强制重启按钮。

## 2026-10-15：满容量监控明细恢复导致历史保存失败（beta.24 发布验证）

- 复现旧版故障：记录 4096 个不同关联并产生溢出后，排序后的溢出汇总排在具体明细之前。恢复与历史合并把该汇总占为一个明细名额，导致最后一个原有明细被并入溢出；下一次差量检查持续返回 `monitoring request dimensions regressed`，事务回滚，页面持续报告保存失败。新增两项针对性测试在修改前分别复现计数回退和溢出误吸收一条原有明细。
- 采集、恢复和历史合并共用容量规则：保留最多 4096 条具体关联，溢出汇总单独保留，已有身份的数量、耗时与具体条件过滤在恢复后不减少。未放宽计数回退校验、未删除检查点或历史；正常事务成功后使用已有逻辑清除管理页存储错误。
- 覆盖三次 JSON 保存 / 恢复、溢出项先后顺序、已保留身份继续累计、新身份继续进入溢出，以及真实 SQLite 关闭 / 重开后继续提交域名批次与历史增量、重试不重复、历史条件过滤。App 测试模拟旧版挤掉明细后保存失败，确认事务保留原检查点、错误对外可见，再从完整检查点恢复并验证后续保存清除告警、累计历史没有少计或重复。
- `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build go test -race ./internal/dnsserver ./internal/database ./internal/app -count=1 -timeout 180s` 三包通过；相同三包 `go vet`、`npm --prefix web run build`、`go build ./...` 通过。前端与原生源码未修改，未重复运行 UI 或 Swift 测试。
- 发布前 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make package package-check` 通过，覆盖完整 App / 后台 / CLI、版本、资源、签名、DMG 布局、ZIP 与 SHA256SUMS；旧产物备份至 `artifacts/pre-beta24-backup-20261015/`，包日志为 `artifacts/releases/v1.0.4-beta.24/package.log`。版本更新后的升级、版本提示和全量翻译三文件共 39 项测试通过。
- 实现阶段未提交、发布、安装、重启现用后台或读取现用数据库。本轮获授权发布 `v1.0.4-beta.24`（源码 build 106），双仓库工作流、附件和版本核验证据保存至 `artifacts/releases/v1.0.4-beta.24/`。已保存检查点与历史可保留；旧版失败期间尚未落盘的请求无法保证跨进程重启恢复，不能补造已丢失的请求关联。修复需随新版后台更新生效。

## 2026-10-15：主机名探测与状态栏后端选择（beta.25 发布验证）

- 「设置 → 日志与隐私」新增默认关闭的 `probe_hostnames`，兼容旧配置并参与克隆和配置保存。后台使用系统解析器串行反向查询未备注客户端，带单次 2 秒超时、每批最多 8 个及轮转选择、成功 1 小时 / 失败 5 分钟缓存；结果随运行实例结束清除，不写入手工备注或持久客户端计数。客户端列表提供探测来源 / 失败提示，大屏名称复用备注优先规则与敏感数据保护，仍按 IP 统计和筛选。覆盖备注跳过、IP 归一化、无效 PTR、缓存重试 / 淘汰、取消、实例启停和只读状态接口。
- 状态栏「打开管理窗口」复用现有本机配置刷新中的已保存后端列表，有远程后端时展开本地与远程子菜单，地址悬停与上次选择勾选；仅本地时直接打开本地。选择同一后端只唤起窗口；切换后端更新原有选择保存和页面会话后重新加载，仍通过本机认证中继访问远端，不切换状态栏监控目标。菜单检查覆盖同名后端按 ID 区分、地址变更、移除、选择动作与本地快捷入口。
- 实现阶段：相关前端 6 文件 / 48 项测试通过；相关 Go 客户端 race 与 vet、前后端构建通过。原生菜单、核心、偏好、本地化、远端重启与真实 WKWebView 检查通过，完整原生程序编译通过。原生 UI 在受限环境下无法访问 WebKit 系统服务，放行后单独重跑通过；既有 `update_dialog.swift` WebKit 并发警告仍存在。
- 发布前 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 全部通过：前端 35 文件 / 189 项、Go 全包 race / vet、原生核心与真实 WKWebView、17 页面 / 14 设置分类（1440 / 1120 / 390）、9 项缓存 UI 检查、完整 App / 后台 / CLI 打包与版本 / 资源 / 签名 / DMG 布局 / ZIP / SHA256 校验（源码 `1.0.4-beta.25 / build 107`）。本地原始日志 `artifacts/releases/v1.0.4-beta.25/local-verification.log`；CI 回归另执行版本解析 fuzz。
- 本轮发布目标为 `v1.0.4-beta.25`（源码 build 107），本地发布检查、打包与双仓库发布证据保存在 `artifacts/releases/v1.0.4-beta.25/`；原有包及二进制备份至 `artifacts/pre-beta25-backup-20261015/`。
- 未授权安装、升级或重启现用 App / 后台；未做真实局域网 PTR、LAN / Surge / NAT / IPv6 流量及长期资源占用验收。后端菜单检查与窗口 / 会话逻辑验证不代替现用 App 的完整远程登录现场验收。

- 发布后核验完成：`v1.0.4-beta.25` 标签指向 `352a8f82c43038957325910ff68acc2e468650a6`，双仓库 main / 注释标签对象在推送后已核对一致。GitHub 回归 `38690630180`（含 fuzz）和 Release `38690630151` 成功，Gitea `2103` 整体成功复用 GitHub 制品 / 回退跳过。两个 Release 均为非草稿测试版，只有 DMG、ZIP、SHA256SUMS 三附件，六附件下载非空、哈希完全一致；实际 App 与捆绑后台均为 `1.0.4-beta.25 / build 33`，ZIP、代码签名与 DMG 完整性通过。证据为 `artifacts/releases/v1.0.4-beta.25/publication-verification.json`。AGENTS.md 的最近已核验发布与本轮交接已同步最终事实，后续仅文档提交不移动标签。

## 2026-10-15：配置按模块应用与主机名域部分截断（beta.26 发布验证）

- 完成所有设置项依赖梳理，应用范围表见 [配置按模块应用](configuration-application.md)。DNS / 缓存 / 健康运行实例与 BGP speaker 分别管理生命周期；日志与隐私、客户端备注与强制策略原地更新。BGP 邻居与手动路由沿用现有实例差量应用。DNS 源地址同时作为启用邻居绑定时仍需联动 BGP，Fake-IP 门禁、响应排空、TTL 和路由健康绑定不绕过。
- 隔离真实 GoBGP 双邻居验证：隐私与客户端修改不替换 DNS 或 BGP；DNS 超时、缓存容量以及同时修改 DNS 和手动路由时，BGP 会话建立时间保持不变，未绑定健康的手动前缀保留；BGP 全局 ASN 修改保留 DNS 实例。DNS 监听启用失败后恢复与重试均保留原 BGP 管理实例。另覆盖记录扩缩容、关闭清理及拒绝旧代写回、并发记录调整和 Fake-IP 响应排空 / TTL 等待。
- `npm --prefix web run build` 通过；`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build go test -race ./... -count=1 -timeout 180s` 全包通过（含 integration）；同一 GOCACHE 下 `go vet ./...`、`go build ./...` 通过。原始前端构建、针对性 race 与完整 race 日志在 `artifacts/configuration-application-2026-10-15/`。测试使用临时数据库、随机本地端口和隔离邻居，未操作现用后台。
- 新增日志与隐私选项 `hostname_device_only`，默认关闭，开启后客户端列表与大屏只显示探测名称第一个点之前的设备名。完整缓存保留，关闭后恢复完整显示；不改变手工备注、客户端身份或策略，不重启 DNS / BGP / PTR 任务。针对性 app / config race 与前端 3 文件 / 17 测试通过，覆盖开关保存、中英文文案、视图热更新与恢复以及 BGP 会话保持。
- `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 最终全部通过，包含新增截断选项：前端 35 文件 / 189 测试、Go 全包 race / vet、原生核心与真实 WKWebView、17 页面 / 14 设置分类（1440 / 1120 / 390）、9 项缓存 UI 检查、完整 App / 后台 / CLI 打包与版本 / 资源 / 签名 / DMG 布局 / ZIP / SHA256 校验。本地 App 与后台均为 `1.0.4-beta.26 / build 108`。
- 发布核验完成：标签 `v1.0.4-beta.26` 指向 `70ddfedf3108ed421be3e408593e0ae9e9d7b233`，双仓库 main / 注释标签对象已核对一致。GitHub 回归 `38696844640`（含 fuzz）与 Release `38696844858` 成功；Gitea `2104` 整体成功复用 GitHub 制品，回退跳过。两个 Release 均为非草稿测试版，各有 DMG / ZIP / SHA256SUMS 三附件，六附件下载非空、SHA256 完全一致；包内 App 与后台均为 `1.0.4-beta.26 / build 34`，ZIP、签名与 DMG 完整性通过。证据为 `artifacts/releases/v1.0.4-beta.26/publication-verification.json`，旧包 / bin 备份在 `artifacts/pre-beta26-backup-20261015/`。AGENTS.md 已更新最新发布及最终交接，后续文档提交不移动标签。未安装升级现用环境，未验收真实网关 RIB / FIB、PF、LAN / IPv6 或长期运行。

## 2026-10-15：多个管理窗口（beta.27 发布验证）

- 状态栏逐后端打开独立窗口，相同 ID 与地址的窗口直接唤起并恢复最小化；标题显示连接名称和地址。每个窗口拥有独立的 WebKit 非持久存储、本地登录会话、CSRF、页面后端选择和可见性事件，不复用状态栏客户端的 Cookie。已保存凭证沿用原有流程独立登录，本机 App 偏好共享但逐窗口保留后端种子。
- 原生 JS bridge 按 `message.webView` 寻找来源窗口，登录和保存结果只回到来源页面；关闭来源后丢弃未完成回调。关闭或最小化一个窗口不改变其他窗口，最后一个窗口关闭后继续后台运行。管理监听改变或恢复时，重新加载各窗口并保留其后端选择。后台通知独立绑定窗口会话，避免一个窗口退出影响其他连接，并在新窗口接续后清理同一后端的旧关闭窗口监控。
- 新请求客户端为远程读取、写入与异步任务轮询携带选中地址，后台中继核对配置中的实际目标，地址改变返回 409。既有登录凭证地址保护保留；旧调用未携带绑定头时保持兼容。普通浏览器标签页的同站点 Cookie 共享规则不变。
- `sh scripts/test_native.sh` 最终整套通过，包括使用真实 AppDelegate / WKWebView 和现有 `web/src/client.js` 的新多窗口验收。隔离 loopback HTTP fixture 验证三个窗口分别访问本地 / 远端 A / 远端 B，Cookie 和 CSRF 独立、后台窗口登录回复不串页、保留未保存编辑、偏好广播不覆盖选择、最小化 / 关闭 / 重开与管理入口迁移。测试构建关闭生产入口和实际凭证 / 偏好种子，不启动现用后台或更新助手。
- `npm --prefix web test` 35 文件 / 190 项通过，`npm --prefix web run build` 通过；`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build go test -race ./internal/remote ./internal/api -count=1 -timeout 90s` 通过，同一 GOCACHE 下 `go vet ./internal/remote ./internal/api` 与 `go build ./...` 通过。按 `scripts/build_app.sh` 的完整 Swift 源码清单，以 macOS 13 目标编译并链接原生可执行文件通过。日志见 `artifacts/multiple-management-windows-2026-10-15/`，既有 `update_dialog.swift` WebKit 并发警告仍存在。
- 发布前 `env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 全部通过：前端 35 文件 / 190 项，Go 全包 race / vet，原生核心 / 真实 WKWebView / 多窗口隔离，17 页面 / 14 分类响应式检查（1440 / 1120 / 390），9 项缓存 UI 和完整 App / 后台 / CLI / DMG / ZIP 校验。本地包 App 与后台均为 `1.0.4-beta.27 / build 109`，签名、版本、资源、哈希与 DMG 布局通过。
- 发布核验完成：`v1.0.4-beta.27` 标签指向 `7c36b3aae8a7bbe590f8dce11c1fac13b371e781`，双仓库 main / 注释标签对象核对一致。GitHub 回归 `38721553427`（含 fuzz）与 Release `38721553616` 成功，Gitea `2105` 整体成功镜像 GitHub 制品 / 回退跳过。两个发布页均为非草稿测试版，各有 DMG / ZIP / SHA256SUMS 三附件，六附件下载非空、哈希相同；App 与后台均为 `1.0.4-beta.27 / build 35`，签名 / ZIP / DMG 完整性通过。证据为 `artifacts/releases/v1.0.4-beta.27/publication-verification.json`，旧包与 bin 保存在 `artifacts/pre-beta27-backup-20261015/`。AGENTS.md 已同步最近发布与最终交接，后续文档提交不移动标签。未安装或重启现用环境，未连接真实远程后端或验收实际钥匙串登录、升级、长期多窗口资源占用及现场网络路径。

## 2026-10-18：DHCPv4（v1.0.5-beta.2 发布检查）

- 实现可选 DHCPv4、九项全局 / 逐设备选项、固定地址、租约持久化、Option 121 与默认路由；启用前检查其他响应服务器和 macOS 网口地址方式。配置独立应用，支持备份 / CLI / 远程中继。
- `env GOCACHE=/private/tmp/routepilot-dhcp-gocache make verify-release` 通过：前端 37 文件 / 197 项，Go 全包 race / vet，原生核心 / 真实 WKWebView / 多窗口，18 页面 / 14 设置分类在 1440 / 1120 / 390 宽度检查，9 项缓存 UI，以及完整 App / 后台 / CLI / DMG / ZIP / 签名 / 版本 / SHA256 校验。本地身份为 `1.0.5-beta.2 / build 114`。
- 同一 GOCACHE 下 `make fuzz` 通过，207,725 次执行。发布前备份旧包及 bin 至 `artifacts/pre-beta2-backup-20261018/`，日志和包报告在 `artifacts/releases/v1.0.5-beta.2/`。
- 已发布 `v1.0.5-beta.2`（`c1f7b31`）：GitHub 回归 `39135288272` / Release `39135304875` 及 Gitea `2118` 全成功。双仓库三附件均下载核验，哈希一致，App / 后台 `1.0.5-beta.2 / build 40`；证据见该版本目录的 `publication-verification.json`。未安装或重启现用环境，未实际开启 DHCP；真实广播收发、iOS 租约 / 路由采纳及长期运行不在本轮本地证据范围内。

## 2026-10-18：DHCP 列表内设备编辑与布局（v1.0.5-beta.3）

- 移除独立设备配置卡片，租约列表保留无有效租约的配置设备，行内打开设备属性弹窗；保存通过独立设备接口，不提交全局草稿。后端能力字段可选，旧版仅降级该入口。
- `npm --prefix web test`：37 文件 / 199 项通过；`go test -race ./internal/api ./internal/remote ./internal/dhcpserver` 及对应 `go vet` 通过。`make ui-test` 覆盖 18 页面 / 14 分类 / 1440、1120、390 宽度，新增 DHCP 明暗主题、单标题、无额外分组边框、保存区间距、编辑弹窗与无障碍名称检查；9 项缓存 UI 通过。
- `env GOCACHE=/private/tmp/routepilot-dhcp-gocache make package package-check` 通过，App / 后台为 `1.0.5-beta.3 / build 115`。日志与包报告在 `artifacts/releases/v1.0.5-beta.3/`。已发布 `v1.0.5-beta.3`（`64ff605`）：GitHub 回归 `39136395753` / Release `39136416256`、Gitea `2119` 均成功，双边六附件下载 / 哈希 / 签名 / ZIP / DMG 校验通过且一致，App / 后台为 build 41；证据见该版 `publication-verification.json`。未安装或重启现用环境，未做真实 DHCP 客户端验收。

## 2026-10-18：设备表单无多余分割线（v1.0.5-beta.4）

- 仅移除 DHCP 设备属性窗口 footer 上边框，新增真实浏览器断言；保留输入框风格、标题、列表内编辑和间距。
- 前端构建、`make ui-test`（18 页 / 14 分类 / 三宽度、明暗主题、9 项缓存检查）和 `env GOCACHE=/private/tmp/routepilot-dhcp-gocache make package package-check` 通过。本地 App / 后台为 `1.0.5-beta.4 / build 116`，证据在 `artifacts/releases/v1.0.5-beta.4/`。未重复本地 Go / 原生测试；beta.3 对应验证保留。
- 已发布 `v1.0.5-beta.4`（`d3f479b`）：GitHub 回归 `39137089104` / Release `39137101493`、Gitea `2120` 均成功。双发布页为非草稿测试版，六附件均下载并完成 SHA256 / 签名 / ZIP / DMG / 包内身份核验，哈希一致，App / 后台均为 build 42；捆绑后台已包含最新无分割线 CSS。证据为该版 `publication-verification.json`。未安装、重启现用环境或实际启用 DHCP。

## 2026-10-18：统计数值自适应字号（v1.0.5-beta.5）

- 共用 `MetricValue` 保留完整数值，宽度不足时缩小、扩宽时恢复，始终单行。DNS / 缓存 / 客户端 / 路由 / 监控卡片复用同一共享 ResizeObserver；仅数值、空间、字体变化时重新测量，无定时器。
- `npm --prefix web test`：38 文件 / 201 项通过，覆盖内容变化、缩小 / 恢复、隐藏零宽度、共享 observer 清理和无 ResizeObserver 的 resize 回退。
- 前端构建与 `make ui-test` 通过（18 页面 / 14 分类 / 1440、1120、390 宽度及 9 项缓存 UI）；真实浏览器使用 16 位数值验证不换行 / 不溢出 / 不缩写和扩宽恢复。
- `env GOCACHE=/private/tmp/routepilot-dhcp-gocache make package package-check` 通过，本地 App / 后台 `1.0.5-beta.5 / build 117`。本轮未重复本地 Go / 原生测试，发布提交仍需完整 CI；未安装或重启现用环境。已发布 `v1.0.5-beta.5`（`cc8f34d`），GitHub 回归 `39137981766` / Release `39137997174` 及 Gitea `2121` 全部成功；双仓库六附件下载 / SHA256 / 签名 / ZIP / DMG / 包内身份校验通过且哈希一致，App / 后台为 build 43。最新统计字号 / 无分割线 CSS 已在捆绑后台中核对。证据：`artifacts/releases/v1.0.5-beta.5/publication-verification.json`。

## 2026-10-30：Dock 图标尺寸（v1.0.5-beta.8）

- App 图标绘制整体缩小 10%，四周保留透明空间；独立生成的图标已目视检查，状态栏图标 PNG 与修改前逐字节一致。
- 本地 `make package package-check` 在沙箱外通过，App 与后台为 `1.0.5-beta.8 / build 120`；沙箱内首次运行因 hdiutil 创建设备失败。实际 Dock 显示及现用 App 升级尚未验收，发布工作流与附件核验进行中。
