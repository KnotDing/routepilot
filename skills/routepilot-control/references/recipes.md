# Command recipes

Substitute the user's host and actual configured values. Commands use `routepilot` on PATH. Every example acts on `ROUTEPILOT_URL`; global `--url` overrides it. JSON files containing passwords or tokens must stay private; do not paste their content into an agent response.

## Connect and inspect

```sh
export ROUTEPILOT_URL=http://192.0.2.10:1055
routepilot session
routepilot login --username admin --password-file /absolute/private/password-file
routepilot status
routepilot health
routepilot bgp status
routepilot routing status
routepilot dns queries
routepilot cache list --domain example.com --limit 100
routepilot dns test --domain example.com --client-ip 192.0.2.20 --protocol tcp --port 443
```

## Settings and resources

```sh
routepilot settings get > /private/tmp/routepilot-settings.json
# Edit only the requested keys in the fetched complete settings object.
routepilot settings set --file /private/tmp/routepilot-settings.json
routepilot config wait
routepilot health

routepilot routes list
routepilot routes schema
# Prepare a reviewed complete Route entry in route.json.
routepilot routes add --file route.json
routepilot config wait
routepilot bgp status
# Match the exact created entry ID from routes list before updating/deleting.
routepilot routes update ACTUAL_ID --file revised-route.json
routepilot routes delete ACTUAL_ID
routepilot hosts bulk-delete FIRST_ACTUAL_ID SECOND_ACTUAL_ID
```

The same resource verbs apply to `hosts`, `interception`, `overrides`, `peers`, `routes`, `feeds`, and `backends`. Bulk deletion is supported only for hosts, interception, overrides, peers and routes. A backend profile is metadata; use its URL explicitly for direct CLI control. BGP local ASN and Router ID belong in settings, not each peer. Lists can be `null` when empty.

Settings include `surge.restart` for anomaly recovery, `dns.aaaa_fallback`, cache optimistic refresh options and notification error selection. Obtain exact field names and current values from `settings get`; do not construct a partial settings document.

## URL route imports

```sh
routepilot --no-wait routes preview --url https://example.com/routes.txt
routepilot operation status ACTUAL_OPERATION_ID
routepilot routes import --file reviewed-import.json
```

Preview returns `prefixes` and skipped rows. Import body fields: `prefixes`, `next_hop4`, `next_hop6`, `med`, `local_preference`, `communities`, `health_binding`, `enabled`, `description`. For automatic updating also set the original `url`, `auto_update: true`, `interval_minutes`, `sync_ipv4`, and `sync_ipv6`. Use prefixes from the reviewed preview. `feeds list/update/delete` manages the imported feed. Removing a feed retains its installed routes as manual entries; removing routes requires separate requested deletions.

## Diagnostics and API escape hatch

```sh
routepilot cache flush --domain example.com
routepilot --no-wait dns import-hosts
routepilot operation status ACTUAL_OPERATION_ID
routepilot routing verify --file diagnostic.json
routepilot bgp export-frr --file gateway.json
routepilot api GET 'dns/cache?limit=100&offset=100&decision=DIRECT'
routepilot config export > backup.json
```

`routing verify` takes `prefix`, `target`, and `port` for a configured static Fake-IP TCP diagnostic; `bgp export-frr` renders configuration, never installs it on a gateway. FRR input: `peer_id` (optional), `gateway_asn`, `gateway_router_id`, `controller_address`. Read the API error for missing or inconsistent values. `api` accepts a same-origin relative path and optional JSON `--file`; it does not fetch arbitrary URLs, follow redirects, bypass management authorization, or expose login/session tokens. Long operations should use the named commands, which recognize their async endpoints.

## Complete App upgrade

```sh
routepilot updates check
routepilot updates install --version CHECKED_LATEST_VERSION --yes
routepilot updates status
```

Installation is asynchronous and survives closing the CLI. Check support/reason first: currently standard installed macOS Apple Silicon services, with management authentication enabled. `phase: succeeded` confirms installation/service upgrade; inspect `app_current`, `app_restart` and `app_restart_error` to confirm the running App. Target-side active users may need to approve restart. Connection loss does not justify another install request. Reconnect with the same origin; if session handoff failed/expired, login again before polling.
