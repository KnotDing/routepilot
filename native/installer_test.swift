import Foundation

@main
struct InstallerTests {
    static func main() throws {
        let files = FileManager.default
        let fixture = URL(fileURLWithPath: "/private/tmp/RoutePilot-stage-test-" + UUID().uuidString, isDirectory: true)
        try files.createDirectory(at: fixture, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
        defer { try? files.removeItem(at: fixture) }
        let service = fixture.appendingPathComponent("Service")
        let data = fixture.appendingPathComponent("data")
        for directory in [service, service.appendingPathComponent("bin"), service.appendingPathComponent("scripts"), service.appendingPathComponent("launchd"), data] {
            try files.createDirectory(at: directory, withIntermediateDirectories: false)
        }
        let resources = ["bin/surge-controller", "scripts/install_macos.sh", "launchd/com.routepilot.surge-controller.plist"]
        for path in resources { try Data("fixture".utf8).write(to: service.appendingPathComponent(path)) }
        let database = data.appendingPathComponent("controller.sqlite")
        let key = data.appendingPathComponent("controller.sqlite.key")
        try Data("fixture-database".utf8).write(to: database)
        try Data("fixture-key".utf8).write(to: key)
        let stage = try InstallerStage.prepare(service: service, data: data)
        defer { stage.remove() }
        func check(_ condition: Bool, _ message: String) throws {
            if !condition { throw NSError(domain: "InstallerTest", code: 1, userInfo: [NSLocalizedDescriptionKey: message]) }
        }
        for name in ["controller.sqlite", "controller.sqlite.key"] {
            let staged = stage.data.appendingPathComponent(name)
            try check(Data(contentsOf: staged) == Data(contentsOf: data.appendingPathComponent(name)), "migration bytes changed")
            try check((files.attributesOfItem(atPath: staged.path)[.posixPermissions] as? NSNumber)?.intValue == 0o600, "private file permissions lost")
        }
        try check((files.attributesOfItem(atPath: stage.directory.path)[.posixPermissions] as? NSNumber)?.intValue == 0o700, "private staging permissions lost")
        for path in resources {
            try check(Data(contentsOf: stage.service.appendingPathComponent(path)) == Data(contentsOf: service.appendingPathComponent(path)), "staged resource changed")
        }
        stage.remove()
        let update = try InstallerStage.prepare(service: service, data: nil)
        try check(!files.fileExists(atPath: update.data.appendingPathComponent("controller.sqlite").path), "update accessed user database")
        update.remove()
        try check(!files.fileExists(atPath: stage.directory.path) && files.fileExists(atPath: database.path), "cleanup removed original or leaked stage")
        let wal = data.appendingPathComponent("controller.sqlite-wal")
        try Data().write(to: wal)
        do {
            let unexpected = try InstallerStage.prepare(service: service, data: data)
            unexpected.remove()
            fatalError("active WAL was accepted")
        } catch { try check((error as NSError).code == 2, "wrong WAL rejection") }
        try files.removeItem(at: wal)
        try files.removeItem(at: key)
        try files.createSymbolicLink(at: key, withDestinationURL: database)
        do {
            let unexpected = try InstallerStage.prepare(service: service, data: data)
            unexpected.remove()
            fatalError("symlink key was accepted")
        } catch { try check((error as NSError).code == 1, "wrong symlink rejection") }
        print("PASS: staged data/resources identical; private permissions; cleanup; WAL and symlink rejection")
    }
}
