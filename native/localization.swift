import Foundation

enum NativeText {
    static var locale: String {
        let stored = UserDefaults.standard.string(forKey: "uiLanguage")
        if stored == "en" || stored == "zh-CN" { return stored! }
        return Locale.preferredLanguages.first?.hasPrefix("zh") == true ? "zh-CN" : "en"
    }
    static func catalog(_ name: String) -> [String: String] {
        guard let url = Bundle.main.url(forResource: name, withExtension: "json"), let data = try? Data(contentsOf: url), let value = try? JSONDecoder().decode([String: String].self, from: data) else { return [:] }
        return value
    }
    static let english = catalog("ui-en")
    static let chineseErrors = catalog("errors-zh")
    static func matchedParameters(_ template: String, _ text: String) -> [String: Any]? {
        let marker = try! NSRegularExpression(pattern: "\\{(\\w+)\\}")
        let matches = marker.matches(in: template, range: NSRange(template.startIndex..., in: template))
        guard !matches.isEmpty else { return nil }
        var pattern = "^", keys: [String] = [], previous = template.startIndex
        for match in matches {
            guard let range = Range(match.range, in: template), let keyRange = Range(match.range(at: 1), in: template) else { return nil }
            pattern += NSRegularExpression.escapedPattern(for: String(template[previous..<range.lowerBound])) + "([\\s\\S]*?)"
            keys.append(String(template[keyRange])); previous = range.upperBound
        }
        pattern += NSRegularExpression.escapedPattern(for: String(template[previous...])) + "$"
        guard let expression = try? NSRegularExpression(pattern: pattern), let result = expression.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)) else { return nil }
        var parameters: [String: Any] = [:]
        for (index, key) in keys.enumerated() {
            guard let range = Range(result.range(at: index + 1), in: text) else { return nil }
            parameters[key] = String(text[range])
        }
        return parameters
    }
    static func translate(_ text: String, locale: String, english: [String: String], errors: [String: String], params: [String: Any] = [:]) -> String {
        var result = locale == "en" ? english[text] ?? text : errors[text] ?? text
        var parameters = params
        if result == text && params.isEmpty {
            let pairs = locale == "en" ? english : errors.merging(Dictionary(english.map { ($0.value, $0.key) }, uniquingKeysWith: { first, _ in first }), uniquingKeysWith: { first, _ in first })
            for (source, target) in pairs {
                if source == text { result = target; break }
                if let matched = matchedParameters(source, text) { result = target; parameters = matched; break }
            }
        }
        let expression = try! NSRegularExpression(pattern: "\\{(\\w+)\\}")
        let range = NSRange(result.startIndex..., in: result)
        var output = result
        for match in expression.matches(in: result, range: range).reversed() {
            guard let keyRange = Range(match.range(at: 1), in: result), let outputRange = Range(match.range, in: output), let value = parameters[String(result[keyRange])] else { continue }
            output.replaceSubrange(outputRange, with: String(describing: value))
        }
        return output
    }
}
func L(_ text: String, _ params: [String: Any] = [:]) -> String {
    NativeText.translate(text, locale: NativeText.locale, english: NativeText.english, errors: NativeText.chineseErrors, params: params)
}
func stateText(_ state: Any) -> String {
    let value = String(describing: state)
    let pairs: [String: [String]] = ["STARTING": ["启动中", "Starting"], "HEALTHY": ["健康", "Healthy"], "RECOVERING": ["恢复中", "Recovering"], "DIRECT_FALLBACK": ["真实 DNS 回退", "Real DNS fallback"], "SURGE_DEGRADED": ["Surge 降级", "Surge degraded"], "STOPPING": ["停止中", "Stopping"], "UP": ["正常", "Up"], "DOWN": ["故障", "Down"], "auto": ["自动", "Automatic"], "force-direct": ["强制直连", "Force direct"], "force-surge": ["全部交给 Surge", "Send all to Surge"], "bgp": ["BGP", "BGP"], "static": ["静态", "Static"], "hybrid": ["混合", "Hybrid"]]
    return pairs[value]?[NativeText.locale == "en" ? 1 : 0] ?? value
}
