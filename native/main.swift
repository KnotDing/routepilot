import AppKit
import WebKit
import ServiceManagement
import Darwin
import Security

struct AppFailure: LocalizedError {
    let message: String
    let statusCode: Int?
    init(message: String, statusCode: Int? = nil) {
        self.message = message
        self.statusCode = statusCode
    }
    var errorDescription: String? { message }
}

struct LoginCredential: Codable {
    let username: String
    let password: String
}

enum CredentialVault {
    private static let service = Bundle.main.bundleIdentifier ?? "com.routepilot.app"
    private static let account = "controller-login"
    private static var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }
    static func load() -> LoginCredential? {
        var request = query
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data else { return nil }
        return try? JSONDecoder().decode(LoginCredential.self, from: data)
    }
    static func save(username: String, password: String) throws {
        let data = try JSONEncoder().encode(LoginCredential(username: username, password: password))
        let attributes: [String: Any] = [kSecValueData as String: data, kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly]
        let result = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if result == errSecItemNotFound {
            var request = query
            attributes.forEach { request[$0.key] = $0.value }
            let addResult = SecItemAdd(request as CFDictionary, nil)
            guard addResult == errSecSuccess else { throw AppFailure(message: "钥匙串无法保存登录凭证") }
        } else if result != errSecSuccess {
            throw AppFailure(message: "钥匙串无法保存登录凭证")
        }
    }
    static func delete() {
        SecItemDelete(query as CFDictionary)
    }
}

