import AppKit
import Foundation

struct StatusBarPreferences: Codable {
    var animation = true
    var display = "none"
    var metric = "queries_per_second"
    var health = "Surge API"
    var healthValue = "latency_ms"
    static let metrics = [
        "queries_per_second": "每秒 DNS 请求数", "queries_per_minute": "每分钟 DNS 请求数",
        "queries": "DNS 请求总数", "direct": "DIRECT 请求总数", "surge": "SURGE 请求总数",
        "reject": "REJECT 请求总数", "errors": "DNS 错误总数",
        "client_count": "已跟踪客户端数", "client_queries": "客户端 DNS 请求总数",
        "client_direct": "客户端 DIRECT 请求数", "client_surge": "客户端 SURGE 请求数",
        "client_reject": "客户端 REJECT 请求数", "client_unknown": "客户端 UNKNOWN 请求数",
        "client_errors": "客户端 DNS 错误数",
        "direct_ratio": "DIRECT 请求占比", "surge_ratio": "SURGE 请求占比",
        "reject_ratio": "REJECT 请求占比", "unknown_ratio": "UNKNOWN 请求占比", "errors_ratio": "DNS 错误占比",
        "bgp_advertised": "BGP 已发布路由数", "bgp_utilization": "BGP 路由容量占比",
        "decision_cache_entries": "决策缓存条目数", "decision_cache_hits": "决策缓存命中次数", "decision_cache_misses": "决策缓存未命中次数",
        "response_cache_entries": "查询缓存条目数", "response_cache_hits": "查询缓存命中次数", "response_cache_misses": "查询缓存未命中次数",
        "uptime_seconds": "后台运行秒数"
    ]
    var valid: Bool {
        ["none", "metric", "health_state", "health_value"].contains(display)
        && Self.metrics[metric] != nil
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
        func percentage(_ value: Any?, total: Any?) -> String {
            guard let value = value as? NSNumber, let total = total as? NSNumber,
                  value.doubleValue.isFinite, value.doubleValue >= 0,
                  total.doubleValue.isFinite, total.doubleValue > 0 else { return "—" }
            return number(min(value.doubleValue / total.doubleValue, 1) * 100, suffix: "%")
        }
        if display == "metric" {
            let counters = status["counters"] as? [String: Any] ?? [:]
            switch metric {
            case "queries_per_second": return number(status["queries_per_second"], suffix: "/s")
            case "queries_per_minute": return number(status["queries_per_minute"], suffix: "/min")
            case "direct_ratio", "surge_ratio", "reject_ratio", "unknown_ratio", "errors_ratio":
                let key = ["direct_ratio":"Direct", "surge_ratio":"Surge", "reject_ratio":"Reject", "unknown_ratio":"Unknown", "errors_ratio":"Errors"][metric]!
                return percentage(counters[key], total: counters["Queries"])
            case "bgp_advertised": return number(bgp["advertised"])
            case "bgp_utilization": return percentage(bgp["advertised"], total: bgp["limit"])
            case "decision_cache_entries", "decision_cache_hits", "decision_cache_misses":
                let stats = (status["cache"] as? [String: Any])?["decision"] as? [String: Any] ?? [:]
                let key = ["decision_cache_entries":"entries", "decision_cache_hits":"hits", "decision_cache_misses":"misses"][metric]!
                return number(stats[key])
            case "response_cache_entries", "response_cache_hits", "response_cache_misses":
                let stats = (status["cache"] as? [String: Any])?["response"] as? [String: Any] ?? [:]
                let key = ["response_cache_entries":"entries", "response_cache_hits":"hits", "response_cache_misses":"misses"][metric]!
                return number(stats[key])
            case "client_count", "client_queries", "client_direct", "client_surge", "client_reject", "client_unknown", "client_errors":
                let stats = (status["client_statistics"] as? [String: Any])?["stats"] as? [String: Any] ?? [:]
                let key = ["client_count":"clients", "client_queries":"queries", "client_direct":"direct", "client_surge":"surge", "client_reject":"reject", "client_unknown":"unknown", "client_errors":"errors"][metric]!
                return number(stats[key])
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
