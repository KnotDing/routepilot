import Foundation
import Darwin

struct AutomationPreferences: Codable, Equatable {
    var cliEnabled = false
    var automaticSkillInstall = false
    static let agents = ["codex", "claude", "hermes", "cursor", "copilot", "gemini", "opencode", "windsurf", "cline", "roo", "kilo", "antigravity", "openclaw", "qwen", "amp", "pi"]
    var skillTargets = ["codex"]
    var valid: Bool {
        skillTargets.allSatisfy { Self.agents.contains($0) } &&
        Set(skillTargets).count == skillTargets.count && (!automaticSkillInstall || !skillTargets.isEmpty)
    }
    enum CodingKeys: String, CodingKey { case cliEnabled, automaticSkillInstall, skillTargets, skillTarget }
    init() {}
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        cliEnabled = try c.decodeIfPresent(Bool.self, forKey: .cliEnabled) ?? false
        automaticSkillInstall = try c.decodeIfPresent(Bool.self, forKey: .automaticSkillInstall) ?? false
        if let targets = try c.decodeIfPresent([String].self, forKey: .skillTargets) {
            skillTargets = targets
        } else {
            let legacy = try c.decodeIfPresent(String.self, forKey: .skillTarget) ?? "codex"
            skillTargets = legacy == "both" ? ["codex", "claude"] : [legacy]
        }
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(cliEnabled, forKey: .cliEnabled)
        try c.encode(automaticSkillInstall, forKey: .automaticSkillInstall)
        try c.encode(skillTargets, forKey: .skillTargets)
    }
}

struct AutomationConflict: Error {
    let items: [[String: String]]
}

// All destinations are selected locally, never supplied by the web page.
// Links follow complete App updates; foreign entries are backed up before replacement.
enum AutomationIntegration {
    private struct State: Codable {
        var cli_enabled = false
        var preferences: AutomationPreferences?
        var owned: [String: String] = [:]
        var backups: [String: String] = [:]
        var skipped: [String: String] = [:]
    }
    private struct Entry {
        let id: String
        let path: URL
        let source: URL
    }
    private static let fm = FileManager.default
    private static var directory: URL {
        fm.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("RoutePilot/CLI", isDirectory: true)
    }
    private static var stateFile: URL { directory.appendingPathComponent("integration.json") }
    private static var backupDirectory: URL { directory.appendingPathComponent("backups", isDirectory: true) }
    private static var entries: [Entry] {
        let home = fm.homeDirectoryForCurrentUser
        let resources = Bundle.main.resourceURL ?? Bundle.main.bundleURL
        let codexHome = ProcessInfo.processInfo.environment["CODEX_HOME"].flatMap { $0.hasPrefix("/") ? URL(fileURLWithPath: $0) : nil } ?? home.appendingPathComponent(".codex", isDirectory: true)
        let hermesHome = ProcessInfo.processInfo.environment["HERMES_HOME"].flatMap { $0.hasPrefix("/") ? URL(fileURLWithPath: $0) : nil } ?? home.appendingPathComponent(".hermes", isDirectory: true)
        let piHome = ProcessInfo.processInfo.environment["PI_CODING_AGENT_DIR"].flatMap { $0.hasPrefix("/") ? URL(fileURLWithPath: $0) : nil } ?? home.appendingPathComponent(".pi/agent", isDirectory: true)
        let skill = resources.appendingPathComponent("Skills/routepilot-control", isDirectory: true)
        let locations: [(String, URL)] = [
            ("codex", codexHome.appendingPathComponent("skills")),
            ("claude", home.appendingPathComponent(".claude/skills")),
            ("hermes", hermesHome.appendingPathComponent("skills")),
            ("cursor", home.appendingPathComponent(".cursor/skills")),
            ("copilot", home.appendingPathComponent(".copilot/skills")),
            ("gemini", home.appendingPathComponent(".gemini/skills")),
            ("opencode", home.appendingPathComponent(".config/opencode/skills")),
            ("windsurf", home.appendingPathComponent(".codeium/windsurf/skills")),
            ("cline", home.appendingPathComponent(".cline/skills")),
            ("roo", home.appendingPathComponent(".roo/skills")),
            ("kilo", home.appendingPathComponent(".kilo/skills")),
            ("antigravity", home.appendingPathComponent(".gemini/config/skills")),
            ("openclaw", home.appendingPathComponent(".openclaw/skills")),
            ("qwen", home.appendingPathComponent(".qwen/skills")),
            ("amp", home.appendingPathComponent(".config/amp/skills")),
            ("pi", piHome.appendingPathComponent("skills"))
        ]
        return [
            Entry(id: "cli", path: home.appendingPathComponent(".local/bin/routepilot"), source: resources.appendingPathComponent("Service/bin/routepilot")),
        ] + locations.map { Entry(id: $0.0, path: $0.1.appendingPathComponent("routepilot-control"), source: skill) }
    }
    static var supported: Bool {
        guard let cli = entries.first, let skill = entries.last else { return false }
        return fm.isExecutableFile(atPath: cli.source.path) && fm.fileExists(atPath: skill.source.appendingPathComponent("SKILL.md").path)
    }
    private static func exists(_ url: URL) -> Bool { (try? fm.attributesOfItem(atPath: url.path)) != nil }
    private static func link(_ url: URL) -> String? { try? fm.destinationOfSymbolicLink(atPath: url.path) }
    private static func failure(_ text: String) -> NSError { NSError(domain: "RoutePilot.Automation", code: 1, userInfo: [NSLocalizedDescriptionKey: L(text)]) }
    private static func ensureDirectory(_ url: URL, privateMode: Bool = false) throws {
        if exists(url) {
            let attributes = try fm.attributesOfItem(atPath: url.path)
            guard attributes[.type] as? FileAttributeType == .typeDirectory, link(url) == nil else { throw failure("安装目录不是普通文件夹，请检查路径。") }
            if privateMode, let mode = attributes[.posixPermissions] as? NSNumber, mode.intValue & 0o077 != 0 { throw failure("CLI 配置目录权限应为 0700。") }
        } else {
            try fm.createDirectory(at: url, withIntermediateDirectories: true, attributes: [.posixPermissions: privateMode ? 0o700 : 0o755])
        }
    }
    private static func readState() throws -> State {
        guard exists(stateFile) else { return State() }
        let attributes = try fm.attributesOfItem(atPath: stateFile.path)
        guard attributes[.type] as? FileAttributeType == .typeRegular, link(stateFile) == nil,
              let mode = attributes[.posixPermissions] as? NSNumber, mode.intValue & 0o077 == 0 else { throw failure("CLI 集成状态文件权限或类型无效。") }
        return try JSONDecoder().decode(State.self, from: Data(contentsOf: stateFile))
    }
    private static func backup(_ path: String?) -> URL? {
        guard let path else { return nil }
        let url = URL(fileURLWithPath: path).standardizedFileURL
        return url.path.hasPrefix(backupDirectory.path + "/") ? url : nil
    }
    static func status() -> [String: Any] {
        let state = try? readState()
        let values = entries.map { entry -> [String: Any] in
            ["id": entry.id, "path": entry.path.path, "installed": link(entry.path) == entry.source.path,
             "skipped": state?.skipped[entry.path.path] != nil]
        }
        return ["entries": values, "backupDirectory": backupDirectory.path]
    }

