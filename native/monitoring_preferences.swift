import Foundation

struct MonitoringPreferences: Codable, Equatable {
    static let cardIDs: Set<String> = ["received", "upstream", "completed", "inflight", "unconfirmed", "latency", "dnsFlow", "clientFlow", "policies", "sources", "trend", "decisions", "rcodes"]
    var hiddenCards: [String] = []
    var protectSensitive = false
    var valid: Bool { hiddenCards.count <= Self.cardIDs.count && Set(hiddenCards).count == hiddenCards.count && hiddenCards.allSatisfy(Self.cardIDs.contains) }
    enum CodingKeys: String, CodingKey { case hiddenCards, protectSensitive }
    init() {}
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        hiddenCards = try values.decodeIfPresent([String].self, forKey: .hiddenCards) ?? []
        protectSensitive = try values.decodeIfPresent(Bool.self, forKey: .protectSensitive) ?? false
    }
}
