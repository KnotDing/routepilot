import AppKit
@preconcurrency import WebKit

@MainActor
final class ManagementPage {
    let window: NSWindow
    let view: WKWebView
    var base: URL
    var backend: ManagementBackend?
    let notificationID = UUID().uuidString
    var notificationGeneration = 0
    var notificationKey: String?

    init(window: NSWindow, view: WKWebView, base: URL, backend: ManagementBackend?) {
        self.window = window; self.view = view; self.base = base; self.backend = backend
    }
    var selection: [String: String]? { backend.map { ["id": $0.id, "url": $0.url] } }
    var frameName: String { backend.map { ManagementWindowFrame.name + "." + $0.id } ?? ManagementWindowFrame.name }
    func updateTitle() {
        window.title = "RoutePilot · " + (backend?.name ?? L("本地服务"))
        window.subtitle = backend?.url ?? base.absoluteString
    }
}

@MainActor
final class ManagementPages {
    private(set) var pages: [ManagementPage] = []
    private weak var lastActive: ManagementPage?
    var active: ManagementPage? { pages.first { $0.window.isKeyWindow } ?? lastActive ?? pages.last }
    func page(for view: WKWebView) -> ManagementPage? { pages.first { $0.view === view } }
    func page(for window: NSWindow) -> ManagementPage? { pages.first { $0.window === window } }
    func page(for controller: WKUserContentController) -> ManagementPage? {
        pages.first { $0.view.configuration.userContentController === controller }
    }
    func matching(_ backend: ManagementBackend?, base: URL) -> ManagementPage? {
        pages.first { $0.base == base && $0.backend?.id == backend?.id && $0.backend?.url == backend?.url }
    }
    func append(_ page: ManagementPage) { pages.append(page); lastActive = page }
    func focus(_ page: ManagementPage) { lastActive = page }
    func remove(_ page: ManagementPage) {
        pages.removeAll { $0 === page }
        if lastActive === page { lastActive = pages.last }
    }
}

@MainActor
extension AppDelegate {
    @objc func openManagement() {
        if let page = managementPages.active { openManagement(backend: page.backend) }
        else { openManagement(backend: preferredManagementBackend()) }
    }

    func openManagement(backend: ManagementBackend?, activate: Bool = true) {
        if activate { NSApp.setActivationPolicy(.regular) }
        if let page = managementPages.matching(backend, base: client.base) {
            managementPages.focus(page)
            if page.window.isMiniaturized { page.window.deminiaturize(nil) }
            if activate { page.window.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true) }
            return
        }
        let base = client.base
        let configuration = WKWebViewConfiguration()
        // Each page owns its login cookies as well as sessionStorage and JS state.
        configuration.websiteDataStore = .nonPersistent()
        for name in ["routepilotLocale", "routepilotCredentials", "routepilotPreferences", "routepilotGitHubToken", "routepilotServiceHealth", "routepilotUpdateAction"] {
            configuration.userContentController.add(self, name: name)
        }
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = self; view.uiDelegate = self
        let panel = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1150, height: 780), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        panel.minSize = NSSize(width: 420, height: 480)
        panel.level = .normal; panel.collectionBehavior = [.fullScreenPrimary]
        panel.contentView = view; panel.delegate = self; panel.isReleasedWhenClosed = false
        let page = ManagementPage(window: panel, view: view, base: base, backend: backend)
        managementPages.append(page)
        page.updateTitle()
        configureLanguageSeed(configuration.userContentController)
        configurePreferenceSeed(configuration.userContentController)
        ManagementWindowFrame.restore(panel, name: page.frameName)
        if managementPages.pages.count > 1 {
            let previous = managementPages.pages[managementPages.pages.count - 2].window.frame
            panel.cascadeTopLeft(from: NSPoint(x: previous.minX, y: previous.maxY))
        }
        // Bootstrap a fresh local session via the existing saved-credential flow.
        // Reusing the status client's session cookie would couple logout across pages.
        Task { @MainActor [weak self, weak page] in
            guard let self, let page, self.managementPages.page(for: page.view) === page, page.base == base else { return }
            page.view.load(URLRequest(url: base))
        }
        if activate { NSApp.activate(ignoringOtherApps: true); panel.makeKeyAndOrderFront(nil) }
        if !smoke { ManagementWindowFrame.isOpen = true }
    }

    func windowDidBecomeKey(_ notification: Notification) {
        if let window = notification.object as? NSWindow, let page = managementPages.page(for: window) {
            managementPages.focus(page)
            persistManagementSelection(page)
        }
    }
    func windowWillClose(_ notification: Notification) {
        guard let window = notification.object as? NSWindow, let page = managementPages.page(for: window) else { return }
        ManagementWindowFrame.remember(window, name: page.frameName)
        page.notificationGeneration += 1
        managementPages.remove(page)
        ManagementWebViewLifecycle.release(page.view, from: window)
        if !smoke && !quitting { ManagementWindowFrame.isOpen = !managementPages.pages.isEmpty }
        if managementPages.pages.isEmpty { NSApp.setActivationPolicy(.accessory) }
    }
    func windowDidMiniaturize(_ notification: Notification) { setManagementWindowVisible(false, notification) }
    func windowDidDeminiaturize(_ notification: Notification) { setManagementWindowVisible(true, notification) }
    private func setManagementWindowVisible(_ visible: Bool, _ notification: Notification) {
        guard let window = notification.object as? NSWindow, let page = managementPages.page(for: window) else { return }
        page.view.evaluateJavaScript("window.dispatchEvent(new CustomEvent('routepilot-window-visibility',{detail:{visible:\(visible)}}))")
    }
    func refreshManagementPreferences() {
        for page in managementPages.pages { configurePreferenceSeed(page.view.configuration.userContentController) }
    }
    func migrateManagementBase(to base: URL, navigating: WKWebView? = nil) {
        remoteNotificationWatches.removeAll()
        for page in managementPages.pages {
            page.base = base; page.notificationGeneration += 1; page.notificationKey = nil
            page.updateTitle()
            configurePreferenceSeed(page.view.configuration.userContentController)
            if page.view !== navigating { page.view.load(URLRequest(url: base)) }
        }
    }
    func clearRemoteNotification(for page: ManagementPage) {
        page.notificationGeneration += 1
        if let key = page.notificationKey, !managementPages.pages.contains(where: { $0 !== page && $0.notificationKey == key }) {
            remoteNotificationWatches[key] = nil
        }
        page.notificationKey = nil
    }
}
