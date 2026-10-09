import Foundation
import CryptoKit
import Security
import Darwin

struct LoginCredential: Codable {
    let username: String
    let password: String
}

enum CredentialStorage: String, CaseIterable {
    case keychain, localFile

    var title: String { self == .keychain ? "macOS 钥匙串" : "本地文件" }
    var explanation: String {
        self == .keychain
            ? "登录成功后，凭证将保存在 macOS 钥匙串中。"
            : "登录成功后，凭证将保存在本机用户目录中，更新 App 后无需钥匙串授权。同一用户运行的其他程序可能读取此文件。"
    }
}

enum CredentialVault {
    static var storage: CredentialStorage {
        get { CredentialStorage(rawValue: UserDefaults.standard.string(forKey: "credentialStorage") ?? "") ?? .keychain }
        set { UserDefaults.standard.set(newValue.rawValue, forKey: "credentialStorage") }
    }

    static func load(from selected: CredentialStorage? = nil) -> LoginCredential? {
        switch selected ?? storage {
        case .keychain: return KeychainCredentials.load()
        case .localFile: return try? LocalCredentials.shared.load()
        }
    }

    static func save(username: String, password: String, to selected: CredentialStorage? = nil, scope: String? = nil) throws {
        let credential = LoginCredential(username: username, password: password)
        switch selected ?? storage {
        case .keychain: try KeychainCredentials.save(credential, account: account(scope))
        case .localFile: try LocalCredentials.shared.save(credential, filename: filename(scope))
        }
    }

    private static func digest(_ scope: String) -> String {
        SHA256.hash(data: Data(scope.utf8)).map { String(format: "%02x", $0) }.joined()
    }
    private static func account(_ scope: String?) -> String { scope.map { "remote-login-" + digest($0) } ?? "controller-login" }
    private static func filename(_ scope: String?) -> String { scope.map { "remote-" + digest($0) + ".json" } ?? "login.json" }
    static func saved(scope: String, from selected: CredentialStorage? = nil) throws -> LoginCredential? {
        switch selected ?? storage {
        case .keychain: return KeychainCredentials.load(account: account(scope))
        case .localFile: return try LocalCredentials.shared.load(filename: filename(scope))
        }
    }
    static func forget(scope: String, from selected: CredentialStorage? = nil) throws {
        switch selected ?? storage {
        case .keychain: KeychainCredentials.delete(account: account(scope))
        case .localFile: try LocalCredentials.shared.delete(filename: filename(scope))
        }
    }

    static func delete(from selected: CredentialStorage? = nil) {
        switch selected ?? storage {
        case .keychain: KeychainCredentials.delete()
        case .localFile: try? LocalCredentials.shared.delete()
        }
    }
}

private enum KeychainCredentials {
    private static let service = Bundle.main.bundleIdentifier ?? "com.routepilot.app"
    private static func query(_ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }
    static func load(account: String = "controller-login") -> LoginCredential? {
        var request = query(account)
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data else { return nil }
        return try? JSONDecoder().decode(LoginCredential.self, from: data)
    }
    static func save(_ credential: LoginCredential, account: String) throws {
        let data = try JSONEncoder().encode(credential)
        let attributes: [String: Any] = [kSecValueData as String: data, kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly]
        let result = SecItemUpdate(query(account) as CFDictionary, attributes as CFDictionary)
        if result == errSecItemNotFound {
            var request = query(account)
            attributes.forEach { request[$0.key] = $0.value }
            let addResult = SecItemAdd(request as CFDictionary, nil)
            guard addResult == errSecSuccess else { throw AppFailure(message: L("钥匙串无法保存登录凭证")) }
        } else if result != errSecSuccess {
            throw AppFailure(message: L("钥匙串无法保存登录凭证"))
        }
    }
    static func delete(account: String = "controller-login") { SecItemDelete(query(account) as CFDictionary) }
}

struct LocalCredentials {
    // Keep credentials separate from the backend's data directory and exports.
    static let shared = LocalCredentials(directory: FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        .appendingPathComponent("RoutePilot/Credentials", isDirectory: true))
    let directory: URL
    private func failure() -> AppFailure { AppFailure(message: L("本地登录凭证文件无法访问，请检查用户目录权限。")) }

    private func openDirectory(create: Bool) throws -> Int32? {
        if create {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        }
        let fd = Darwin.open(directory.path, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        if fd < 0 {
            if !create && errno == ENOENT { return nil }
            throw failure()
        }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_uid == geteuid(), fchmod(fd, 0o700) == 0 else {
            Darwin.close(fd)
            throw failure()
        }
        return fd
    }

    func load(filename: String = "login.json") throws -> LoginCredential? {
        guard filename.hasSuffix(".json"), !filename.contains("/"), filename.utf8.count <= 255 else { throw failure() }
        guard let directoryFD = try openDirectory(create: false) else { return nil }
        defer { Darwin.close(directoryFD) }
        let fd = openat(directoryFD, filename, O_RDONLY | O_NOFOLLOW | O_NONBLOCK | O_CLOEXEC)
        if fd < 0 {
            if errno == ENOENT { return nil }
            throw failure()
        }
        let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
        defer { try? handle.close() }
        var info = stat()
        guard fstat(fd, &info) == 0, (info.st_mode & S_IFMT) == S_IFREG,
              info.st_uid == geteuid(), info.st_nlink == 1, info.st_size <= 16_384,
              fchmod(fd, 0o600) == 0 else { throw failure() }
        guard let data = try handle.readToEnd() else { return nil }
        return try JSONDecoder().decode(LoginCredential.self, from: data)
    }

    func save(_ credential: LoginCredential, filename: String) throws {
        guard filename.hasSuffix(".json"), !filename.contains("/"), filename.utf8.count <= 255 else { throw failure() }
        guard let directoryFD = try openDirectory(create: true) else { throw failure() }
        defer { Darwin.close(directoryFD) }
        let data = try JSONEncoder().encode(credential)
        guard data.count <= 16_384 else { throw failure() }
        let temporary = ".login-\(UUID().uuidString).tmp"
        let fd = openat(directoryFD, temporary, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC, 0o600)
        guard fd >= 0 else { throw failure() }
        defer { unlinkat(directoryFD, temporary, 0) }
        let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
        defer { try? handle.close() }
        guard fchmod(fd, 0o600) == 0 else { throw failure() }
        try handle.write(contentsOf: data)
        try handle.synchronize()
        guard renameat(directoryFD, temporary, directoryFD, filename) == 0 else { throw failure() }
    }

    func delete(filename: String = "login.json") throws {
        guard filename.hasSuffix(".json"), !filename.contains("/"), filename.utf8.count <= 255 else { throw failure() }
        guard let directoryFD = try openDirectory(create: false) else { return }
        defer { Darwin.close(directoryFD) }
        guard unlinkat(directoryFD, filename, 0) == 0 || errno == ENOENT else { throw failure() }
    }
}
