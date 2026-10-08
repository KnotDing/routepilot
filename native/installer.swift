import Foundation

struct InstallerStage {
    let directory: URL
    var service: URL { directory.appendingPathComponent("Service") }
    var data: URL { directory.appendingPathComponent("data") }

    static func prepare(service sourceService: URL, data sourceData: URL?) throws -> InstallerStage {
        let files = FileManager.default
        func require(_ url: URL, type: FileAttributeType) throws {
            let attributes = try files.attributesOfItem(atPath: url.path)
            guard attributes[.type] as? FileAttributeType == type else {
                throw NSError(domain: "RoutePilotInstaller", code: 1, userInfo: [NSLocalizedDescriptionKey: "安装来源必须是普通目录和文件，不能使用符号链接。"])
            }
        }
        if let sourceData = sourceData {
            try require(sourceData, type: .typeDirectory)
            for name in ["controller.sqlite", "controller.sqlite.key"] {
                try require(sourceData.appendingPathComponent(name), type: .typeRegular)
            }
            guard !files.fileExists(atPath: sourceData.appendingPathComponent("controller.sqlite-wal").path) else {
                throw NSError(domain: "RoutePilotInstaller", code: 2, userInfo: [NSLocalizedDescriptionKey: "后台数据库尚未完整关闭；安装已取消，原配置已保留。"])
            }
        }
        // The authorized application can read its own data and resources. Stage
        // them privately outside protected user/removable folders before the
        // administrator helper runs, so it never needs those folder permissions.
        let stage = InstallerStage(directory: URL(fileURLWithPath: "/private/tmp/RoutePilot-install-" + UUID().uuidString, isDirectory: true))
        try files.createDirectory(at: stage.directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
        do {
            for directory in [stage.service, stage.service.appendingPathComponent("bin"), stage.service.appendingPathComponent("scripts"), stage.service.appendingPathComponent("launchd"), stage.data] {
                try files.createDirectory(at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
            }
            for path in ["bin/surge-controller", "scripts/install_macos.sh", "scripts/update_macos.sh", "launchd/com.routepilot.surge-controller.plist"] {
                let source = sourceService.appendingPathComponent(path)
                try require(source, type: .typeRegular)
                let target = stage.service.appendingPathComponent(path)
                try files.copyItem(at: source, to: target)
                try files.setAttributes([.posixPermissions: path.hasPrefix("bin/") ? 0o700 : 0o600], ofItemAtPath: target.path)
            }
            if let sourceData = sourceData {
                for name in ["controller.sqlite", "controller.sqlite.key"] {
                    let target = stage.data.appendingPathComponent(name)
                    try files.copyItem(at: sourceData.appendingPathComponent(name), to: target)
                    try files.setAttributes([.posixPermissions: 0o600], ofItemAtPath: target.path)
                }
            }
            return stage
        } catch {
            try? files.removeItem(at: stage.directory)
            throw error
        }
    }

    func remove() { try? FileManager.default.removeItem(at: directory) }
}
