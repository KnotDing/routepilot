import AppKit

// One decision window for both manual checks and available-update reminders.
final class ReleaseUpdateDialog: NSObject {
    enum Choice { case install, later, skip }
    private let alert = NSAlert()
    private let accessory = NSView(frame: NSRect(x: 0, y: 0, width: 460, height: 32))
    private let disclosure = NSButton(checkboxWithTitle: L("更新说明"), target: nil, action: nil)
    private let scroll = NSScrollView(frame: NSRect(x: 0, y: 0, width: 460, height: 230))
    init(release: GiteaRelease, assistantReady: Bool) {
        super.init()
        alert.messageText = L("RoutePilot {tag} 可用", ["tag": release.tagName])
        let current = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "—"
        alert.informativeText = L("当前版本：{version}。升级会保留配置、数据库和登录凭证，并短暂重启本机服务。", ["version": current])
        if !assistantReady { alert.informativeText += "\n" + L("首次升级需授权安装更新助手，后续升级无需再次输入管理员密码。") }
        alert.addButton(withTitle: L("立即升级"))
        alert.addButton(withTitle: L("稍后升级"))
        alert.addButton(withTitle: L("跳过该版本"))
        alert.buttons[1].keyEquivalent = "\u{1b}"
        disclosure.setButtonType(.pushOnPushOff)
        disclosure.isBordered = false
        disclosure.title = "▸ " + L("更新说明")
        disclosure.target = self
        disclosure.action = #selector(toggleNotes)
        disclosure.frame = NSRect(x: 0, y: 0, width: 460, height: 28)
        disclosure.alignment = .left
        disclosure.setAccessibilityLabel(L("展开或折叠更新说明"))
        accessory.addSubview(disclosure)
        let text = NSTextView(frame: scroll.bounds)
        text.isEditable = false
        text.isSelectable = true
        text.isRichText = false
        text.font = .systemFont(ofSize: 13)
        text.textContainerInset = NSSize(width: 10, height: 10)
        text.autoresizingMask = [.width]
        text.isVerticallyResizable = true
        text.textContainer?.widthTracksTextView = true
        text.string = release.body?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false ? release.body! : L("此版本未提供更新说明。")
        scroll.documentView = text
        scroll.hasVerticalScroller = true
        scroll.borderType = .bezelBorder
        scroll.isHidden = true
        accessory.addSubview(scroll)
        alert.accessoryView = accessory
    }
    @objc private func toggleNotes() {
        let expanded = disclosure.state == .on
        disclosure.title = (expanded ? "▾ " : "▸ ") + L("更新说明")
        accessory.setFrameSize(NSSize(width: 460, height: expanded ? 268 : 32))
        disclosure.frame.origin.y = expanded ? 238 : 0
        scroll.isHidden = !expanded
        alert.layout()
    }
    func run() -> Choice {
        NSApp.activate(ignoringOtherApps: true)
        switch alert.runModal() {
        case .alertFirstButtonReturn: return .install
        case .alertThirdButtonReturn: return .skip
        default: return .later
        }
    }
}
