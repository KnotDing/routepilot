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
        precondition(translated("检查 Beta 版本更新", "en") == "Check for beta updates")
        precondition(translated("自动检查 Beta 版本", "en") == "Check for beta updates automatically")
        precondition(translated("启用“自动检查 Beta 版本”后，自动检查会同时比较正式版与最新 Beta；Beta 遵循更新提示、自动安装和计划时间设置。测试版可能包含未完成的功能。", "en") == "When enabled, automatic checks compare the latest stable release with the latest beta. Beta updates follow your update prompts, automatic installation, and scheduled time settings. Beta builds may contain unfinished features.")
        precondition(translated("请在“设置 → 软件更新”中执行完整升级，以同步更新 App 和已安装的本机网络后台。通过 DMG/ZIP 手动替换 App 不会更新后台；测试版请使用“检查 Beta 版本更新”修复。", "en") == "Use the full upgrade in Settings → Software Updates to update the App and installed local network service together. Replacing the App manually from a DMG/ZIP does not update the service; for a beta, use Check for beta updates to repair it.")
        precondition(translated("要用 App 携带的后台覆盖正在运行的本机网络后台吗？覆盖会短暂中断网络服务。", "en") == "Overwrite the running local network service with the backend bundled in this App? The network service is briefly interrupted during the replacement.")
        precondition(translated("覆盖后台服务", "en") == "Overwrite backend service")
        precondition(translated("后台服务已覆盖", "en") == "Backend service overwritten")
        precondition(translated("等待后台覆盖完成超时，请检查网络服务状态后重试", "en") == "Timed out waiting for the backend replacement. Check the network service and retry.")
        precondition(translated("Bundled backend version does not match its App metadata", "zh-CN") == "内置后台的版本与 App 版本信息不一致")
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
