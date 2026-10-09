import AppKit
import Darwin
import Foundation

struct UpdateAssistantStatus: Decodable {
    let ready: Bool?
    let phase: String?
    let target: String?
    let error: String?
}

enum UpdateAssistant {
    static let socketPath = "/var/run/com.routepilot.updater.sock"
    static let appPath = "/Applications/RoutePilot.app"
    static var configured: Bool {
        var info = stat()
        return lstat(socketPath, &info) == 0 && info.st_mode & S_IFMT == S_IFSOCK && info.st_uid == getuid() && info.st_mode & 0o077 == 0
    }
    static func request(_ action: String, version: String? = nil) async throws -> UpdateAssistantStatus {
        try await Task.detached {
            guard configured else { throw AppFailure(message: "更新助手尚未启用，请先授权安装更新助手") }
            let fd = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
            guard fd >= 0 else { throw AppFailure(message: "无法连接更新助手") }
            defer { Darwin.close(fd) }
            var timeout = timeval(tv_sec: 30, tv_usec: 0)
            _ = setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout.size(ofValue: timeout)))
            _ = setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, socklen_t(MemoryLayout.size(ofValue: timeout)))
            var noSignal: Int32 = 1
            _ = setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &noSignal, socklen_t(MemoryLayout.size(ofValue: noSignal)))
            var address = sockaddr_un()
            address.sun_family = sa_family_t(AF_UNIX)
            address.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
            let path = Array(socketPath.utf8) + [UInt8(0)]
            withUnsafeMutableBytes(of: &address.sun_path) { $0.copyBytes(from: path) }
            let connected = withUnsafePointer(to: &address) { ptr in
                ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { Darwin.connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size)) }
            }
            var uid: uid_t = 1, gid: gid_t = 1
            guard connected == 0, getpeereid(fd, &uid, &gid) == 0, uid == 0 else { throw AppFailure(message: "无法连接可信的更新助手") }
            var payload = ["action": action]
            if let version { payload["version"] = version }
            var data = try JSONSerialization.data(withJSONObject: payload)
            data.append(10)
            try data.withUnsafeBytes { bytes in
                var sent = 0
                while sent < bytes.count {
                    let count = Darwin.write(fd, bytes.baseAddress!.advanced(by: sent), bytes.count - sent)
                    guard count > 0 else { throw AppFailure(message: "更新助手请求发送失败") }
                    sent += count
                }
            }
            var response = Data(), buffer = [UInt8](repeating: 0, count: 4096)
            while response.count < 65536 {
                let count = Darwin.read(fd, &buffer, buffer.count)
                guard count > 0 else { throw AppFailure(message: "更新助手没有响应，请重试") }
                response.append(contentsOf: buffer.prefix(count))
                if response.contains(10) { break }
            }
            guard response.count < 65536 else { throw AppFailure(message: "更新助手响应无效") }
            let status = try JSONDecoder().decode(UpdateAssistantStatus.self, from: response)
            if status.ready != true { throw AppFailure(message: status.error ?? "更新助手响应无效") }
            return status
        }.value
    }
    @MainActor static func disable() async throws {
        guard Bundle.main.bundleURL.path == appPath else { throw AppFailure(message: "请从 /Applications 中的 RoutePilot 停用更新助手") }
        let executable = appPath + "/Contents/Resources/Service/bin/surge-controller"
        let command = "'" + executable + "' uninstall-update-agent"
        let escaped = command.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
        var failure: NSDictionary?
        guard NSAppleScript(source: "do shell script \"\(escaped)\" with administrator privileges")?.executeAndReturnError(&failure) != nil else {
            throw AppFailure(message: failure?[NSAppleScript.errorMessage] as? String ?? "更新助手未停用")
        }
    }
    @MainActor static func enable() async throws {
        guard Bundle.main.bundleURL.path == appPath else { throw AppFailure(message: "请先把 RoutePilot.app 移到 /Applications，再启用更新助手") }
        let executable = appPath + "/Contents/Resources/Service/bin/surge-controller"
        let command = "'" + executable + "' install-update-agent " + String(getuid())
        let escaped = command.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
        var failure: NSDictionary?
        guard NSAppleScript(source: "do shell script \"\(escaped)\" with administrator privileges")?.executeAndReturnError(&failure) != nil else {
            throw AppFailure(message: failure?[NSAppleScript.errorMessage] as? String ?? "更新助手未安装")
        }
        for _ in 0..<10 {
            if configured, let status = try? await request("status"), status.ready == true { return }
            try await Task.sleep(nanoseconds: 1_000_000_000)
        }
        throw AppFailure(message: "更新助手未启动，请重试启用")
    }
}
