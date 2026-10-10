import AppKit
import WebKit
import ServiceManagement
import Darwin

struct AppFailure: LocalizedError {
    let message: String
    let statusCode: Int?
    init(message: String, statusCode: Int? = nil) {
        self.message = message
        self.statusCode = statusCode
    }
    var errorDescription: String? { message }
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
final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate, NSMenuDelegate, WKScriptMessageHandler {
    let arguments = CommandLine.arguments
    var client: ControllerClient!
    var statusItem: NSStatusItem!
    var statusMenuOpen = false
    var menuBrandImage: NSImage?
    var statusIconAnimator: StatusBarIconAnimator?
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
	let serviceNotifications = ServiceNotifications()
	var notificationSettings: [String: Any] = [:]
    var notificationHealth: [[String: Any]] = []
	var backendFailures = 0
    var remoteNotificationWatch: RemoteNotificationWatch?
    var remoteNotificationGeneration = 0
    var refreshingRemoteNotifications = false
    var lastError = "正在连接…"
    var versionWarning: String?
    var lastVersionWarningSignature: String?
    var smokeStarted = false
    var smokeDownloadURL: URL?
    var automationError = ""
    var smoke: Bool { arguments.contains("--smoke-test") }
    func installApplicationMenu() {
        let mainMenu = NSMenu()
        let appItem = NSMenuItem(title: "RoutePilot", action: nil, keyEquivalent: "")
        let appMenu = NSMenu(title: "RoutePilot")
        appMenu.addItem(withTitle: L("隐藏 RoutePilot"), action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        let hideOthers = appMenu.addItem(withTitle: L("隐藏其他应用"), action: #selector(NSApplication.hideOtherApplications(_:)), keyEquivalent: "h")
        hideOthers.keyEquivalentModifierMask = [.command, .option]
        appMenu.addItem(withTitle: L("显示全部"), action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        let quitItem = appMenu.addItem(withTitle: L("退出 RoutePilot"), action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        quitItem.target = NSApp
        appItem.submenu = appMenu
        mainMenu.addItem(appItem)
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
        let windowItem = NSMenuItem(title: L("窗口"), action: nil, keyEquivalent: "")
        let windowMenu = NSMenu(title: L("窗口"))
        windowMenu.addItem(withTitle: L("最小化"), action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
        windowMenu.addItem(withTitle: L("缩放"), action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
        windowMenu.addItem(withTitle: L("关闭窗口"), action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")
        let fullScreen = windowMenu.addItem(withTitle: L("切换全屏"), action: #selector(NSWindow.toggleFullScreen(_:)), keyEquivalent: "f")
        fullScreen.keyEquivalentModifierMask = [.command, .control]
        windowItem.submenu = windowMenu
        mainMenu.addItem(windowItem)
        NSApp.mainMenu = mainMenu
        NSApp.windowsMenu = windowMenu
    }
    func argument(_ key: String) -> String? {
        guard let i = arguments.firstIndex(of: key), i + 1 < arguments.count else { return nil }
        return arguments[i + 1]
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        DesktopNotifications.shared.prepare()
        installApplicationMenu()
        if let icon = Bundle.main.url(forResource: "AppIcon", withExtension: "icns") { NSApp.applicationIconImage = NSImage(contentsOf: icon) }
        let stored = argument("--backend-url") ?? UserDefaults.standard.string(forKey: "backendURL") ?? "http://127.0.0.1:1055"
        guard let url = validURL(stored) else { alert("管理地址无效", "仅支持本机 loopback HTTP/HTTPS 地址。"); NSApp.terminate(nil); return }
        client = ControllerClient(url)
        dataDirectory = argument("--data-directory").map { URL(fileURLWithPath: $0) } ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("RoutePilot")
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem.button {
            button.title = ""
            button.imagePosition = .imageOnly
            menuBrandImage = Bundle.main.url(forResource: "MenuIcon", withExtension: "png").flatMap { NSImage(contentsOf: $0) }
            button.image = menuBrandImage
            button.image?.size = NSSize(width: 18, height: 18)
            button.image?.isTemplate = true
            button.toolTip = "RoutePilot · " + L("正在连接…")
            statusIconAnimator = StatusBarIconAnimator(button: button, brand: menuBrandImage)
        }
        if !smoke {
            do { try AutomationIntegration.apply(AppPreferences.current.automation, startup: true) }
            catch is AutomationConflict { automationError = L("检测到 CLI 或 skill 文件冲突，请到设置中选择覆盖或跳过。") }
            catch { automationError = error.localizedDescription }
        }
        rebuildMenu()
        timer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
            Task { @MainActor in await self?.refreshRemoteNotifications() }
        }
        Task { await refresh() }
        Task { await checkReleaseUpdate(silent: true) }
        releaseTimer = Timer.scheduledTimer(withTimeInterval: 60 * 60, repeats: true) { [weak self] _ in
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
            authenticated = false; status = [:]; lastError = connectionError; versionWarning = nil
            backendFailures += 1
            if backendFailures >= 3 {
                serviceNotifications.observe(scope: client.base.absoluteString, name: L("本地服务"), health: notificationHealth + [["name": "Management connection", "state": "DOWN"]], settings: notificationSettings)
            }
            if child == nil && !starting && bootFailures < 5 && Date() >= nextBoot && client.base.scheme == "http" && (smoke || !FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist")) {
                startBackend()
            }
        }
        updateToolTip()
        rebuildMenu()
    }
    func refreshFromBackend() async throws {
        authenticated = try await client.establish()
        let credentialStorage = CredentialVault.storage
        if !authenticated, let credential = CredentialVault.load(from: credentialStorage) {
            do {
                _ = try await client.call("login", method: "POST", body: ["username": credential.username, "password": credential.password])
                authenticated = try await client.establish()
            } catch let failure as AppFailure where failure.statusCode == 401 {
                CredentialVault.delete(from: credentialStorage)
                throw failure
            }
        }
        if authenticated {
            status = try await client.call("status") as? [String: Any] ?? [:]
            health = try await client.call("health") as? [[String: Any]] ?? []
            bgp = try await client.call("bgp/status") as? [String: Any] ?? [:]
            let settings = try await client.call("settings") as? [String: Any] ?? [:]
            notificationSettings = settings["notifications"] as? [String: Any] ?? [:]
            backendFailures = 0
            var activeHealth = health
            let peerAddresses = Set((bgp["peers"] as? [[String: Any]] ?? []).compactMap { $0["address"] as? String })
            activeHealth.removeAll { component in
                guard let name = component["name"] as? String, name.hasPrefix("BGP peer ") else { return false }
                return !peerAddresses.contains(String(name.dropFirst("BGP peer ".count)))
            }
            notificationHealth = activeHealth
            activeHealth.append(["name": "Management connection", "state": "UP"])
            serviceNotifications.observe(scope: client.base.absoluteString, name: status["name"] as? String ?? L("本地服务"), health: activeHealth, settings: notificationSettings)
            lastError = ""
            checkBackendVersion()
            if smoke && !smokeStarted {
                smokeStarted = true
                Task { await runSmoke() }
            }
        } else {
            status = [:]; lastError = "需要登录管理服务"; versionWarning = nil
        }
    }
    func checkBackendVersion() {
        let software = status["software"] as? [String: Any] ?? [:]
        func identity(_ value: Any?) -> String {
            let raw = value as? String ?? (value as? NSNumber)?.stringValue ?? ""
            return raw.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        func version(_ value: String) -> String {
            value.hasPrefix("v") ? String(value.dropFirst()) : value
        }
        let appVersion = identity(Bundle.main.infoDictionary?["CFBundleShortVersionString"])
        let appBuild = identity(Bundle.main.infoDictionary?["CFBundleVersion"])
        let backendVersion = identity(software["version"])
        let backendBuild = identity(software["build"])
        let complete = !backendVersion.isEmpty && !backendBuild.isEmpty
        if complete && version(appVersion) == version(backendVersion) && appBuild == backendBuild {
            versionWarning = nil
            lastVersionWarningSignature = nil
            return
        }
        let title = complete ? "App 与后台版本不一致" : "后台版本无法确认"
        versionWarning = title
        let signature = [client.base.absoluteString, appVersion, appBuild, backendVersion, backendBuild].joined(separator: "|")
        guard !smoke, lastVersionWarningSignature != signature else { return }
        lastVersionWarningSignature = signature
        let appInfo = L("App：{version}（构建 {build}）", ["version": appVersion, "build": appBuild])
        let backendInfo = complete
            ? L("后台：{version}（构建 {build}）", ["version": backendVersion, "build": backendBuild])
            : L("后台未提供完整的版本和构建编号，请更新后台服务。")
        alert(title, appInfo + "\n" + backendInfo + "\n\n" + L("请将 App 和后台更新到同一发布版本。在“设置 → 软件更新”中在线升级，会同步更新 App 和已安装的本机网络后台。"))
    }
    var serviceVisualState: StatusBarVisualState {
        let runtimeState = status["state"] as? String ?? ""
        let applying = (status["configuration_application"] as? [String: Any])?["state"] as? String ?? ""
        if ["pending", "applying"].contains(applying) { return .connecting }
        if stopping || starting || lastError == "正在连接…" || ["STARTING", "RECOVERING", "STOPPING"].contains(runtimeState) { return .connecting }
        if !authenticated { return lastError == "需要登录管理服务" ? .login : .offline }
        if !lastError.isEmpty { return .offline }
        let down = notificationHealth.contains { ($0["state"] as? String)?.uppercased() == "DOWN" }
        let intentionalDirect = status["mode"] as? String == "force-direct" && runtimeState == "DIRECT_FALLBACK"
        if down || applying == "failed" || versionWarning != nil || runtimeState != "HEALTHY" && !intentionalDirect { return .warning }
        return .healthy
    }
    func updateToolTip() {
        let visual = serviceVisualState
        let preference = AppPreferences.current.statusBar
        statusIconAnimator?.configure(animation: preference.animation, queriesPerSecond: (status["queries_per_second"] as? NSNumber)?.doubleValue ?? 0)
        let caption = preference.text(status: status, health: health, bgp: bgp, available: authenticated && lastError.isEmpty)
        statusItem.length = caption.isEmpty ? NSStatusItem.squareLength : NSStatusItem.variableLength
        statusItem.button?.title = caption.isEmpty ? "" : " " + caption
        statusItem.button?.font = NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .regular)
        statusItem.button?.imagePosition = .imageLeading
        let counters = status["counters"] as? [String: Any] ?? [:]
        statusIconAnimator?.setTraffic(direct: (counters["Direct"] as? NSNumber)?.doubleValue ?? 0,
                                      surge: (counters["Surge"] as? NSNumber)?.doubleValue ?? 0)
        statusIconAnimator?.setState(visual)
        let summary = "RoutePilot · " + L(visual.label)
        statusItem.button?.setAccessibilityLabel(summary)
        statusItem.button?.toolTip = summary + "\n" + (lastError.isEmpty ? stateText(status["state"] ?? "—") + " · " + stateText(status["mode"] ?? "auto") : L(lastError))
        let down = notificationHealth.filter { ($0["state"] as? String)?.uppercased() == "DOWN" }.compactMap { $0["name"] as? String }
        if authenticated && !down.isEmpty { statusItem.button?.toolTip? += "\n" + down.map { L($0) }.joined(separator: "、") }
        if visual == .healthy { statusItem.button?.toolTip? += "\n" + L("分流动画：上支 DIRECT，下支 SURGE") }
        if !caption.isEmpty { statusItem.button?.toolTip? += "\n" + (preference.display == "metric" ? L(["queries_per_second":"每秒 DNS 请求数", "queries":"DNS 请求总数", "direct":"DIRECT 请求总数", "surge":"SURGE 请求总数", "reject":"REJECT 请求总数", "errors":"DNS 错误总数", "bgp_advertised":"BGP 已发布路由数", "uptime_seconds":"后台运行秒数"][preference.metric] ?? "状态栏指标") : preference.health) + " · " + caption }
        if let versionWarning { statusItem.button?.toolTip? += "\n" + L(versionWarning) }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        let origin = message.frameInfo.securityOrigin
        let trusted = message.frameInfo.isMainFrame && origin.protocol == client.base.scheme && origin.host == client.base.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")) && origin.port == client.base.port
        if message.name == "routepilotPreferences" {
            guard trusted, let values = message.body as? [String: Any] else { return }
            func reply(_ ok: Bool, extra: [String: Any] = [:]) {
                guard let id = values["_requestID"] as? String, id.count <= 64,
                      let data = try? JSONSerialization.data(withJSONObject: extra.merging(["requestID": id, "ok": ok]) { _, value in value }),
                      let json = String(data: data, encoding: .utf8) else { return }
                webView?.evaluateJavaScript("window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:" + json + "}));", completionHandler: nil)
            }
            guard let data = try? JSONSerialization.data(withJSONObject: values),
                  var preferences = try? JSONDecoder().decode(AppPreferences.self, from: data), (1...168).contains(preferences.checkIntervalHours), preferences.statusBar.valid, preferences.automation.valid else { reply(false); return }
            preferences.automaticInstall = preferences.automaticInstall && preferences.automaticCheck && UpdateAssistant.configured
            let previous = AppPreferences.current
            if preferences.automation != previous.automation || values["_automation"] as? Bool == true {
                let choices = values["_automationChoices"] as? [String: String] ?? [:]
                guard choices.keys.allSatisfy({ ["cli", "codex", "claude"].contains($0) }), choices.values.allSatisfy({ ["overwrite", "skip"].contains($0) }) else { reply(false); return }
                do { try AutomationIntegration.apply(preferences.automation, choices: choices); automationError = "" }
                catch let conflict as AutomationConflict {
                    reply(false, extra: ["code": "automation_conflict", "conflicts": conflict.items, "error": L("检测到同名文件，请选择覆盖或跳过。")]); return
                } catch {
                    automationError = error.localizedDescription
                    configurePreferenceSeed(userContentController)
                    reply(false, extra: ["error": automationError]); return
                }
            }
            AppPreferences.current = preferences
            updateToolTip()
            if preferences.automaticCheck && (!previous.automaticCheck || preferences.automaticDownload != previous.automaticDownload || preferences.updatePrompts != previous.updatePrompts || preferences.automaticInstall != previous.automaticInstall) {
                UserDefaults.standard.removeObject(forKey: "releaseUpdateLastCheck")
            }
            configurePreferenceSeed(userContentController)
            reply(true)
            Task {
                if preferences.desktopNotifications && !previous.desktopNotifications {
                    if !(await DesktopNotifications.shared.authorize()) { alert("通知未授权", "请在 macOS 系统设置 → 通知 → RoutePilot 中允许通知。") }
                    serviceNotifications.reset()
                }
                await checkReleaseUpdate(silent: true)
            }
            return
        }
        if message.name == "routepilotUpdateAction" {
            guard trusted, let action = message.body as? String else { return }
            if action == "check" { Task { await checkReleaseUpdate(silent: false) } }
            else if action == "install" { performReleaseUpdate() }
            else if action == "enableAssistant" { enableUpdateAssistant() }
            else if action == "disableAssistant" { disableUpdateAssistant() }
            return
        }
        if message.name == "routepilotServiceHealth" {
            guard trusted, let values = message.body as? [String: Any] else { return }
            if values["clear"] as? Bool == true { remoteNotificationGeneration += 1; remoteNotificationWatch = nil; return }
            guard let scope = values["scope"] as? String, scope.count <= 512,
                  let name = values["name"] as? String, name.count <= 100,
                  let health = values["health"] as? [[String: Any]], let settings = values["settings"] as? [String: Any] else { return }
            serviceNotifications.observe(scope: scope, name: name, health: health, settings: settings)
            if let id = values["backendID"] as? String, !id.isEmpty, id.count <= 64, id.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "_" || $0 == "-") }), let webView {
                if let watch = remoteNotificationWatch, watch.id == id && watch.scope == scope {
                    watch.settings = settings; watch.health = health.filter { $0["name"] as? String != "Management connection" }
                } else {
                    let base = client.base
                    remoteNotificationGeneration += 1
                    let generation = remoteNotificationGeneration
                    webView.configuration.websiteDataStore.httpCookieStore.getAllCookies { [weak self] cookies in
                        Task { @MainActor in
                            guard let self, self.client.base == base, self.remoteNotificationGeneration == generation,
                                  let cookie = cookies.first(where: { $0.name == "controller_session" && $0.domain.trimmingCharacters(in: CharacterSet(charactersIn: ".[]")) == base.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")) }) else { return }
                            self.remoteNotificationWatch = RemoteNotificationWatch(id: id, scope: scope, name: name, base: base, cookie: cookie, settings: settings)
                            self.remoteNotificationWatch?.health = health.filter { $0["name"] as? String != "Management connection" }
                        }
                    }
                }
            }
            return
        }
        if message.name == "routepilotCredentials" {
            if trusted, let values = message.body as? [String: String], values["action"] != nil {
                Task { await handleRemoteCredentials(values) }
                return
            }
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
        installApplicationMenu()
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
    func configurePreferenceSeed(_ controller: WKUserContentController) {
        let marker = "/* RoutePilot preferences */"
        let scripts = controller.userScripts.filter { !$0.source.hasPrefix(marker) }
        controller.removeAllUserScripts()
        scripts.forEach { controller.addUserScript($0) }
        guard let data = try? JSONEncoder().encode(AppPreferences.current),
              var preferences = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
        preferences["statusBarHealth"] = health.compactMap { $0["name"] as? String }
        preferences["assistantReady"] = UpdateAssistant.configured
        preferences["assistantSupported"] = true
        preferences["remoteCredentialsSupported"] = true
        preferences["preferencesSaveAcknowledgement"] = true
        preferences["automationSupported"] = AutomationIntegration.supported
        preferences["automationStatus"] = AutomationIntegration.status().merging(["error": automationError]) { _, value in value }
        guard let seed = try? JSONSerialization.data(withJSONObject: preferences), let json = String(data: seed, encoding: .utf8) else { return }
        let source = marker + "window.routepilotNativePreferences=" + json + ";window.dispatchEvent(new Event('routepilot-preferences-changed'));"
        controller.addUserScript(WKUserScript(source: source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        webView?.evaluateJavaScript(source, completionHandler: nil)
    }
    func refreshRemoteNotifications() async {
        guard !refreshingRemoteNotifications, !quitting, !stopping, AppPreferences.current.desktopNotifications,
              let watch = remoteNotificationWatch else { return }
        if window?.isVisible == true && window?.isMiniaturized != true { return }
        refreshingRemoteNotifications = true
        defer { refreshingRemoteNotifications = false }
        let path = "management/relay/" + watch.id + "/"
        do {
            let health = try await watch.client.call(path + "health") as? [[String: Any]] ?? []
            let settings = try await watch.client.call(path + "settings") as? [String: Any] ?? [:]
            let peers = try await watch.client.call(path + "bgp/peers") as? [[String: Any]] ?? []
            guard remoteNotificationWatch === watch else { return }
            let routing = settings["routing"] as? [String: Any] ?? [:]
            let mode = routing["mode"] as? String ?? "bgp"
            let usesBGP = mode == "bgp" || mode == "hybrid" && (routing["fake_ip_backend"] as? String == "bgp" || routing["dns_backend"] as? String == "bgp")
            let bgpEnabled = usesBGP && (settings["bgp"] as? [String: Any])?["enabled"] as? Bool == true
            let active = Set(peers.filter { bgpEnabled && $0["enabled"] as? Bool == true }.compactMap { ($0["address"] as? String).map { "BGP peer " + $0 } })
            watch.health = health.filter { !($0["name"] as? String ?? "").hasPrefix("BGP peer ") || active.contains($0["name"] as? String ?? "") }
            watch.settings = settings["notifications"] as? [String: Any] ?? [:]
            watch.failures = 0
            serviceNotifications.observe(scope: watch.scope, name: watch.name, health: watch.health + [["name": "Management connection", "state": "UP"]], settings: watch.settings)
        } catch let failure as AppFailure where failure.statusCode == 401 || failure.statusCode == 403 || failure.statusCode == 404 {
            if remoteNotificationWatch === watch { remoteNotificationWatch = nil }
        } catch {
            guard remoteNotificationWatch === watch else { return }
            watch.failures += 1
            if watch.failures >= 3 {
                serviceNotifications.observe(scope: watch.scope, name: watch.name, health: watch.health + [["name": "Management connection", "state": "DOWN"]], settings: watch.settings)
            }
        }
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
    func submenu(_ title: String, items: [NSMenuItem], enabled: Bool = true) -> NSMenuItem {
        let row = item(title, enabled: enabled)
        let children = NSMenu(); children.autoenablesItems = false
        items.forEach { children.addItem($0) }
        row.submenu = children
        return row
    }
    func rebuildMenu() {
        updateToolTip()
        // Keep menu items stable while the user is choosing an action.
        guard !statusMenuOpen else { return }
        let menu = NSMenu(); menu.autoenablesItems = false; menu.delegate = self
        let connected = authenticated && lastError.isEmpty
        let controlsEnabled = connected && !stopping
        let needsLogin = !authenticated && lastError == "需要登录管理服务"
        let summary = L(serviceVisualState.label)
        menu.addItem(item("RoutePilot · " + summary, enabled: false))
        menu.addItem(item("打开管理窗口", action: #selector(openManagement)))

        var details = [item(L("本机后台：{address}", ["address": client?.base.absoluteString ?? "—"]), enabled: false)]
        if connected {
            details.append(item(L("Controller：{state}", ["state": stateText(status["state"] ?? "—")]), enabled: false))
            let surge = health.first { $0["name"] as? String == "Surge CLI" }?["state"] ?? "—"
            details.append(item(L("Surge：{state} · Fake-IP：{enabled}", ["state": stateText(surge), "enabled": L((status["fake_ip_enabled"] as? Bool ?? false) ? "启用" : "禁用")]), enabled: false))
            let peers = bgp["peers"] as? [[String: Any]] ?? []
            let up = peers.filter { ($0["state"] as? String)?.uppercased() == "ESTABLISHED" }.count
            details.append(item(L("BGP：{up}/{total} 邻居 · {routes} 路由", ["up": up, "total": peers.count, "routes": bgp["advertised"] ?? 0]), enabled: false))
            details.append(item(L("路由：{mode} · Fake-IP {fake} / DNS {dns}", ["mode": stateText(bgp["mode"] ?? "bgp"), "fake": stateText(bgp["fake_ip_backend"] ?? "bgp"), "dns": stateText(bgp["dns_backend"] ?? "bgp")]), enabled: false))
            details.append(item(L("查询：{count} / 分钟", ["count": status["queries_per_minute"] ?? 0]), enabled: false))
        } else {
            let error = L(lastError)
            let row = item(String(error.prefix(90)), enabled: false); row.toolTip = error
            details.append(row)
        }
        if let versionWarning { details.append(item(versionWarning, enabled: false)) }
        menu.addItem(submenu("本机服务状态", items: details))
        menu.addItem(.separator())

        var modes: [NSMenuItem] = []
        for (value, label) in [("auto", "自动"), ("force-direct", "强制 DIRECT"), ("force-surge", "强制 SURGE")] {
            let row = item(label, action: #selector(changeMode(_:)), enabled: controlsEnabled)
            row.representedObject = value
            row.state = status["mode"] as? String == value ? .on : .off
            modes.append(row)
        }
        menu.addItem(submenu("本机 DNS", items: [
            submenu("DNS 模式", items: modes, enabled: controlsEnabled),
            item("清理决策与响应缓存", action: #selector(flushCache), enabled: controlsEnabled),
            item("从 Surge 导入 Hosts…", action: #selector(importHosts), enabled: controlsEnabled)
        ], enabled: controlsEnabled))

        let serviceInstalled = FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist")
        var serviceItems: [NSMenuItem] = []
        if needsLogin { serviceItems.append(item("登录本机后台…", action: #selector(signIn), enabled: !stopping)); serviceItems.append(.separator()) }
        if !serviceInstalled { serviceItems.append(item("安装网络后台服务…", action: #selector(installService), enabled: !stopping && !smoke && child != nil)) }
        serviceItems.append(item("重新连接", action: #selector(reconnect), enabled: !stopping))
        serviceItems.append(item("打开服务日志", action: #selector(openLogs)))
        menu.addItem(submenu("本机后台", items: serviceItems))

        menu.addItem(item(checkingRelease ? "正在检查更新…" : "检查更新", action: #selector(performReleaseUpdate), enabled: !checkingRelease && !stopping))

        let login = item(SMAppService.mainApp.status == .requiresApproval ? "登录时启动（等待系统授权）" : "登录时启动", action: #selector(toggleLogin), enabled: !smoke)
        login.state = SMAppService.mainApp.status == .enabled ? .on : SMAppService.mainApp.status == .requiresApproval ? .mixed : .off
        menu.addItem(login)
        let stores = CredentialStorage.allCases.map { storage -> NSMenuItem in
            let row = item(storage.title, action: #selector(changeCredentialStorage(_:)))
            row.representedObject = storage.rawValue
            row.state = CredentialVault.storage == storage ? .on : .off
            return row
        }
        menu.addItem(submenu("应用偏好", items: [submenu("登录凭证保存方式", items: stores)]))
        menu.addItem(.separator())
        let quit = item(child == nil ? "退出 RoutePilot" : "退出 RoutePilot（停止临时后台）", action: #selector(quit))
        quit.toolTip = L(child == nil ? "后台独立运行，不会随 App 退出。" : "同时停止 App 启动的临时后台。")
        menu.addItem(quit)
        statusItem.menu = menu
    }
    func checkReleaseUpdate(silent: Bool) async {
        guard !checkingRelease, !stopping else { return }
        let preferences = AppPreferences.current
        if silent && !preferences.automaticCheck { return }
        let lastCheck = UserDefaults.standard.object(forKey: "releaseUpdateLastCheck") as? Date ?? .distantPast
        if silent && Date().timeIntervalSince(lastCheck) < Double(preferences.checkIntervalHours * 3600) { return }
        checkingRelease = true
        rebuildMenu()
        defer { checkingRelease = false; rebuildMenu() }
        do {
            let latest = try await ReleaseUpdater.latest()
            UserDefaults.standard.set(Date(), forKey: "releaseUpdateLastCheck")
            let appVersion = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
            let backendVersion = (status["software"] as? [String: Any])?["version"] as? String ?? ""
            let repairLocalService = child == nil && FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist") && ReleaseUpdater.needsBackendUpdate(latest, app: appVersion, backend: backendVersion)
            availableRelease = ReleaseUpdater.newer(latest) || repairLocalService ? latest : nil
            guard let release = availableRelease else {
                if !silent { alert("RoutePilot 已是最新版本", "当前版本：\(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "未知")") }
                return
            }
            if silent && UserDefaults.standard.string(forKey: "releaseUpdateSkippedTag") == release.tagName { return }
            if silent && preferences.automaticInstall && UpdateAssistant.configured {
                await installRelease(release)
                return
            }
            if silent && preferences.automaticDownload {
                do { _ = try await ReleaseUpdater.downloadPackage(from: release) }
                catch { if preferences.updatePrompts { alert("自动下载更新失败", error.localizedDescription) } }
            }
            if !silent || preferences.updatePrompts && UserDefaults.standard.string(forKey: "releaseUpdateLastNotifiedTag") != release.tagName {
                UserDefaults.standard.set(release.tagName, forKey: "releaseUpdateLastNotifiedTag")
                await presentRelease(release)
            }
        } catch {
            if !silent { alert("检查更新失败", error.localizedDescription) }
        }
    }
    func presentRelease(_ release: GiteaRelease) async {
        let dialog = ReleaseUpdateDialog(release: release, assistantReady: UpdateAssistant.configured)
        switch await dialog.run() {
        case .install:
            UserDefaults.standard.removeObject(forKey: "releaseUpdateSkippedTag")
            await installRelease(release)
        case .skip:
            UserDefaults.standard.set(release.tagName, forKey: "releaseUpdateSkippedTag")
        case .later: break
        }
    }
    @objc func performReleaseUpdate() {
        // Both entry points perform a fresh check and open exactly one decision window.
        Task { await checkReleaseUpdate(silent: false) }
    }
    @objc func enableUpdateAssistant() {
        guard !stopping, !checkingRelease else { return }
        let dialog = NSAlert()
        dialog.messageText = L("启用更新助手")
        dialog.informativeText = L("一次管理员授权后，当前用户可免密码更新 /Applications 中的 RoutePilot 和本机网络服务。助手只安装项目官方 Release，不接受任意命令；自动安装默认关闭。")
        dialog.addButton(withTitle: L("启用")); dialog.addButton(withTitle: L("取消"))
        NSApp.activate(ignoringOtherApps: true)
        guard dialog.runModal() == .alertFirstButtonReturn else { return }
        stopping = true
        Task {
            defer { stopping = false; rebuildMenu() }
            do { try await UpdateAssistant.enable(); if let controller = webView?.configuration.userContentController { configurePreferenceSeed(controller) } }
            catch { alert("更新助手未启用", error.localizedDescription) }
        }
    }
    @objc func disableUpdateAssistant() {
        guard !stopping, !checkingRelease else { return }
        let dialog = NSAlert()
        dialog.messageText = L("停用更新助手")
        dialog.informativeText = L("将移除常驻更新助手并关闭自动安装，保留 App 和网络服务。移除助手需要管理员授权。")
        dialog.addButton(withTitle: L("停用")); dialog.addButton(withTitle: L("取消"))
        NSApp.activate(ignoringOtherApps: true)
        guard dialog.runModal() == .alertFirstButtonReturn else { return }
        stopping = true
        Task {
            defer { stopping = false; rebuildMenu() }
            do {
                try await UpdateAssistant.disable()
                var preferences = AppPreferences.current
                preferences.automaticInstall = false
                AppPreferences.current = preferences
                if let controller = webView?.configuration.userContentController { configurePreferenceSeed(controller) }
            } catch { alert("更新助手未停用", error.localizedDescription) }
        }
    }
    func installRelease(_ release: GiteaRelease) async {
        guard !stopping else { return }
        stopping = true
        rebuildMenu()
        let progress = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 430, height: 100), styleMask: [.titled], backing: .buffered, defer: false)
        progress.title = L("正在升级 RoutePilot")
        let label = NSTextField(wrappingLabelWithString: L("正在准备更新…"))
        label.frame = NSRect(x: 24, y: 40, width: 382, height: 40)
        progress.contentView?.addSubview(label)
        progress.center(); progress.makeKeyAndOrderFront(nil)
        defer { progress.close() }
        do {
            if !UpdateAssistant.configured { try await UpdateAssistant.enable() }
            if let controller = webView?.configuration.userContentController { configurePreferenceSeed(controller) }
            _ = try await UpdateAssistant.request("check")
            if !(await stopChild()) { throw AppFailure(message: "后台进程尚未停止，升级已取消") }
            var status = try await UpdateAssistant.request("install", version: release.tagName)
            let deadline = Date().addingTimeInterval(10 * 60)
            var failures = 0
            while status.phase != "succeeded" && status.phase != "failed" {
                guard Date() < deadline else { throw AppFailure(message: "等待更新超时，请重新打开 App 查看安装状态") }
                let phases = ["downloading": "正在下载并校验安装包…", "validating": "正在验证更新…", "restarting": "正在重启本机网络服务…"]
                label.stringValue = L(phases[status.phase ?? ""] ?? "正在替换 App…")
                try await Task.sleep(nanoseconds: 1_000_000_000)
                do { status = try await UpdateAssistant.request("status"); failures = 0 }
                catch { failures += 1; if failures >= 3 { throw error } }
            }
            guard status.phase == "succeeded", status.target == release.tagName else { throw AppFailure(message: L(status.error ?? "在线升级失败，现有安装已保留")) }
            _ = try? await UpdateAssistant.request("refresh-agent")
            let launch = Process()
            launch.executableURL = URL(fileURLWithPath: "/usr/bin/open")
            launch.arguments = ["-n", UpdateAssistant.appPath]
            try launch.run()
            NSApp.terminate(nil)
        } catch {
            stopping = false
            alert("RoutePilot 未升级", error.localizedDescription)
            reconnect()
            rebuildMenu()
        }
    }
    func menuWillOpen(_ menu: NSMenu) { statusMenuOpen = true; Task { await refresh() } }
    func menuDidClose(_ menu: NSMenu) { statusMenuOpen = false; rebuildMenu() }
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
    @objc func changeCredentialStorage(_ sender: NSMenuItem) {
        guard let value = sender.representedObject as? String,
              let storage = CredentialStorage(rawValue: value), storage != CredentialVault.storage else { return }
        CredentialVault.storage = storage
        rebuildMenu()
        // Do not read/migrate the other store: opting out of Keychain must not
        // cause another Keychain authorization prompt.
        alert("登录凭证保存方式已修改", L(storage.explanation) + "\n\n" + L("请通过“本机后台 → 登录本机后台”或管理页面重新登录一次。两种方式的凭证独立保存；切换不会读取或迁移原有凭证。"))
    }
    @objc func signIn() {
        Task {
            do {
                _ = try await client.establish()
                let credentialStorage = CredentialVault.storage
                let dialog = NSAlert(); dialog.messageText = L("登录 Controller"); dialog.informativeText = L(credentialStorage.explanation)
                dialog.addButton(withTitle: L("登录")); dialog.addButton(withTitle: L("取消"))
                let fields = NSView(frame: NSRect(x: 0, y: 0, width: 300, height: 66))
                let saved = CredentialVault.load(from: credentialStorage)
                let username = NSTextField(frame: NSRect(x: 0, y: 38, width: 300, height: 24)); username.stringValue = saved?.username ?? "admin"
                let password = NSSecureTextField(frame: NSRect(x: 0, y: 4, width: 300, height: 24)); password.placeholderString = L("密码"); password.stringValue = saved?.password ?? ""
                fields.addSubview(username); fields.addSubview(password); dialog.accessoryView = fields
                NSApp.activate(ignoringOtherApps: true)
                guard dialog.runModal() == .alertFirstButtonReturn else { return }
                defer { password.stringValue = "" }
                _ = try await client.call("login", method: "POST", body: ["username": username.stringValue, "password": password.stringValue])
                do { try CredentialVault.save(username: username.stringValue, password: password.stringValue, to: credentialStorage) }
                catch { alert("登录成功，但凭证未保存", error.localizedDescription) }
                await refresh()
            } catch { alert("登录失败", error.localizedDescription) }
        }
    }
    @objc func openManagement() {
        // A visible management window needs a regular application's menu bar.
        // Return to accessory mode when it closes so background operation keeps
        // the original menu-bar-only behavior.
        NSApp.setActivationPolicy(.regular)
        if window == nil {
            let configuration = WKWebViewConfiguration()
            configuration.websiteDataStore = .nonPersistent()
            configuration.userContentController.add(self, name: "routepilotLocale")
            configuration.userContentController.add(self, name: "routepilotCredentials")
            configuration.userContentController.add(self, name: "routepilotPreferences")
            configuration.userContentController.add(self, name: "routepilotServiceHealth")
            configuration.userContentController.add(self, name: "routepilotUpdateAction")
            configureLanguageSeed(configuration.userContentController)
            configurePreferenceSeed(configuration.userContentController)
            let view = WKWebView(frame: .zero, configuration: configuration)
            view.navigationDelegate = self
            view.uiDelegate = self
            let panel = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1150, height: 780), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
            panel.title = L("RoutePilot · DNS 和路由控制器")
            panel.minSize = NSSize(width: 420, height: 480)
            panel.level = .normal
            panel.collectionBehavior = [.fullScreenPrimary]
            panel.contentView = view; panel.delegate = self; panel.isReleasedWhenClosed = false
            // AppKit persists the normal window frame across app restarts and
            // adjusts restored frames for the screens currently available.
            ManagementWindowFrame.restore(panel)
            window = panel; webView = view
            let cookies = client.session.configuration.httpCookieStorage?.cookies(for: client.base) ?? []
            let group = DispatchGroup()
            for cookie in cookies { group.enter(); view.configuration.websiteDataStore.httpCookieStore.setCookie(cookie) { group.leave() } }
            group.notify(queue: .main) { view.load(URLRequest(url: self.client.base)) }
        }
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }
    func windowWillClose(_ notification: Notification) {
        guard let closed = notification.object as? NSWindow, closed === window else { return }
        ManagementWindowFrame.remember(closed)
        NSApp.setActivationPolicy(.accessory)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func trustedDialog(_ frame: WKFrameInfo) -> Bool {
        let origin = frame.securityOrigin
        return frame.isMainFrame && origin.protocol == client.base.scheme && origin.host == client.base.host?.trimmingCharacters(in: CharacterSet(charactersIn: "[]")) && origin.port == client.base.port
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        guard trustedDialog(frame) else { completionHandler(false); return }
        let dialog = NSAlert(); dialog.messageText = L("确认管理操作")
        dialog.informativeText = (frame.request.url?.host ?? "RoutePilot") + "\n\n" + message
        dialog.addButton(withTitle: L("确认")); dialog.addButton(withTitle: L("取消"))
        NSApp.activate(ignoringOtherApps: true)
        if let window = webView.window { dialog.beginSheetModal(for: window) { completionHandler($0 == .alertFirstButtonReturn) } }
        else { completionHandler(dialog.runModal() == .alertFirstButtonReturn) }
    }
    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        guard trustedDialog(frame) else { completionHandler(); return }
        let dialog = NSAlert(); dialog.messageText = "RoutePilot"
        dialog.informativeText = (frame.request.url?.host ?? "RoutePilot") + "\n\n" + message
        dialog.addButton(withTitle: L("确定"))
        if let window = webView.window { dialog.beginSheetModal(for: window) { _ in completionHandler() } }
        else { _ = dialog.runModal(); completionHandler() }
    }
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
        if SMAppService.mainApp.status == .requiresApproval { SMAppService.openSystemSettingsLoginItems(); return }
        do {
            if SMAppService.mainApp.status == .enabled { try SMAppService.mainApp.unregister() }
            else {
                try SMAppService.mainApp.register()
                if SMAppService.mainApp.status == .requiresApproval { SMAppService.openSystemSettingsLoginItems() }
            }
            rebuildMenu()
        } catch { alert("登录启动未修改", error.localizedDescription) }
    }
    @objc func reconnect() { bootFailures = 0; nextBoot = .distantPast; Task { await refresh() } }
    @objc func openLogs() {
        let url = child == nil && FileManager.default.fileExists(atPath: "/var/log/surge-controller") ? URL(fileURLWithPath: "/var/log/surge-controller") : dataDirectory
        NSWorkspace.shared.open(url)
    }
    func shellQuote(_ value: String) -> String { "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'" }
    @objc func installService() {
        guard !FileManager.default.fileExists(atPath: "/Library/LaunchDaemons/com.routepilot.surge-controller.plist") else { return }
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
    func stopChild() async -> Bool {
        guard let process = child else { return true }
        process.terminate()
        for _ in 0..<200 {
            if !process.isRunning { return true }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return !process.isRunning
    }
    func applicationWillTerminate(_ notification: Notification) { statusIconAnimator?.stop() }
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
            self.timer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
                Task { @MainActor in await self?.refresh() }
                Task { @MainActor in await self?.refreshRemoteNotifications() }
            }
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
        if CommandLine.arguments.contains("--finish-remote-update") {
            application.setActivationPolicy(.accessory)
            Task { @MainActor in
                await RemoteUpdateRestart.run()
                Darwin.exit(0)
            }
            application.run()
            return
        }
        let delegate = AppDelegate()
        application.delegate = delegate
        application.run()
        withExtendedLifetime(delegate) {}
    }
}