final class ControllerClient {
    var base: URL
    var csrf = ""
    let session: URLSession
    init(_ base: URL, cookieStorage: HTTPCookieStorage? = nil) {
        self.base = base
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 4
        configuration.httpShouldSetCookies = true
        if let cookieStorage { configuration.httpCookieStorage = cookieStorage }
        configuration.urlCache = nil
        session = URLSession(configuration: configuration)
    }
    func call(_ path: String, method: String = "GET", body: Any? = nil) async throws -> Any {
        var request = URLRequest(url: base.appendingPathComponent("api/v1/" + path))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if method != "GET" { request.setValue(csrf, forHTTPHeaderField: "X-CSRF-Token") }
        if let body = body { request.httpBody = try JSONSerialization.data(withJSONObject: body) }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 0
            let value = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
            throw AppFailure(message: value?["error"] as? String ?? L("请求失败（HTTP {code}）", ["code": code]), statusCode: code)
        }
        return try JSONSerialization.jsonObject(with: data)
    }
    func establish() async throws -> Bool {
        let value = try await call("session") as? [String: Any] ?? [:]
        csrf = value["csrf"] as? String ?? ""
        return value["authenticated"] as? Bool ?? false
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKDownloadDelegate, NSMenuDelegate, WKScriptMessageHandler {
    let arguments = CommandLine.arguments
    var client: ControllerClient!
    var statusItem: NSStatusItem!
    var window: NSWindow?
    var webView: WKWebView?
    var child: Process?
    var logHandle: FileHandle?
    var timer: Timer?
    var releaseTimer: Timer?
    var refreshing = false
    var quitting = false
    var authenticated = false
    var starting = false
    var stopping = false
    var bootFailures = 0
    var lastStartError = ""
    var nextBoot = Date.distantPast
    var dataDirectory = URL(fileURLWithPath: "")
    var status: [String: Any] = [:]
    var health: [[String: Any]] = []
    var bgp: [String: Any] = [:]
    var availableRelease: GiteaRelease?
    var checkingRelease = false
    var lastError = "正在连接…"
    var smokeStarted = false
    var smokeDownloadURL: URL?
    var smoke: Bool { arguments.contains("--smoke-test") }
    func installEditMenu() {
        let mainMenu = NSMenu()
        let editItem = NSMenuItem(title: L("编辑"), action: nil, keyEquivalent: "")
        let editMenu = NSMenu(title: L("编辑"))
        for (title, selector, key) in [
            ("剪切", #selector(NSText.cut(_:)), "x"),
            ("复制", #selector(NSText.copy(_:)), "c"),
            ("粘贴", #selector(NSText.paste(_:)), "v"),
            ("全选", #selector(NSText.selectAll(_:)), "a")
        ] {
            let item = NSMenuItem(title: L(title), action: selector, keyEquivalent: key)
            item.keyEquivalentModifierMask = .command
            editMenu.addItem(item)
        }
        editItem.submenu = editMenu
        mainMenu.addItem(editItem)
        NSApp.mainMenu = mainMenu
    }
    func argument(_ key: String) -> String? {
        guard let i = arguments.firstIndex(of: key), i + 1 < arguments.count else { return nil }
        return arguments[i + 1]
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        installEditMenu()
        if let icon = Bundle.main.url(forResource: "AppIcon", withExtension: "icns") { NSApp.applicationIconImage = NSImage(contentsOf: icon) }
        let stored = argument("--backend-url") ?? UserDefaults.standard.string(forKey: "backendURL") ?? "http://127.0.0.1:1055"
        guard let url = validURL(stored) else { alert("管理地址无效", "仅支持本机 loopback HTTP/HTTPS 地址。"); NSApp.terminate(nil); return }
        client = ControllerClient(url)
        dataDirectory = argument("--data-directory").map { URL(fileURLWithPath: $0) } ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("RoutePilot")
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = Bundle.main.url(forResource: "MenuIcon", withExtension: "png").flatMap { NSImage(contentsOf: $0) }
            button.image?.size = NSSize(width: 20, height: 20)
            button.image?.isTemplate = true
            button.toolTip = "RoutePilot · " + L("正在连接…")
        }
        rebuildMenu()
        timer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
        Task { await refresh() }
        Task { await checkReleaseUpdate(silent: true) }
        releaseTimer = Timer.scheduledTimer(withTimeInterval: 12 * 60 * 60, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.checkReleaseUpdate(silent: true) }
        }
        if smoke {
            DispatchQueue.main.asyncAfter(deadline: .now() + 25) { [weak self] in
                guard let self = self, !self.smokeStarted else { return }
                self.finishSmoke(["passed": false, "error": self.lastError, "startup_error": self.lastStartError, "boot_failures": self.bootFailures])
            }
        }
    }
    func validURL(_ text: String) -> URL? {
        guard let url = URL(string: text), ["http", "https"].contains(url.scheme ?? ""),
              isLocalAddress(url.host ?? ""),
              url.user == nil, url.password == nil, url.query == nil, url.fragment == nil,
              url.path.isEmpty || url.path == "/", url.port != nil, (1...65535).contains(url.port ?? 0) else { return nil }
        return url
    }
    func isLocalAddress(_ rawHost: String) -> Bool {
        let host = rawHost.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if host == "localhost" || host == "::1" { return true }
        var ipv4 = in_addr()
        if inet_pton(AF_INET, host, &ipv4) == 1 {
            let bytes = withUnsafeBytes(of: ipv4) { Array($0) }
            if bytes.first == 127 { return true }
        }
        var interfaces: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&interfaces) == 0, let first = interfaces else { return false }
        defer { freeifaddrs(first) }
        var current: UnsafeMutablePointer<ifaddrs>? = first
        while let item = current {
            let entry = item.pointee
            if let address = entry.ifa_addr {
                if Int32(address.pointee.sa_family) == AF_INET {
                    let candidate = UnsafeRawPointer(address).assumingMemoryBound(to: sockaddr_in.self).pointee.sin_addr
                    if inet_pton(AF_INET, host, &ipv4) == 1, candidate.s_addr == ipv4.s_addr { return true }
                } else if Int32(address.pointee.sa_family) == AF_INET6 {
                    var candidate = UnsafeRawPointer(address).assumingMemoryBound(to: sockaddr_in6.self).pointee.sin6_addr
                    var ipv6 = in6_addr()
                    if inet_pton(AF_INET6, host, &ipv6) == 1,
                       memcmp(&candidate, &ipv6, MemoryLayout<in6_addr>.size) == 0 { return true }
                }
            }
            current = entry.ifa_next
        }
        return false
    }
    func refresh() async {
        guard !refreshing, !quitting, !stopping else { return }
        refreshing = true
        defer { refreshing = false }
        do {
            try await refreshFromBackend()
        } catch {
            let connectionError = error.localizedDescription
            let recovery = URL(string: "http://127.0.0.1:1055")!
            if !smoke, client.base.absoluteString != recovery.absoluteString {
                let previous = client!
                client = ControllerClient(recovery, cookieStorage: previous.session.configuration.httpCookieStorage)
                do {
                    try await refreshFromBackend()
                    UserDefaults.standard.set(recovery.absoluteString, forKey: "backendURL")
                    if let webView { webView.load(URLRequest(url: recovery)) }
                    updateToolTip()
                    rebuildMenu()
                    return
                } catch {
                    client = previous
                }
            }
            authenticated = false; status = [:]; lastError = connectionError
            if child == nil && !starting && bootFailures < 5 && Date() >= nextBoot && client.base.scheme == "http" && (smoke || !FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist")) {
                startBackend()
            }
        }
        updateToolTip()
        rebuildMenu()
    }
    func refreshFromBackend() async throws {
        authenticated = try await client.establish()
        if !authenticated, let credential = CredentialVault.load() {
            do {
                _ = try await client.call("login", method: "POST", body: ["username": credential.username, "password": credential.password])
                authenticated = try await client.establish()
            } catch let failure as AppFailure where failure.statusCode == 401 {
                CredentialVault.delete()
                throw failure
            }
        }
        if authenticated {
            status = try await client.call("status") as? [String: Any] ?? [:]
            health = try await client.call("health") as? [[String: Any]] ?? []
            bgp = try await client.call("bgp/status") as? [String: Any] ?? [:]
            lastError = ""
            if smoke && !smokeStarted {
                smokeStarted = true
                Task { await runSmoke() }
            }
        } else {
            status = [:]; lastError = "需要登录管理服务"
        }
    }
    func updateToolTip() {
        statusItem.button?.toolTip = "RoutePilot · " + (lastError.isEmpty ? stateText(status["state"] ?? "—") + " · " + stateText(status["mode"] ?? "auto") : L(lastError))
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        let origin = message.frameInfo.securityOrigin
        if message.name == "routepilotCredentials" {
            let host = client.base.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
            guard message.frameInfo.isMainFrame, origin.protocol == client.base.scheme,
                  origin.host == host, origin.port == client.base.port,
                  let values = message.body as? [String: String],
                  let username = values["username"], !username.isEmpty,
                  let password = values["password"], (12...72).contains(password.utf8.count) else { return }
            do {
                try CredentialVault.save(username: username, password: password)
                Task { await refresh() }
            } catch { alert("登录凭证未保存", error.localizedDescription) }
            return
        }
        guard message.name == "routepilotLocale", let locale = message.body as? String,
              NativeText.acceptsLocale(locale, mainFrame: message.frameInfo.isMainFrame, scheme: origin.protocol, host: origin.host, port: origin.port, backend: client.base) else { return }
        UserDefaults.standard.set(locale, forKey: "uiLanguage")
        configureLanguageSeed(userContentController)
        window?.title = L("RoutePilot · DNS 和路由控制器")
        updateToolTip()
        rebuildMenu()
    }
    func configureLanguageSeed(_ controller: WKUserContentController) {
        let marker = "/* RoutePilot language */"
        let scripts = controller.userScripts.filter { !$0.source.hasPrefix(marker) }
        controller.removeAllUserScripts()
        scripts.forEach { controller.addUserScript($0) }
        let source = marker + "if(!localStorage.getItem('routepilot.locale'))localStorage.setItem('routepilot.locale','\(NativeText.locale)');"
        controller.addUserScript(WKUserScript(source: source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
    }
    func startBackend() {
        guard child == nil, !starting, !quitting else { return }
        starting = true
        defer { starting = false }
        do {
            try FileManager.default.createDirectory(at: dataDirectory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            let logURL = dataDirectory.appendingPathComponent("service.log")
            if let size = try? logURL.resourceValues(forKeys: [.fileSizeKey]).fileSize, size > 8 * 1024 * 1024 {
                let old = dataDirectory.appendingPathComponent("service.previous.log")
                try? FileManager.default.removeItem(at: old)
                try FileManager.default.moveItem(at: logURL, to: old)
            }
            if !FileManager.default.fileExists(atPath: logURL.path) {
                FileManager.default.createFile(atPath: logURL.path, contents: nil, attributes: [.posixPermissions: 0o600])
            }
            logHandle = try FileHandle(forWritingTo: logURL)
            try logHandle?.seekToEnd()
            let process = Process()
            process.executableURL = Bundle.main.resourceURL!.appendingPathComponent("Service/bin/surge-controller")
            let host = client.base.host == "localhost" ? "127.0.0.1" : client.base.host!.replacingOccurrences(of: "[", with: "").replacingOccurrences(of: "]", with: "")
            let bind = (host.contains(":") ? "[\(host)]" : host) + ":\(client.base.port ?? 1055)"
            process.arguments = ["serve", "--database", dataDirectory.appendingPathComponent("controller.sqlite").path, "--web-listen", bind]
            process.currentDirectoryURL = dataDirectory
            process.standardOutput = logHandle; process.standardError = logHandle
            process.terminationHandler = { [weak self] _ in
                Task { @MainActor in
                    guard let self = self else { return }
                    self.child = nil
                    try? self.logHandle?.close(); self.logHandle = nil
                    if self.quitting { NSApp.reply(toApplicationShouldTerminate: true) }
                    else if !self.stopping {
                        self.bootFailures += 1
                        self.nextBoot = Date().addingTimeInterval(Double(min(30, self.bootFailures * 3)))
                        self.lastError = "后台服务已退出；查看日志了解原因"
                        self.rebuildMenu()
                    }
                }
            }
            try process.run()
            child = process
        } catch { bootFailures += 1; lastStartError = error.localizedDescription; lastError = error.localizedDescription }
    }
    func item(_ title: String, action: Selector? = nil, enabled: Bool = true) -> NSMenuItem {
        let item = NSMenuItem(title: L(title), action: action, keyEquivalent: "")
        item.target = self; item.isEnabled = enabled
        return item
    }
    func rebuildMenu() {
        let menu = NSMenu(); menu.autoenablesItems = false; menu.delegate = self
        menu.addItem(item("RoutePilot", enabled: false))
        menu.addItem(item(L("Controller：{state}", ["state": lastError.isEmpty ? stateText(status["state"] ?? "—") : L(lastError)]), enabled: false))
        let surge = health.first { $0["name"] as? String == "Surge CLI" }?["state"] ?? "—"
        menu.addItem(item(L("Surge：{state} · Fake-IP：{enabled}", ["state": stateText(surge), "enabled": L((status["fake_ip_enabled"] as? Bool ?? false) ? "启用" : "禁用")]), enabled: false))
        let peers = bgp["peers"] as? [[String: Any]] ?? []
        let up = peers.filter { ($0["state"] as? String)?.uppercased() == "ESTABLISHED" }.count
        menu.addItem(item(L("BGP：{up}/{total} 邻居 · {routes} 路由", ["up": up, "total": peers.count, "routes": bgp["advertised"] ?? 0]), enabled: false))
        menu.addItem(item(L("路由：{mode} · Fake-IP {fake} / DNS {dns}", ["mode": stateText(bgp["mode"] ?? "bgp"), "fake": stateText(bgp["fake_ip_backend"] ?? "bgp"), "dns": stateText(bgp["dns_backend"] ?? "bgp")]), enabled: false))
        menu.addItem(item(L("查询：{count} / 分钟", ["count": status["queries_per_minute"] ?? 0]), enabled: false))
        menu.addItem(.separator())
        let mode = item("DNS 模式", enabled: authenticated)
        let modes = NSMenu(); modes.autoenablesItems = false
        for (value, label) in [("auto", "自动"), ("force-direct", "强制 DIRECT"), ("force-surge", "强制 SURGE")] {
            let row = item(label, action: #selector(changeMode(_:)), enabled: authenticated)
            row.representedObject = value
            row.state = status["mode"] as? String == value ? .on : .off
            modes.addItem(row)
        }
        mode.submenu = modes; menu.addItem(mode)
        menu.addItem(item("清理 DNS 缓存", action: #selector(flushCache), enabled: authenticated))
        menu.addItem(item("从 Surge 导入 Hosts…", action: #selector(importHosts), enabled: authenticated))
        menu.addItem(item(authenticated ? "管理窗口 / 配置本地 DNS…" : "管理窗口 / 登录…", action: #selector(openManagement)))
        menu.addItem(item("登录状态栏控制…", action: #selector(signIn), enabled: !authenticated))
        menu.addItem(.separator())
        let login = item("登录时启动", action: #selector(toggleLogin))
        login.state = SMAppService.mainApp.status == .enabled ? .on : .off
        menu.addItem(login)
        let serviceInstalled = FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist")
        menu.addItem(item(serviceInstalled ? "更新网络后台服务…" : "安装网络后台服务…", action: #selector(installService), enabled: (serviceInstalled || authenticated) && !stopping && !smoke))
        menu.addItem(item("管理服务地址…", action: #selector(changeAddress), enabled: !stopping))
        menu.addItem(item("打开服务日志", action: #selector(openLogs)))
        menu.addItem(item("重新连接", action: #selector(reconnect)))
        let updateTitle = checkingRelease ? L("正在检查软件更新…") : (availableRelease.map { L("在线升级至 {tag}", ["tag": $0.tagName]) } ?? L("检查软件更新…"))
        menu.addItem(item(updateTitle, action: #selector(performReleaseUpdate), enabled: !checkingRelease && !stopping))
        menu.addItem(.separator())
        menu.addItem(item(child == nil ? "退出状态栏 APP（服务继续运行）" : "退出 APP 并停止后台服务", action: #selector(quit)))
        statusItem.menu = menu
    }
    func checkReleaseUpdate(silent: Bool) async {
        if checkingRelease { return }
        let lastCheck = UserDefaults.standard.object(forKey: "releaseUpdateLastCheck") as? Date ?? .distantPast
        if silent && Date().timeIntervalSince(lastCheck) < 12 * 60 * 60 { return }
        checkingRelease = true
        rebuildMenu()
        defer { checkingRelease = false; rebuildMenu() }
        do {
            let latest = try await ReleaseUpdater.latest()
            UserDefaults.standard.set(Date(), forKey: "releaseUpdateLastCheck")
            availableRelease = ReleaseUpdater.newer(latest) ? latest : nil
            if !silent && availableRelease == nil { alert("RoutePilot 已是最新版本", "当前版本：\(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "未知")") }
        } catch {
            if !silent { alert("检查更新失败", error.localizedDescription) }
        }
    }
    @objc func performReleaseUpdate() {
        Task {
            if availableRelease == nil { await checkReleaseUpdate(silent: false) }
            guard let release = availableRelease else { return }
            let dialog = NSAlert()
            dialog.messageText = L("在线升级 RoutePilot")
            dialog.informativeText = L("将下载并校验 Release {tag}，替换此 App；若已安装网络后台服务，也会在管理员授权后同步升级。数据库、密钥和配置保留。", ["tag": release.tagName])
            dialog.addButton(withTitle: L("下载并升级")); dialog.addButton(withTitle: L("取消"))
            NSApp.activate(ignoringOtherApps: true)
            guard dialog.runModal() == .alertFirstButtonReturn else { return }
            stopping = true
            do {
                let stagedApp = try await ReleaseUpdater.verifiedApp(from: release)
                defer { try? FileManager.default.removeItem(at: stagedApp.deletingLastPathComponent()) }
                if !(await stopChild()) { throw AppFailure(message: "后台进程尚未停止，升级已取消") }
                let helper = stagedApp.appendingPathComponent("Contents/Resources/Service/scripts/upgrade_app_macos.sh")
                guard FileManager.default.isReadableFile(atPath: helper.path) else { throw AppFailure(message: "升级包缺少 App 更新程序") }
                let command = "/bin/sh " + shellQuote(helper.path) + " " + shellQuote(stagedApp.path) + " " + shellQuote(Bundle.main.bundleURL.path)
                let escaped = command.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
                var failure: NSDictionary?
                guard NSAppleScript(source: "do shell script \"\(escaped)\" with administrator privileges")?.executeAndReturnError(&failure) != nil else {
                    throw AppFailure(message: failure?[NSAppleScript.errorMessage] as? String ?? "在线升级失败，现有安装已保留")
                }
                let launch = Process()
                launch.executableURL = URL(fileURLWithPath: "/usr/bin/open")
                launch.arguments = ["-n", Bundle.main.bundleURL.path]
                try launch.run()
                NSApp.terminate(nil)
            } catch {
                stopping = false
                alert("RoutePilot 未升级", error.localizedDescription)
                reconnect()
            }
        }
    }
    func menuWillOpen(_ menu: NSMenu) { Task { await refresh() } }
    @objc func changeMode(_ sender: NSMenuItem) {
        guard let mode = sender.representedObject as? String else { return }
        Task {
            do { _ = try await client.call("mode", method: "PUT", body: ["mode": mode]); await refresh() }
            catch { alert("模式未修改", error.localizedDescription) }
        }
    }
    @objc func flushCache() {
        Task { do { _ = try await client.call("dns/cache/flush", method: "POST", body: ["domain": ""]); await refresh() }
        catch { alert("缓存清理失败", error.localizedDescription) } }
    }
    @objc func importHosts() {
        Task {
            do {
                let result = try await client.call("dns/hosts/import-surge", method: "POST") as? [String: Any] ?? [:]
                let skipped = result["skipped"] as? [[String: Any]] ?? []
                let details = skipped.prefix(20).map { L("第 {line} 行：{reason}", ["line": $0["line"] ?? 0, "reason": L($0["reason"] as? String ?? "不支持")]) }.joined(separator: "\n")
                alert("Hosts 导入完成", L("导入 {imported} 项，跳过 {skipped} 项。手动条目保留且优先。", ["imported": result["imported"] ?? 0, "skipped": skipped.count]) + "\n" + details)
                await refresh()
            } catch { alert("Hosts 导入失败", error.localizedDescription) }
        }
    }
    @objc func signIn() {
        Task {
            do {
                _ = try await client.establish()
                let dialog = NSAlert(); dialog.messageText = L("登录 Controller"); dialog.informativeText = L("登录成功后，凭证将安全保存在 macOS 钥匙串中。")
                dialog.addButton(withTitle: L("登录")); dialog.addButton(withTitle: L("取消"))
                let fields = NSView(frame: NSRect(x: 0, y: 0, width: 300, height: 66))
                let saved = CredentialVault.load()
                let username = NSTextField(frame: NSRect(x: 0, y: 38, width: 300, height: 24)); username.stringValue = saved?.username ?? "admin"
                let password = NSSecureTextField(frame: NSRect(x: 0, y: 4, width: 300, height: 24)); password.placeholderString = L("密码"); password.stringValue = saved?.password ?? ""
                fields.addSubview(username); fields.addSubview(password); dialog.accessoryView = fields
                NSApp.activate(ignoringOtherApps: true)
                guard dialog.runModal() == .alertFirstButtonReturn else { return }
                defer { password.stringValue = "" }
                _ = try await client.call("login", method: "POST", body: ["username": username.stringValue, "password": password.stringValue])
                do { try CredentialVault.save(username: username.stringValue, password: password.stringValue) }
                catch { alert("登录成功，但凭证未保存", error.localizedDescription) }
                await refresh()
            } catch { alert("登录失败", error.localizedDescription) }
        }
    }
    @objc func openManagement() {
        if window == nil {
            let configuration = WKWebViewConfiguration()
            configuration.websiteDataStore = .nonPersistent()
            configuration.userContentController.add(self, name: "routepilotLocale")
            configuration.userContentController.add(self, name: "routepilotCredentials")
            configureLanguageSeed(configuration.userContentController)
            let view = WKWebView(frame: .zero, configuration: configuration)
            view.navigationDelegate = self
            let panel = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1150, height: 780), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
            panel.title = L("RoutePilot · DNS 和路由控制器")
            panel.minSize = NSSize(width: 420, height: 480)
            panel.contentView = view; panel.delegate = self; panel.isReleasedWhenClosed = false
            panel.center(); window = panel; webView = view
            let cookies = client.session.configuration.httpCookieStorage?.cookies(for: client.base) ?? []
            let group = DispatchGroup()
            for cookie in cookies { group.enter(); view.configuration.websiteDataStore.httpCookieStore.setCookie(cookie) { group.leave() } }
            group.notify(queue: .main) { view.load(URLRequest(url: self.client.base)) }
        }
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
        if navigationAction.shouldPerformDownload {
            let page = webView.url
            let trustedPage = page?.host == client.base.host && page?.port == client.base.port && page?.scheme == client.base.scheme
            if trustedPage && (url.scheme == "blob" || url.host == client.base.host && url.port == client.base.port && url.scheme == client.base.scheme) {
                decisionHandler(.download)
            } else { decisionHandler(.cancel) }
            return
        }
        // Never navigate the privileged control surface to an unrelated web origin.
        if url.scheme == client.base.scheme && url.host == client.base.host && url.port == client.base.port { decisionHandler(.allow); return }
        // A successful management setting change redirects to its new loopback listener.
        if navigationAction.targetFrame?.isMainFrame == true, let changed = validURL(url.absoluteString) {
            client = ControllerClient(changed)
            if !smoke { UserDefaults.standard.set(changed.absoluteString, forKey: "backendURL") }
            decisionHandler(.allow); return
        }
        decisionHandler(.cancel)
    }
    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { download.delegate = self }
    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { download.delegate = self }
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        if smoke, let path = argument("--report"), suggestedFilename == "routepilot-frr.conf" {
            let destination = URL(fileURLWithPath: path).deletingLastPathComponent().appendingPathComponent("native-frr-export.conf")
            try? FileManager.default.removeItem(at: destination)
            smokeDownloadURL = destination
            completionHandler(destination)
            return
        }
        let panel = NSSavePanel()
        panel.title = L("保存导出文件")
        panel.prompt = L("保存")
        panel.nameFieldStringValue = URL(fileURLWithPath: suggestedFilename).lastPathComponent
        panel.canCreateDirectories = true
        NSApp.activate(ignoringOtherApps: true)
        panel.begin { result in completionHandler(result == .OK ? panel.url : nil) }
    }
    func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
        if (error as NSError).code != NSURLErrorCancelled { alert("导出未完成", error.localizedDescription) }
    }
    @objc func toggleLogin() {
        do {
            if SMAppService.mainApp.status == .enabled { try SMAppService.mainApp.unregister() }
            else {
                try SMAppService.mainApp.register()
                if SMAppService.mainApp.status == .requiresApproval { SMAppService.openSystemSettingsLoginItems() }
            }
            rebuildMenu()
        } catch { alert("登录启动未修改", error.localizedDescription) }
    }
    @objc func changeAddress() {
        let dialog = NSAlert(); dialog.messageText = L("本机管理服务地址")
        dialog.addButton(withTitle: L("保存")); dialog.addButton(withTitle: L("取消"))
        let input = NSTextField(frame: NSRect(x: 0, y: 0, width: 340, height: 24)); input.stringValue = client.base.absoluteString
        dialog.accessoryView = input; NSApp.activate(ignoringOtherApps: true)
        guard dialog.runModal() == .alertFirstButtonReturn else { return }
        guard let url = validURL(input.stringValue) else { alert("地址无效", "请输入本机 loopback HTTP/HTTPS 地址。"); return }
        UserDefaults.standard.set(url.absoluteString, forKey: "backendURL")
        client = ControllerClient(url); window?.close(); window = nil; webView = nil
        reconnect()
    }
    @objc func reconnect() { bootFailures = 0; nextBoot = .distantPast; Task { await refresh() } }
    @objc func openLogs() {
        let url = child == nil && FileManager.default.fileExists(atPath: "/var/log/surge-controller") ? URL(fileURLWithPath: "/var/log/surge-controller") : dataDirectory
        NSWorkspace.shared.open(url)
    }
    func shellQuote(_ value: String) -> String { "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'" }
    @objc func installService() {
        guard !FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist") else { updateService(); return }
        guard child != nil else { alert("当前连接其他进程", "请先停止当前手动启动的 Controller，再通过 APP 启动并安装网络服务。"); return }
        let dialog = NSAlert(); dialog.messageText = L("安装网络后台服务")
        dialog.informativeText = L("透明 DNS/PF 需要管理员权限。将停止 APP 子进程，把当前数据库及密钥迁移到 /var/db/surge-controller，安装 launchd 服务。Surge 和网关配置由你自行切换。")
        dialog.addButton(withTitle: L("安装")); dialog.addButton(withTitle: L("取消"))
        NSApp.activate(ignoringOtherApps: true)
        guard dialog.runModal() == .alertFirstButtonReturn else { return }
        Task {
            stopping = true
            let stopped = await stopChild()
            guard stopped else { stopping = false; alert("安装已取消", "后台服务尚未停止；未迁移数据库。"); return }
            let stage: InstallerStage
            do {
                stage = try InstallerStage.prepare(service: Bundle.main.resourceURL!.appendingPathComponent("Service"), data: dataDirectory)
            } catch {
                stopping = false
                alert("网络服务未安装", L("准备迁移失败：{reason} 原配置已保留。", ["reason": L(error.localizedDescription)]))
                reconnect()
                return
            }
            defer { stage.remove() }
            let script = stage.service.appendingPathComponent("scripts/install_macos.sh")
            let command = "/bin/sh " + shellQuote(script.path) + " --data-directory " + shellQuote(stage.data.path)
            let escaped = command.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
            var failure: NSDictionary?
            let result = NSAppleScript(source: "do shell script \"\(escaped)\" with administrator privileges")?.executeAndReturnError(&failure)
            stopping = false
            if result == nil { alert("网络服务未安装", failure?[NSAppleScript.errorMessage] as? String ?? "安装失败，原始数据已保留。") }
            else {
                client = ControllerClient(client.base)
                UserDefaults.standard.set(client.base.absoluteString, forKey: "backendURL")
                window?.close(); window = nil; webView = nil
                alert("网络服务已安装", "服务随 macOS 启动；退出状态栏 APP 后仍继续运行。")
            }
            reconnect()
        }
    }
    func updateService() {
        guard child == nil else { alert("网络服务未更新", "APP 仍拥有用户后台进程，请先重新连接已安装的网络服务。"); return }
        let dialog = NSAlert(); dialog.messageText = L("更新网络后台服务")
        dialog.informativeText = L("将通过 macOS 管理员授权替换后台程序，短暂停止 DNS 服务并释放自己的 PF 规则。数据库、密钥和配置保留；启动后恢复已配置的透明拦截。")
        dialog.addButton(withTitle: L("更新")); dialog.addButton(withTitle: L("取消"))
        NSApp.activate(ignoringOtherApps: true)
        guard dialog.runModal() == .alertFirstButtonReturn else { return }
        stopping = true
        do {
            let stage = try InstallerStage.prepare(service: Bundle.main.resourceURL!.appendingPathComponent("Service"), data: nil)
            defer { stage.remove() }
            let command = "/bin/sh " + shellQuote(stage.service.appendingPathComponent("scripts/update_macos.sh").path)
            let escaped = command.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
            var failure: NSDictionary?
            let result = NSAppleScript(source: "do shell script \"\(escaped)\" with administrator privileges")?.executeAndReturnError(&failure)
            if result == nil { alert("网络服务未更新", failure?[NSAppleScript.errorMessage] as? String ?? "更新失败，原配置已保留。") }
            else {
                client = ControllerClient(client.base)
                window?.close(); window = nil; webView = nil
                alert("网络服务已更新", "后台程序已更新；原配置保留。")
            }
        } catch { alert("网络服务未更新", error.localizedDescription) }
        stopping = false
        reconnect()
    }
    func stopChild() async -> Bool {
        guard let process = child else { return true }
        process.terminate()
        for _ in 0..<200 {
            if !process.isRunning { return true }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return !process.isRunning
    }
    @objc func quit() { NSApp.terminate(nil) }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        timer?.invalidate()
        releaseTimer?.invalidate()
        guard let process = child, process.isRunning else { return .terminateNow }
        quitting = true; process.terminate()
        // Allow Go to drain DNS replies, withdraw BGP and release its PF lease.
        DispatchQueue.main.asyncAfter(deadline: .now() + 25) { [weak self] in
            guard let self = self, self.child?.isRunning == true else { return }
            self.quitting = false
            NSApp.reply(toApplicationShouldTerminate: false)
            self.alert("服务仍在停止", "为等待 DNS/BGP/PF 清理，APP 保持运行。可查看服务日志后重试退出。")
            self.timer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in Task { @MainActor in await self?.refresh() } }
        }
        return .terminateLater
    }
    func alert(_ title: String, _ text: String) {
        let dialog = NSAlert(); dialog.messageText = L(title); dialog.informativeText = L(text)
        dialog.addButton(withTitle: L("确定"))
        NSApp.activate(ignoringOtherApps: true); dialog.runModal()
    }
    func runSmoke() async {
        var report: [String: Any] = ["status_bar": statusItem.button != nil, "menu_items": statusItem.menu?.items.count ?? 0, "owned_backend": child != nil]
        do {
            guard child != nil else { throw AppFailure(message: "Smoke test requires isolated owned backend") }
            let initial = status["mode"] as? String ?? "auto"
            _ = try await client.call("mode", method: "PUT", body: ["mode": "force-direct"])
            let changed = try await client.call("mode") as? [String: Any]
            guard changed?["mode"] as? String == "force-direct" else { throw AppFailure(message: "Mode change failed") }
            _ = try await client.call("mode", method: "PUT", body: ["mode": initial])
            report["mode_change_and_restore"] = true
            _ = try await client.call("dns/cache/flush", method: "POST", body: ["domain": ""])
            report["cache_flush"] = true
            var settings = try await client.call("settings") as? [String: Any] ?? [:]
            let previousRouting = settings["routing"] as? [String: Any] ?? [:]
            settings["routing"] = ["mode": "static", "fake_ip_backend": "static", "dns_backend": "static", "static_fake_ip_configured": false, "static_dns_interception": false, "static_dns_configured": false]
            _ = try await client.call("settings", method: "PUT", body: settings)
            let staticView = try await client.call("routing/status") as? [String: Any] ?? [:]
            guard staticView["mode"] as? String == "static", staticView["enabled"] as? Bool == false,
                  let routes = staticView["routes"] as? [[String: Any]], let first = routes.first,
                  first["installed"] is NSNull, first["verified"] as? Bool == false else { throw AppFailure(message: "Static route declaration was reported as installed or verified") }
            report["static_routing_without_bgp"] = true
            settings["routing"] = ["mode": "hybrid", "fake_ip_backend": "static", "dns_backend": "bgp", "static_fake_ip_configured": false, "static_dns_interception": false, "static_dns_configured": false]
            _ = try await client.call("settings", method: "PUT", body: settings)
            let hybridView = try await client.call("routing/status") as? [String: Any] ?? [:]
            guard hybridView["fake_ip_backend"] as? String == "static", hybridView["dns_backend"] as? String == "bgp" else { throw AppFailure(message: "Hybrid backend selection failed") }
            report["hybrid_backend_selection"] = true
            settings["routing"] = previousRouting
            _ = try await client.call("settings", method: "PUT", body: settings)
            report["routing_mode_restore"] = true
            // Export against an isolated configuration; no router operations.
            var exportSettings = try await client.call("settings") as? [String: Any] ?? [:]
            let originalDNS = exportSettings["dns"] as? [String: Any] ?? [:]
            var exportDNS = originalDNS; exportDNS["source4"] = "192.0.2.2"
            exportSettings["dns"] = exportDNS
            _ = try await client.call("settings", method: "PUT", body: exportSettings)
            let exported = try await client.call("bgp/export-frr", method: "POST", body: ["gateway_asn": 65001, "gateway_router_id": "192.0.2.1", "controller_address": "192.0.2.2", "peer_id": ""]) as? [String: Any] ?? [:]
            guard let frr = exported["content"] as? String, frr.contains("router bgp 65001"), frr.contains("neighbor 192.0.2.2 remote-as") else { throw AppFailure(message: "FRR export failed") }
            exportSettings["dns"] = originalDNS
            _ = try await client.call("settings", method: "PUT", body: exportSettings)
            report["frr_export"] = true
            let created = try await client.call("dns/hosts", method: "POST", body: ["domain": "app-smoke.example", "match": "exact", "addresses": ["192.0.2.53"], "ttl": 120, "source": "manual", "enabled": true]) as? [String: Any]
            let hosts = created?["hosts"] as? [[String: Any]] ?? []
            guard let id = hosts.first(where: { $0["domain"] as? String == "app-smoke.example" })?["id"] as? String else { throw AppFailure(message: "Host CRUD failed") }
            _ = try await client.call("dns/hosts/" + id, method: "DELETE")
            report["local_dns_crud"] = true
            let imported = try await client.call("dns/hosts/import-surge", method: "POST") as? [String: Any] ?? [:]
            report["surge_hosts_import"] = imported
            openManagement()
            try await Task.sleep(nanoseconds: 3_000_000_000)
            let webTitle = try await webView?.evaluateJavaScript("document.title") as? String ?? ""
            let webText = try await webView?.evaluateJavaScript("document.body.innerText") as? String ?? ""
            guard webText.contains(L("仪表盘")) else { throw AppFailure(message: "Embedded management UI did not render") }
            report["webview_title"] = webTitle
            let parameters = String(data: try JSONSerialization.data(withJSONObject: [frr, "routepilot-frr.conf"]), encoding: .utf8)!
            _ = try await webView?.evaluateJavaScript("(()=>{const [content,name]=\(parameters);const a=document.createElement('a');a.href=URL.createObjectURL(new Blob([content],{type:'text/plain'}));a.download=name;document.body.appendChild(a);a.click();a.remove();})()")
            for _ in 0..<50 {
                if let file = smokeDownloadURL, (try? String(contentsOf: file, encoding: .utf8)) == frr { break }
                try await Task.sleep(nanoseconds: 100_000_000)
            }
            guard let file = smokeDownloadURL, (try? String(contentsOf: file, encoding: .utf8)) == frr else { throw AppFailure(message: "Native FRR download did not save exact content") }
            report["native_frr_download"] = true
            window?.close()
            try await Task.sleep(nanoseconds: 500_000_000)
            guard child?.isRunning == true && applicationShouldTerminateAfterLastWindowClosed(NSApp) == false else { throw AppFailure(message: "Closing window stopped background app") }
            report["background_after_window_close"] = true
            stopping = true
            report["graceful_shutdown"] = await stopChild()
            report["passed"] = report["graceful_shutdown"] as? Bool == true
            finishSmoke(report)
        } catch { report["passed"] = false; report["error"] = error.localizedDescription; finishSmoke(report) }
    }
    func finishSmoke(_ report: [String: Any]) {
        if let path = argument("--report"), let data = try? JSONSerialization.data(withJSONObject: report, options: [.prettyPrinted, .sortedKeys]) { try? data.write(to: URL(fileURLWithPath: path), options: .atomic) }
        NSApp.terminate(nil)
    }
}

@main
struct RoutePilot {
    @MainActor static func main() {
        let application = NSApplication.shared
        let delegate = AppDelegate()
        application.delegate = delegate
        application.run()
        withExtendedLifetime(delegate) {}
    }
}
