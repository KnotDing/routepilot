import AppKit

struct ManagementBackend: Equatable {
    let id: String
    let name: String
    let url: String
}

@MainActor
enum ManagementBackendMenu {
    static func profiles(_ value: Any?) -> [ManagementBackend] {
        guard let rows = value as? [[String: Any]] else { return [] }
        return rows.compactMap { row in
            guard let id = row["id"] as? String, !id.isEmpty,
                  let name = row["name"] as? String, !name.isEmpty,
                  let url = row["url"] as? String, !url.isEmpty else { return nil }
            return ManagementBackend(id: id, name: name, url: url)
        }
    }

    static func item(backends: [ManagementBackend], selected: [String: String]?, localURL: String,
                     target: AnyObject, action: Selector) -> NSMenuItem {
        let root = NSMenuItem(title: L("打开管理窗口"), action: backends.isEmpty ? action : nil, keyEquivalent: "")
        root.target = target
        root.representedObject = ""
        guard !backends.isEmpty else { return root }
        let menu = NSMenu()
        menu.autoenablesItems = false
        let local = NSMenuItem(title: L("本地服务"), action: action, keyEquivalent: "")
        local.target = target
        local.representedObject = ""
        local.toolTip = localURL
        local.state = selected == nil ? .on : .off
        menu.addItem(local)
        for backend in backends {
            let row = NSMenuItem(title: backend.name, action: action, keyEquivalent: "")
            row.target = target
            row.representedObject = backend.id
            row.toolTip = backend.url
            row.state = selected?["id"] == backend.id && selected?["url"] == backend.url ? .on : .off
            menu.addItem(row)
        }
        root.submenu = menu
        return root
    }
}
