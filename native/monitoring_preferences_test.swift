import Foundation

@main struct MonitoringPreferencesTests {
    static func main() throws {
        let decoder = JSONDecoder()
        let defaults = try decoder.decode(MonitoringPreferences.self, from: Data("{}".utf8))
        precondition(defaults.hiddenCards.isEmpty && !defaults.protectSensitive && defaults.valid)
        var saved = MonitoringPreferences()
        saved.hiddenCards = ["sources", "domains", "dnsFlow", "latency"]
        saved.protectSensitive = true
        let restored = try decoder.decode(MonitoringPreferences.self, from: JSONEncoder().encode(saved))
        precondition(restored == saved && restored.valid)
        saved.hiddenCards = ["unknown"]
        precondition(!saved.valid)
        saved.hiddenCards = ["sources", "sources"]
        precondition(!saved.valid)
        do {
            _ = try decoder.decode(MonitoringPreferences.self, from: Data("{\"protectSensitive\":\"true\"}".utf8))
            preconditionFailure("Invalid protection flag must fail decoding")
        } catch is DecodingError {}
        print("PASS native monitoring preferences: migration, persistence and validation")
    }
}
