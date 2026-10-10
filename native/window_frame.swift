import AppKit

enum ManagementWindowFrame {
    static let name = "RoutePilotManagementWindow"
    static let openKey = "managementWindowOpen"
    static var isOpen: Bool {
        get { UserDefaults.standard.bool(forKey: openKey) }
        set {
            UserDefaults.standard.set(newValue, forKey: openKey)
            // The restart helper and replacement App are separate processes.
            UserDefaults.standard.synchronize()
        }
    }
    static func restore(_ window: NSWindow, name: String = name) {
        if !window.setFrameUsingName(name) { window.center() }
        window.setFrameAutosaveName(name)
    }
    static func remember(_ window: NSWindow, name: String = name) {
        // AppKit preserves the normal frame while a window is full screen.
        guard !window.styleMask.contains(.fullScreen) else { return }
        window.saveFrame(usingName: name)
    }
}
