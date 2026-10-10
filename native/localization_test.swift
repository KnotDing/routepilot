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
        precondition(translated("检查更新", "en") == "Check for updates")
        precondition(translated("检查Beta版本更新", "en") == "Check for beta updates")
        precondition(translated("自动更新时间（本地时间）", "en") == "Automatic update time (local time)")
        precondition(translated("自动检查仍按设置的检查周期独立运行；发现更新后会立即下载并校验，可手动立即安装。启用自动安装后，RoutePilot App 运行期间每天到达设定的本地时间时还会自动安装。若 Mac 在该时间休眠或 App 未运行，当天不会延迟补装。", "en") == "Automatic checks continue on their independent interval. Detected updates are downloaded and verified immediately, and can be installed manually right away. With automatic installation enabled, updates are installed daily at this local time while the App is running. If the Mac is asleep or the App is not running then, the update is not installed later that day.")
        precondition(translated("立即安装", "en") == "Install now")
        precondition(translated("正在检查更新…", "en") == "Checking for updates…")
        precondition(translated("Authorization failed.", "zh-CN") == "管理员授权失败")
        precondition(translated("查询：{count} / 分钟", "en", ["count": 12]) == "Queries: 12 / minute")
        precondition(translated("请求失败（HTTP 503）", "en") == "Request failed (HTTP 503)")
        precondition(translated("real resolver failed: context deadline exceeded", "zh-CN") == "真实 DNS 解析失败：context deadline exceeded")
        precondition(translated("第 {line} 行：{reason}", "en", ["line": 2, "reason": "example.com {raw}\nvalue"]) == "Line 2: example.com {raw}\nvalue")
        precondition(translated("example.com. 30 IN A 192.0.2.1", "zh-CN") == "example.com. 30 IN A 192.0.2.1")
        let backend = URL(string: "http://127.0.0.1:1055")!
        precondition(NativeText.acceptsLocale("en", mainFrame: true, scheme: "http", host: "127.0.0.1", port: 1055, backend: backend))
        precondition(!NativeText.acceptsLocale("en", mainFrame: false, scheme: "http", host: "127.0.0.1", port: 1055, backend: backend))
        precondition(!NativeText.acceptsLocale("en", mainFrame: true, scheme: "http", host: "192.0.2.1", port: 1055, backend: backend))
        precondition(!NativeText.acceptsLocale("en", mainFrame: true, scheme: "http", host: "127.0.0.1", port: 1056, backend: backend))
        precondition(!NativeText.acceptsLocale("fr", mainFrame: true, scheme: "http", host: "127.0.0.1", port: 1055, backend: backend))
        precondition(NativeText.acceptsLocale("zh-CN", mainFrame: true, scheme: "http", host: "::1", port: 1055, backend: URL(string: "http://[::1]:1055")!))
        print("Native localization: menu, diagnostics, templates and raw values verified")
    }
}