    // Atomic integration state is shared with the helper and the running App.
    // Older installations migrate their UserDefaults preferences on first apply.
    static func savedPreferences() throws -> AutomationPreferences? { try readState().preferences }

    private static func locked<T>(_ work: () throws -> T) throws -> T {
        try ensureDirectory(directory, privateMode: true)
        let path = directory.appendingPathComponent("integration.lock").path
        let fd = Darwin.open(path, O_CREAT | O_RDWR | O_NOFOLLOW, 0o600)
        guard fd >= 0 else { throw failure("无法锁定 CLI 与 skill 设置。") }
        defer { Darwin.close(fd) }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_uid == geteuid(), info.st_mode & S_IFMT == S_IFREG,
              info.st_mode & 0o077 == 0, info.st_nlink == 1 else { throw failure("CLI 集成锁文件权限或类型无效。") }
        guard flock(fd, LOCK_EX | LOCK_NB) == 0 else { throw failure("CLI 与 skill 设置正在应用，请稍后重试。") }
        defer { flock(fd, LOCK_UN) }
        return try work()
    }

    static func runHelper() {
        var result: [String: Any] = ["supported": supported]
        do {
            guard geteuid() >= 500 else { throw failure("CLI 与 skill 安装必须以桌面用户运行。") }
            var data = Data()
            while let part = try FileHandle.standardInput.read(upToCount: 4096), !part.isEmpty {
                data.append(part)
                guard data.count <= 65536 else { throw failure("CLI 与 skill 请求过大。") }
            }
            struct Request: Decodable {
                let action: String
                let preferences: AutomationPreferences?
                let choices: [String: String]?
            }
            let request = try JSONDecoder().decode(Request.self, from: data)
            if request.action == "set" {
                guard let preferences = request.preferences else { throw failure("CLI 与 skill 设置无效。") }
                let choices = request.choices ?? [:]
                guard choices.keys.allSatisfy({ $0 == "cli" || AutomationPreferences.agents.contains($0) }), choices.values.allSatisfy({ ["overwrite", "skip"].contains($0) }) else { throw failure("CLI 与 skill 设置无效。") }
                try apply(preferences, choices: choices)
            } else if request.action != "get" { throw failure("CLI 与 skill 设置无效。") }
            result = try locked {
                let preferences = try savedPreferences() ?? AppPreferences.current.automation
                guard preferences.valid else { throw failure("CLI 与 skill 设置无效。") }
                let value = try JSONSerialization.jsonObject(with: JSONEncoder().encode(preferences))
                return ["supported": supported, "preferences": value, "status": status()]
            }
        } catch let conflict as AutomationConflict {
            result["code"] = "automation_conflict"
            result["conflicts"] = conflict.items
            result["error"] = L("检测到同名文件，请选择覆盖或跳过。")
        } catch { result["error"] = error.localizedDescription }
        if let data = try? JSONSerialization.data(withJSONObject: result) { FileHandle.standardOutput.write(data) }
    }

