import AppKit
@preconcurrency import WebKit

@main
struct ManagementWindowTests {
    @MainActor static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        Task { @MainActor in
            do { try await checks(); print("PASS native management windows: separate targets, sessions, CSRF, callbacks, preferences, visibility, reuse and close"); exit(0) }
            catch { fputs("FAIL native management windows: " + error.localizedDescription + "\n", stderr); exit(1) }
        }
        app.run()
    }
    @MainActor static func check(_ condition: Bool, _ message: String) throws {
        if !condition { throw AppFailure(message: message) }
    }
    @MainActor static func wait(_ label: String = "Isolated page did not become ready", _ condition: () async throws -> Bool) async throws {
        for _ in 0..<150 {
            if try await condition() { return }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        throw AppFailure(message: label)
    }
    @MainActor static func js(_ page: ManagementPage, _ source: String) async throws -> Any? {
        try await page.view.callAsyncJavaScript("return eval(source) ?? null", arguments: ["source": source], in: nil, contentWorld: .page)
    }
    @MainActor static func write(_ page: ManagementPage, _ mode: String) async throws -> [String: String] {
        try await page.view.callAsyncJavaScript("return await window.write(mode)", arguments: ["mode": mode], in: nil, contentWorld: .page) as? [String: String] ?? [:]
    }
    @MainActor static func checks() async throws {
        let delegate = AppDelegate()
        let base = URL(string: "http://127.0.0.1:" + CommandLine.arguments[1])!
        delegate.client = ControllerClient(base)
        delegate.refreshing = true // Do not start status monitoring or access local credentials.
        let a = ManagementBackend(id: "test-a", name: "Office", url: "http://192.0.2.10:1055")
        let b = ManagementBackend(id: "test-b", name: "Office", url: "https://192.0.2.20:1055")
        delegate.managementBackends = [a, b]
        defer {
            for page in delegate.managementPages.pages { page.window.close() }
            for name in [ManagementWindowFrame.name, ManagementWindowFrame.name + ".test-a", ManagementWindowFrame.name + ".test-b"] { NSWindow.removeFrame(usingName: name) }
            UserDefaults.standard.removeObject(forKey: "managementBackend")
        }
        delegate.openManagement(backend: a, activate: false)
        delegate.openManagement(backend: b, activate: false)
        delegate.openManagement(backend: nil, activate: false)
        guard let pageA = delegate.managementPages.matching(a, base: base), let pageB = delegate.managementPages.matching(b, base: base), let local = delegate.managementPages.matching(nil, base: base) else { throw AppFailure(message: "Missing independent windows") }
        for page in [pageA, pageB, local] {
            try await wait { try await js(page, "window.ready===true") as? Bool == true }
        }
        try check(delegate.managementPages.pages.count == 3, "Opening another backend replaced an existing page")
        try check(pageA.window.subtitle == a.url && pageB.window.subtitle == b.url, "Duplicate names lost backend identity")
        let first = try await write(pageA, "direct"), second = try await write(pageB, "surge"), third = try await write(local, "auto")
        try check(first["path"] == "/api/v1/management/relay/test-a/mode" && second["path"] == "/api/v1/management/relay/test-b/mode" && third["path"] == "/api/v1/mode", "Mutation reached another page's backend")
        try check(first["expected"] == a.url && second["expected"] == b.url && third["expected"] == "", "Requests were not bound to each window's selected address")
        try check(Set([first["session"], second["session"], third["session"]].compactMap { $0 }).count == 3, "Local sessions were shared across windows")
        _ = try await js(pageB, "document.getElementById('edit').value='unsaved';sessionStorage.setItem('selection','b')")
        delegate.openManagement(backend: b, activate: false)
        try check(delegate.managementPages.pages.count == 3, "Same backend opened duplicate window")
        try check(try await js(pageB, "document.getElementById('edit').value") as? String == "unsaved", "Opening another page discarded unsaved input")

        // A background page's reply must never be sent to the focused page.
        delegate.managementPages.focus(pageB)
        _ = try await js(pageA, "window.webkit.messageHandlers.routepilotCredentials.postMessage({action:'local-login',requestID:'page-a-login'})")
        try await wait { try await js(pageA, "window.replies.some(r=>r.requestID==='page-a-login'&&r.session?.authenticated)") as? Bool == true }
        try check(try await js(pageB, "window.replies.length") as? Int == 0, "Login reply leaked into another window")
        try check((try await write(pageB, "surge"))["session"] == second["session"], "Another page's login changed this session")
        _ = try await js(pageA, "window.webkit.messageHandlers.routepilotPreferences.postMessage({_requestID:'invalid-a',statusBar:'invalid'})")
        try await wait { try await js(pageA, "window.preferenceReplies.some(r=>r.requestID==='invalid-a'&&r.ok===false)") as? Bool == true }
        try check(try await js(pageB, "window.preferenceReplies.length") as? Int == 0, "Preference reply went to focused window")

        delegate.refreshManagementPreferences()
        for (page, id) in [(pageA, "test-a"), (pageB, "test-b"), (local, "")] {
            try check(try await js(page, "window.routepilotNativePreferences.managementBackend?.id||''") as? String == id, "Preference refresh changed window selection")
        }
        delegate.windowDidMiniaturize(Notification(name: NSWindow.didMiniaturizeNotification, object: pageA.window))
        try await wait { try await js(pageA, "window.visibility.includes(false)") as? Bool == true }
        try check(try await js(pageB, "window.visibility.length") as? Int == 0, "Minimizing one page paused another")
        try check(try await js(pageB, "sessionStorage.getItem('selection')") as? String == "b", "Window storage was overwritten")

        let cookie = HTTPCookie(properties: [.name: "controller_session", .value: "fixture", .domain: "127.0.0.1", .path: "/"])!
        let watchA = RemoteNotificationWatch(id: a.id, scope: a.url, base: base, cookie: cookie, settings: [:])
        let watchB = RemoteNotificationWatch(id: b.id, scope: b.url, base: base, cookie: cookie, settings: [:])
        pageA.notificationKey = "a"; pageB.notificationKey = "b"
        delegate.remoteNotificationWatches = ["a": watchA, "b": watchB]
        delegate.clearRemoteNotification(for: pageA)
        try check(delegate.remoteNotificationWatches["a"] == nil && delegate.remoteNotificationWatches["b"] === watchB, "Switching one page cleared another remote notification watch")

        // Closing a sender while its asynchronous login is pending cannot reply
        // into the replacement page, including when it controls the same backend.
        _ = try await js(pageA, "window.webkit.messageHandlers.routepilotCredentials.postMessage({action:'local-login',requestID:'closed-page'})")
        pageA.window.close()
        try check(delegate.managementPages.pages.count == 2 && pageB.window.contentView === pageB.view, "Closing a page released another window")
        delegate.openManagement(backend: a, activate: false)
        guard let replacement = delegate.managementPages.matching(a, base: base) else { throw AppFailure(message: "Could not reopen closed page") }
        try await wait { try await js(replacement, "window.ready===true") as? Bool == true }
        try check(try await js(replacement, "window.replies.length") as? Int == 0, "Closed sender's login was delivered to replacement")
        try check((try await write(pageB, "direct"))["path"] == second["path"], "Remaining page cannot mutate its backend after close")
        let moved = URL(string: "http://localhost:" + CommandLine.arguments[1])!
        delegate.client = ControllerClient(moved)
        delegate.migrateManagementBase(to: moved)
        for (page, path) in [(replacement, first["path"]!), (pageB, second["path"]!), (local, third["path"]!)] {
            try await wait("Listener migration did not reload " + path) {
                guard page.view.url?.host == moved.host, page.view.url?.port == moved.port, !page.view.isLoading else { return false }
                return try await js(page, "window.ready===true") as? Bool == true
            }
            try check((try await write(page, "auto"))["path"] == path, "Local listener migration changed a window's target")
        }
        try check(delegate.applicationShouldTerminateAfterLastWindowClosed(NSApp) == false, "Closing management windows stops background app")
    }
}
