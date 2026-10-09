import AppKit
import WebKit

// No privileged bridge is exposed to the offline Markdown viewer.
@MainActor
private final class ReleaseNotesView: WKWebView {
    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        updateAppearance()
    }
    func appearanceValues() -> [String: String] {
        var values: [String: String] = [:]
        effectiveAppearance.performAsCurrentDrawingAppearance {
            for (name, color) in ["notes-background": NSColor.windowBackgroundColor, "text": .labelColor,
                                   "muted": .secondaryLabelColor, "accent": .linkColor,
                                   "border": .separatorColor, "hover": .quaternaryLabelColor] {
                if let rgb = color.usingColorSpace(.sRGB) {
                    values[name] = "rgba(\(Int(rgb.redComponent * 255)),\(Int(rgb.greenComponent * 255)),\(Int(rgb.blueComponent * 255)),\(rgb.alphaComponent))"
                }
            }
        }
        return values
    }
    func updateAppearance() {
        underPageBackgroundColor = .windowBackgroundColor
        guard let data = try? JSONSerialization.data(withJSONObject: appearanceValues()),
              let json = String(data: data, encoding: .utf8) else { return }
        evaluateJavaScript("window.routepilotNotesAppearance?.(\(json))", completionHandler: nil)
    }
}

// A single compact decision window, with a plain disclosure and horizontal actions.
@MainActor
final class ReleaseUpdateDialog: NSObject, NSWindowDelegate, WKNavigationDelegate {
    enum Choice { case install, later, skip }
    private let panel = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 520, height: 260),
                                styleMask: [.titled, .closable], backing: .buffered, defer: false)
    private let disclosure = NSButton(title: "", target: nil, action: nil)
    private let descriptionLabel = NSTextField(wrappingLabelWithString: "")
    private let titleLabel = NSTextField(labelWithString: "")
    private let icon = NSImageView()
    private var notes: ReleaseNotesView!
    private let notesError = NSTextField(wrappingLabelWithString: "")
    private var notesFailed = false
    private var buttons: [NSButton] = []
    private var expanded = false
    private var continuation: CheckedContinuation<Choice, Never>?

    init(release: GiteaRelease, assistantReady: Bool) {
        super.init()
        panel.title = L("软件更新")
        panel.backgroundColor = .windowBackgroundColor
        panel.isReleasedWhenClosed = false
        panel.delegate = self
        let title = L("RoutePilot {tag} 可用", ["tag": release.tagName])
        panel.setAccessibilityLabel(title)
        titleLabel.stringValue = title
        titleLabel.font = .systemFont(ofSize: 17, weight: .semibold)
        let current = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "—"
        descriptionLabel.stringValue = L("当前版本：{version}。升级会保留配置、数据库和登录凭证，并短暂重启本机服务。", ["version": current])
        if !assistantReady {
            descriptionLabel.stringValue += "\n" + L("首次升级需授权安装更新助手，后续升级无需再次输入管理员密码。")
        }
        descriptionLabel.font = .systemFont(ofSize: 13)
        descriptionLabel.textColor = .secondaryLabelColor
        descriptionLabel.isSelectable = true
        icon.image = NSApp.applicationIconImage
        icon.imageScaling = .scaleProportionallyUpOrDown
        disclosure.title = "▸ " + L("更新说明")
        disclosure.setButtonType(.momentaryPushIn)
        disclosure.isBordered = false
        disclosure.alignment = .left
        disclosure.target = self
        disclosure.action = #selector(toggleNotes)
        disclosure.setAccessibilityLabel(L("展开或折叠更新说明"))

        let configuration = WKWebViewConfiguration()
        let contentController = WKUserContentController()
        configuration.userContentController = contentController
        notes = ReleaseNotesView(frame: .zero, configuration: configuration)
        notes.navigationDelegate = self
        notes.underPageBackgroundColor = .windowBackgroundColor
        notes.setAccessibilityLabel(L("更新说明"))
        notesError.stringValue = L("更新说明加载失败，请关闭窗口后重新检查更新。")
        notesError.textColor = .secondaryLabelColor
        notesError.isSelectable = true
        notesError.isHidden = true
        let body = release.body?.trimmingCharacters(in: .whitespacesAndNewlines)
        let payload: [String: Any] = ["body": body?.isEmpty == false ? body! : L("此版本未提供更新说明。"),
                                     "locale": Locale.preferredLanguages.first ?? "zh-CN", "appearance": notes.appearanceValues()]
        if let data = try? JSONSerialization.data(withJSONObject: payload), let json = String(data: data, encoding: .utf8) {
            contentController.addUserScript(WKUserScript(source: "window.routepilotReleaseNotes = \(json);", injectionTime: .atDocumentStart, forMainFrameOnly: true))
        }
        if let directory = Bundle.main.resourceURL?.appendingPathComponent("ReleaseNotes", isDirectory: true),
           FileManager.default.fileExists(atPath: directory.appendingPathComponent("notes.html").path) {
            notes.loadFileURL(directory.appendingPathComponent("notes.html"), allowingReadAccessTo: directory)
        } else {
            notesFailed = true
        }
        notes.isHidden = true
        for (index, title) in [L("跳过该版本"), L("稍后升级"), L("立即升级")].enumerated() {
            let button = NSButton(title: title, target: self, action: #selector(selectChoice(_:)))
            button.tag = index
            button.bezelStyle = .rounded
            if index == 1 { button.keyEquivalent = "\u{1b}" }
            if index == 2 { button.keyEquivalent = "\r" }
            buttons.append(button)
        }
        panel.defaultButtonCell = buttons.last?.cell as? NSButtonCell
        for view in [icon, titleLabel, descriptionLabel, disclosure, notes!, notesError] + buttons {
            panel.contentView?.addSubview(view)
        }
        layout()
    }
    private func layout() {
        let width: CGFloat = 520, padding: CGFloat = 24, inner: CGFloat = width - padding * 2
        let textHeight = ceil((descriptionLabel.stringValue as NSString).boundingRect(
            with: NSSize(width: inner, height: 500), options: [.usesLineFragmentOrigin, .usesFontLeading],
            attributes: [.font: descriptionLabel.font!]).height) + 4
        let height = padding * 2 + 44 + 14 + textHeight + 18 + 28 + 18 + 32 + (expanded ? 272 : 0)
        let oldTop = panel.frame.maxY
        panel.setContentSize(NSSize(width: width, height: height))
        if panel.isVisible { panel.setFrameOrigin(NSPoint(x: panel.frame.minX, y: oldTop - panel.frame.height)) }
        icon.frame = NSRect(x: padding, y: height - padding - 44, width: 44, height: 44)
        titleLabel.frame = NSRect(x: 84, y: height - padding - 34, width: inner - 60, height: 26)
        descriptionLabel.frame = NSRect(x: padding, y: height - padding - 44 - 14 - textHeight, width: inner, height: textHeight)
        disclosure.frame = NSRect(x: padding, y: padding + 32 + 18 + (expanded ? 272 : 0), width: inner, height: 28)
        notes.frame = NSRect(x: padding, y: padding + 32 + 18, width: inner, height: 260)
        notesError.frame = NSRect(x: padding + 4, y: padding + 32 + 18 + 200, width: inner - 8, height: 48)
        let buttonWidth = (inner - 20) / 3
        for (index, button) in buttons.enumerated() {
            button.frame = NSRect(x: padding + CGFloat(index) * (buttonWidth + 10), y: padding, width: buttonWidth, height: 32)
        }
    }
    @objc private func toggleNotes() {
        expanded.toggle()
        disclosure.title = (expanded ? "▾ " : "▸ ") + L("更新说明")
        disclosure.setAccessibilityValue(expanded ? 1 : 0)
        notes.isHidden = !expanded || notesFailed
        notesError.isHidden = !expanded || !notesFailed
        layout()
    }
    @objc private func selectChoice(_ sender: NSButton) {
        finish(sender.tag == 2 ? .install : sender.tag == 0 ? .skip : .later)
    }
    func windowShouldClose(_ sender: NSWindow) -> Bool {
        finish(.later)
        return true
    }
    private func finish(_ choice: Choice) {
        guard let pending = continuation else { return }
        continuation = nil
        panel.orderOut(nil)
        pending.resume(returning: choice)
    }
    private func showNotesFailure() {
        notesFailed = true
        notes.isHidden = true
        notesError.isHidden = !expanded
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        notes.updateAppearance()
        webView.evaluateJavaScript("Boolean(document.querySelector('.markdown-notes'))") { [weak self] value, error in
            guard let self else { return }
            if error != nil || value as? Bool != true { self.showNotesFailure() }
        }
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { showNotesFailure() }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { showNotesFailure() }
    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) { showNotesFailure() }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        if navigationAction.navigationType == .linkActivated {
            if let url = navigationAction.request.url, ["https", "http"].contains(url.scheme?.lowercased() ?? "") {
                NSWorkspace.shared.open(url)
            }
            decisionHandler(.cancel)
        } else {
            // Only the packaged, offline viewer can navigate within this web view.
            decisionHandler(navigationAction.request.url?.isFileURL == true ? .allow : .cancel)
        }
    }
    func run() async -> Choice {
        // Leave the normal application run loop available for WebKit loading and
        // JavaScript rendering while waiting for one of the window's actions.
        await withCheckedContinuation { pending in
            continuation = pending
            NSApp.activate(ignoringOtherApps: true)
            panel.center()
            panel.makeKeyAndOrderFront(nil)
        }
    }
}