    static func apply(_ preferences: AutomationPreferences, choices: [String: String] = [:], startup: Bool = false) throws {
        try locked { try applyUnlocked(preferences, choices: choices, startup: startup) }
    }

    private static func applyUnlocked(_ preferences: AutomationPreferences, choices: [String: String], startup: Bool) throws {
        guard preferences.valid else { throw failure("CLI 与 skill 设置无效。") }
        if preferences.cliEnabled || preferences.automaticSkillInstall {
            guard supported else { throw failure("此 App 未包含 CLI 或 skill，请更新完整应用。") }
        }
        let previous = try readState()
        var desired: [String: String] = [:]
        for entry in entries {
            if entry.id == "cli" ? preferences.cliEnabled : preferences.automaticSkillInstall && preferences.skillTargets.contains(entry.id) {
                desired[entry.path.path] = entry.source.path
            }
        }
        let conflicts = entries.filter { entry in
            guard let source = desired[entry.path.path], exists(entry.path) else { return false }
            if link(entry.path) == source || previous.owned[entry.path.path] == link(entry.path) && link(entry.path) != nil { return false }
            if startup && previous.skipped[entry.path.path] == source { return false }
            return choices[entry.id] != "overwrite" && choices[entry.id] != "skip"
        }.map { ["id": $0.id, "path": $0.path.path] }
        if !conflicts.isEmpty { throw AutomationConflict(items: conflicts) }

        try ensureDirectory(directory, privateMode: true)
        try ensureDirectory(backupDirectory, privateMode: true)
        let staging = directory.appendingPathComponent(".integration-" + UUID().uuidString, isDirectory: true)
        try ensureDirectory(staging, privateMode: true)
        defer { try? fm.removeItem(at: staging) }
        var undo: [() throws -> Void] = []
        var next = previous
        next.cli_enabled = preferences.cliEnabled
        next.preferences = preferences
        func move(_ from: URL, _ to: URL) throws {
            try fm.moveItem(at: from, to: to)
            undo.append { try fm.moveItem(at: to, to: from) }
        }
        do {
            for entry in entries {
                let path = entry.path.path
                let current = link(entry.path)
                let owned = current != nil && (current == previous.owned[path] || current == entry.source.path)
                guard let source = desired[path] else {
                    next.skipped[path] = nil
                    if owned {
                        try move(entry.path, staging.appendingPathComponent(entry.id))
                        if let original = backup(previous.backups[path]), exists(original) {
                            try move(original, entry.path)
                            next.backups[path] = nil
                        }
                    }
                    next.owned[path] = nil
                    continue
                }
                if exists(entry.path) && !owned {
                    if choices[entry.id] == "skip" || startup && previous.skipped[path] == source {
                        next.skipped[path] = source
                        next.owned[path] = nil
                        continue
                    }
                    let destination = backupDirectory.appendingPathComponent(UUID().uuidString + "-" + entry.id)
                    try move(entry.path, destination)
                    // Retain every replacement backup; the latest one is restored on disable.
                    next.backups[path] = destination.path
                } else if owned && current != source {
                    try move(entry.path, staging.appendingPathComponent(entry.id))
                }
                if link(entry.path) != source {
                    try ensureDirectory(entry.path.deletingLastPathComponent())
                    try fm.createSymbolicLink(atPath: path, withDestinationPath: source)
                    undo.append { if link(entry.path) == source { try fm.removeItem(at: entry.path) } }
                }
                next.owned[path] = source
                next.skipped[path] = nil
            }
            // This same flag is checked by the CLI even when invoked by absolute path.
            let pending = staging.appendingPathComponent("state.json")
            try JSONEncoder().encode(next).write(to: pending)
            try fm.setAttributes([.posixPermissions: 0o600], ofItemAtPath: pending.path)
            guard rename(pending.path, stateFile.path) == 0 else { throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno)) }
        } catch {
            var rollbackFailed = false
            for restore in undo.reversed() { do { try restore() } catch { rollbackFailed = true } }
            if rollbackFailed { throw failure("安装失败，部分文件未能恢复；请检查备份目录。") }
            throw error
        }
    }
}
