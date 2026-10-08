import Foundation

@main
struct LocalizationTests {
    static func main() throws {
        let directory = URL(fileURLWithPath: CommandLine.arguments[1])
        let english = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: directory.appendingPathComponent("ui-en.json")))
        let errors = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: directory.appendingPathComponent("errors-zh.json")))
        func translated(_ text: String, _ locale: String, _ params: [String: Any] = [:]) -> String {
            NativeText.translate(text, locale: locale, english: english, errors: errors, params: params)
        }
        precondition(translated("更新网络后台服务…", "en") == "Update network service…")
        precondition(translated("Authorization failed.", "zh-CN") == "管理员授权失败")
        precondition(translated("查询：{count} / 分钟", "en", ["count": 12]) == "Queries: 12 / minute")
        precondition(translated("请求失败（HTTP 503）", "en") == "Request failed (HTTP 503)")
        precondition(translated("real resolver failed: context deadline exceeded", "zh-CN") == "真实 DNS 解析失败：context deadline exceeded")
        precondition(translated("第 {line} 行：{reason}", "en", ["line": 2, "reason": "example.com {raw}\nvalue"]) == "Line 2: example.com {raw}\nvalue")
        precondition(translated("example.com. 30 IN A 192.0.2.1", "zh-CN") == "example.com. 30 IN A 192.0.2.1")
        print("Native localization: menu, diagnostics, templates and raw values verified")
    }
}
