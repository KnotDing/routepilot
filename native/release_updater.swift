import CryptoKit
import Foundation

private final class ReleaseDownloadDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(request.url.map(ReleaseUpdater.trustedDownloadURL) == true ? request : nil)
    }
}

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
    let body: String?
    let draft: Bool?
    let prerelease: Bool?
    enum CodingKeys: String, CodingKey {
        case tagName = "tag_name"
        case assets, body, draft, prerelease
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
        let session = URLSession(configuration: .ephemeral, delegate: ReleaseDownloadDelegate(), delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
              response.url.map(trustedDownloadURL) == true, data.count <= 2 * 1024 * 1024 else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 0
            throw AppFailure(message: L("获取最新版本失败（HTTP {code}）", ["code": code]))
        }
        let release = try JSONDecoder().decode(GiteaRelease.self, from: data)
        guard release.draft != true, release.prerelease != true, stableVersion(release.tagName) != nil else {
            throw AppFailure(message: "更新服务返回了无效的正式版本信息")
        }
        return release
    }

    static func stableVersion(_ raw: String) -> [Int]? {
        let value = raw.hasPrefix("v") ? String(raw.dropFirst()) : raw
        let parts = value.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 3 else { return nil }
        let values = parts.compactMap { part -> Int? in
            guard part.allSatisfy({ $0.isASCII && $0.isNumber }), let number = Int(part), number >= 0,
                  String(number) == part else { return nil }
            return number
        }
        return values.count == 3 ? values : nil
    }
    static func newer(_ release: GiteaRelease,
                      current: String = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0") -> Bool {
        guard release.draft != true, release.prerelease != true,
              let latest = stableVersion(release.tagName), let installed = stableVersion(current) else { return false }
        for index in 0..<3 {
            if latest[index] != installed[index] { return latest[index] > installed[index] }
        }
        return false
    }

    static func trustedDownloadURL(_ url: URL) -> Bool {
        url.scheme == "https" && url.host == host && (url.port == nil || url.port == 443)
            && url.user == nil && url.password == nil && url.fragment == nil
    }
    static func checksum(_ text: String, archiveName: String) throws -> String {
        let matches = text.split(whereSeparator: \.isNewline).compactMap { line -> String? in
            let columns = line.split(whereSeparator: \.isWhitespace)
            guard columns.count == 2 else { return nil }
            let name = columns[1].hasPrefix("*") ? columns[1].dropFirst() : columns[1][...]
            return name == archiveName ? String(columns[0]).lowercased() : nil
        }
        guard matches.count == 1, let expected = matches.first, expected.count == 64,
              expected.allSatisfy({ "0123456789abcdef".contains($0) }) else {
            throw AppFailure(message: "Release 校验文件中没有本机安装包的 SHA-256")
        }
        return expected
    }

    static func downloadPackage(from release: GiteaRelease) async throws -> URL {
        #if arch(arm64)
        let archiveName = "RoutePilot-macos-arm64.zip"
        #else
        let archiveName = "RoutePilot-macos-x86_64.zip"
        #endif
        let archives = release.assets.filter { $0.name == archiveName }, sums = release.assets.filter { $0.name == "SHA256SUMS" }
        guard archives.count == 1, sums.count == 1, let archive = archives.first, let checksums = sums.first else {
            throw AppFailure(message: L("此版本没有适用于本机的安装包"))
        }
        let checksumData = try await download(checksums.browserDownloadURL, maximumBytes: 1024 * 1024)
        guard let checksumText = String(data: checksumData, encoding: .utf8) else {
            throw AppFailure(message: "Release 校验文件中没有本机安装包的 SHA-256")
        }
        let expected = try checksum(checksumText, archiveName: archiveName)
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
        guard trustedDownloadURL(url) else {
            throw AppFailure(message: "Release 下载地址不可信")
        }
        var request = URLRequest(url: url)
        request.timeoutInterval = 180
        let session = URLSession(configuration: .ephemeral, delegate: ReleaseDownloadDelegate(), delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode),
              response.url.map(trustedDownloadURL) == true else {
            throw AppFailure(message: "Release 文件下载失败")
        }
        guard data.count <= maximumBytes else { throw AppFailure(message: "Release 文件超过大小限制") }
        return data
    }
}
