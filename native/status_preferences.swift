import AppKit
import Foundation

struct StatusBarPreferences: Codable {
    var animation = true
    var display = "none"
    var metric = "queries_per_second"
    var health = "Surge API"
    var healthValue = "latency_ms"
    var valid: Bool {
        ["none", "metric", "health_state", "health_value"].contains(display)
        && ["queries_per_second", "queries", "direct", "surge", "reject", "errors", "bgp_advertised", "uptime_seconds"].contains(metric)
        && ["latency_ms", "failures", "successes"].contains(healthValue)
        && !health.isEmpty && health.utf8.count <= 256 && !health.contains(where: { $0.isNewline || $0 == "\0" })
    }
    func text(status: [String: Any], health checks: [[String: Any]], bgp: [String: Any], available: Bool) -> String {
        guard display != "none" else { return "" }
        guard available else { return "—" }
        func number(_ value: Any?, suffix: String = "") -> String {
            guard let value = value as? NSNumber, value.doubleValue.isFinite, value.doubleValue >= 0 else { return "—" }
            return String(format: "%.0f", min(value.doubleValue, 999999999)) + suffix
        }
        if display == "metric" {
            let counters = status["counters"] as? [String: Any] ?? [:]
            switch metric {
            case "queries_per_second": return number(status["queries_per_second"], suffix: "/s")
            case "bgp_advertised": return number(bgp["advertised"])
            case "uptime_seconds": return number(status["uptime_seconds"], suffix: "s")
            default: return number(counters[["queries":"Queries", "direct":"Direct", "surge":"Surge", "reject":"Reject", "errors":"Errors"][metric] ?? ""])
            }
        }
        guard let check = checks.first(where: { $0["name"] as? String == health }) else { return "—" }
        if display == "health_state" {
            return ["UP":"✓", "DOWN":"!", "UNKNOWN":"?", "DISABLED":"–"][check["state"] as? String ?? ""] ?? "?"
        }
        return number(check[healthValue], suffix: healthValue == "latency_ms" ? "ms" : "")
    }
}

// Integrate time instead of multiplying total uptime by a changing speed. A
// new QPS sample never rewinds a packet or changes its chosen branch.
struct StatusBarFlowClock {
    private(set) var elapsed: TimeInterval = 0
    private var last: TimeInterval?
    var queriesPerSecond = 0.0
    static func multiplier(_ qps: Double) -> Double {
        guard qps.isFinite, qps > 0 else { return 1 }
        return 1 + min(3, log2(1 + qps) / 2)
    }
    mutating func advance(_ now: TimeInterval) -> TimeInterval {
        if let last { elapsed += max(0, min(0.25, now - last)) * Self.multiplier(queriesPerSecond) }
        last = now
        return elapsed
    }
    mutating func reset() { elapsed = 0; last = nil }
}
