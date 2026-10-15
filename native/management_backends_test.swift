import AppKit

@MainActor
private final class BackendMenuTarget: NSObject {
    var selected: [String] = []
    @objc func open(_ sender: NSMenuItem) {
        selected.append(sender.representedObject as? String ?? "invalid")
    }
}

@main
struct ManagementBackendMenuTests {
    @MainActor static func main() throws {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        let target = BackendMenuTarget()
        let action = #selector(BackendMenuTarget.open(_:))
        let localURL = "http://127.0.0.1:1055"
        func menu(_ rows: [ManagementBackend], _ selected: [String: String]? = nil) -> NSMenuItem {
            ManagementBackendMenu.item(backends: rows, selected: selected, localURL: localURL, target: target, action: action)
        }
        func select(_ row: NSMenuItem) throws {
            guard let action = row.action else { throw AppFailure(message: "Backend menu has no action") }
            try nativeCheck(app.sendAction(action, to: row.target, from: row), "Backend menu action was not delivered")
        }
        let local = menu([])
        try nativeCheck(local.submenu == nil, "Local-only menu unnecessarily expanded")
        try select(local)
        try nativeCheck(target.selected == [""], "Local-only action did not select local backend")
        let rows = ManagementBackendMenu.profiles([
            ["id": "a", "name": "Office", "url": "http://192.0.2.1:1055"],
            ["id": "b", "name": "Office", "url": "https://[2001:db8::1]:1055"],
            ["name": "Incomplete"]
        ])
        try nativeCheck(rows.count == 2, "Incomplete profiles were accepted or valid profiles lost")
        let expanded = menu(rows, ["id": "b", "url": rows[1].url])
        guard let submenu = expanded.submenu else { throw AppFailure(message: "Saved backends have no submenu") }
        try nativeCheck(submenu.items.count == 3, "Backend submenu omitted local or remote entries")
        try nativeCheck(submenu.items.map(\.toolTip) == [localURL, rows[0].url, rows[1].url], "Backend addresses lost")
        try nativeCheck(submenu.items.map(\.state) == [.off, .off, .on], "Selected backend checkmark is incorrect")
        try select(submenu.items[1]); try select(submenu.items[2]); try select(submenu.items[0])
        try nativeCheck(target.selected == ["", "a", "b", ""], "Duplicate backend names misdirected selection")
        try nativeCheck(menu(rows).submenu?.items.first?.state == .on, "Local selection has no checkmark")
        let stale = menu(rows, ["id": "b", "url": "http://192.0.2.99:1055"])
        try nativeCheck(stale.submenu?.items.allSatisfy { $0.state == .off } == true, "Changed backend address was marked current")
        try nativeCheck(menu(ManagementBackendMenu.profiles(nil)).submenu == nil, "Removed backends left an empty submenu")
        print("PASS native backend menu: local shortcut, saved profiles, exact selection, addresses and checkmarks")
    }
}
