import CryptoKit
import Foundation

struct GiteaRelease: Decodable {
    struct Asset: Decodable {
        let name: String
        let browserDownloadURL: URL
        enum CodingKeys: String, CodingKey {
            case name
            case browserDownloadURL = "browser_download_url"
        }
    }
    let tagName: String
    let assets: [Asset]
    enum CodingKeys: String, CodingKey {
        case tagName = "tag_name"
        case assets
    }
}

enum ReleaseUpdater {
    static let repository = "https://gitea.knotding.com/knot/routepilot"
    private static let host = "gitea.knotding.com"

    static func latest() async throws -> GiteaRelease {
        guard let url = URL(string: "https://\(host)/api/v1/repos/knot/routepilot/releases/latest") else {
            throw AppFailure(message: "更新服务地址无效")
        }
        var request = URLRequest(url: url)
        request.timeoutInterval = 10
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
              response.url?.scheme == "https", response.url?.host == host else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 0
            throw AppFailure(message: L("获取最新版本失败（HTTP {code}）", ["code": code]))
        }
        return try JSONDecoder().decode(GiteaRelease.self, from: data)
    }

    static func newer(_ release: GiteaRelease) -> Bool {
        let latest = release.tagName.drop(while: { !$0.isNumber }).split(separator: ".").compactMap { Int($0.prefix(while: \.isNumber)) }
        let currentVersion = (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0").split(separator: ".").compactMap { Int($0.prefix(while: \.isNumber)) }
        for index in 0..<max(latest.count, currentVersion.count) {
            let lhs = index < latest.count ? latest[index] : 0
            let rhs = index < currentVersion.count ? currentVersion[index] : 0
            if lhs != rhs { return lhs > rhs }
        }
        return false
    }

    static func downloadPackage(from release: GiteaRelease) async throws -> URL {
        #if arch(arm64)
        let archiveName = "RoutePilot-macos-arm64.zip"
        #else
        let archiveName = "RoutePilot-macos-x86_64.zip"
        #endif
        guard let archive = release.assets.first(where: { $0.name == archiveName }),
              let checksums = release.assets.first(where: { $0.name == "SHA256SUMS" }) else {
            throw AppFailure(message: L("此版本没有适用于本机的安装包"))
        }
        let checksumData = try await download(checksums.browserDownloadURL, maximumBytes: 1024 * 1024)
        guard let checksumText = String(data: checksumData, encoding: .utf8),
              let expected = checksumText.split(whereSeparator: \.isNewline).compactMap({ line -> String? in
                  let columns = line.split(whereSeparator: \.isWhitespace)
                  return columns.count == 2 && String(columns[1]) == archiveName ? String(columns[0]).lowercased() : nil
              }).first, expected.count == 64, expected.allSatisfy({ "0123456789abcdef".contains($0) }) else {
            throw AppFailure(message: "Release 校验文件中没有本机安装包的 SHA-256")
        }
        let cache = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0].appendingPathComponent("RoutePilot/Updates", isDirectory: true)
        try FileManager.default.createDirectory(at: cache, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: cache.path)
        let cachedArchive = cache.appendingPathComponent(expected + ".zip")
        if let size = try? cachedArchive.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= 1024 * 1024 * 1024,
           let cached = try? Data(contentsOf: cachedArchive),
           SHA256.hash(data: cached).map({ String(format: "%02x", $0) }).joined() == expected { return cachedArchive }
        let archiveData = try await download(archive.browserDownloadURL, maximumBytes: 1024 * 1024 * 1024)
        let actual = SHA256.hash(data: archiveData).map { String(format: "%02x", $0) }.joined()
        guard actual == expected else { throw AppFailure(message: "安装包 SHA-256 校验失败，未执行升级") }

        try archiveData.write(to: cachedArchive, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: cachedArchive.path)
        for old in (try? FileManager.default.contentsOfDirectory(at: cache, includingPropertiesForKeys: nil)) ?? [] where old.pathExtension == "zip" && old != cachedArchive {
            try? FileManager.default.removeItem(at: old)
        }
        return cachedArchive
    }

    static func verifiedApp(from release: GiteaRelease) async throws -> URL {
        let archiveURL = try await downloadPackage(from: release)
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("RoutePilot-update-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
        do {
            let process = Process()
            process.executableURL = URL(fileURLWithPath: "/usr/bin/ditto")
            process.arguments = ["-x", "-k", archiveURL.path, directory.path]
            try process.run(); process.waitUntilExit()
            guard process.terminationStatus == 0 else { throw AppFailure(message: "无法解压在线升级包") }
            let app = directory.appendingPathComponent("RoutePilot.app", isDirectory: true)
            let attributes = try FileManager.default.attributesOfItem(atPath: app.path)
            guard attributes[.type] as? FileAttributeType == .typeDirectory,
                  let info = NSDictionary(contentsOf: app.appendingPathComponent("Contents/Info.plist")) as? [String: Any],
                  info["CFBundleIdentifier"] as? String == "com.routepilot.app",
                  info["CFBundleShortVersionString"] as? String == String(release.tagName.drop(while: { !$0.isNumber })),
                  FileManager.default.isExecutableFile(atPath: app.appendingPathComponent("Contents/MacOS/RoutePilot").path) else {
                throw AppFailure(message: "升级包中的 RoutePilot.app 无效")
            }
            return app
        } catch {
            try? FileManager.default.removeItem(at: directory)
            throw error
        }
    }

    private static func download(_ url: URL, maximumBytes: Int) async throws -> Data {
        guard url.scheme == "https", url.host == host, url.user == nil, url.password == nil else {
            throw AppFailure(message: "Release 下载地址不可信")
        }
        var request = URLRequest(url: url)
        request.timeoutInterval = 180
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
              response.url?.scheme == "https", response.url?.host == host else {
            throw AppFailure(message: "Release 文件下载失败")
        }
        guard data.count <= maximumBytes else { throw AppFailure(message: "Release 文件超过大小限制") }
        return data
    }
}
