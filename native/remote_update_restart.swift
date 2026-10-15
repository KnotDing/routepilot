import AppKit
import CoreGraphics
import Darwin
import UserNotifications

// Runs only in the existing user's GUI session, after the backend commits the
// complete update. It never starts a previously closed App or creates a session.
@MainActor
enum RemoteUpdateRestart {
    static let installedPath = "/Applications/RoutePilot.app"

    static func matches(_ app: NSRunningApplication, before: Date) -> Bool {
        matchesProcess(pid: app.processIdentifier, terminated: app.isTerminated, launched: app.launchDate,
                       path: app.executableURL?.path, before: before)
    }

    static func matchesProcess(pid: pid_t, terminated: Bool, launched: Date?, path: String?, before: Date, helperPID: pid_t = getpid()) -> Bool {
        guard pid != helperPID, !terminated, let launched, launched < before, let path else { return false }
        return path == installedPath + "/Contents/MacOS/RoutePilot"
            || (path.hasPrefix("/Applications/.RoutePilot-update-") && path.hasSuffix("/previous.app/Contents/MacOS/RoutePilot"))
    }

    static func applications(before: Date) -> [NSRunningApplication] {
        NSRunningApplication.runningApplications(withBundleIdentifier: "com.routepilot.app").filter { matches($0, before: before) }
    }

    static func inUse(_ apps: [NSRunningApplication]) -> Bool {
        if apps.contains(where: { $0.isActive }) { return true }
        // Window owner, layer and bounds do not require screen capture. Treat a
        // missing inventory conservatively: ask instead of closing an unknown UI.
        guard let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else { return true }
        let pids = Set(apps.map { $0.processIdentifier })
        return windows.contains { window in
            guard let pid = window[kCGWindowOwnerPID as String] as? NSNumber,
                  pids.contains(pid.int32Value), (window[kCGWindowLayer as String] as? NSNumber)?.intValue == 0,
                  let bounds = window[kCGWindowBounds as String] as? [String: Any],
                  let width = bounds["Width"] as? NSNumber, let height = bounds["Height"] as? NSNumber else { return false }
            return width.doubleValue > 100 && height.doubleValue > 60
        }
    }

    static func managementOpen(_ apps: [NSRunningApplication]) -> Bool {
        if UserDefaults.standard.object(forKey: ManagementWindowFrame.openKey) != nil {
            return ManagementWindowFrame.isOpen
        }
        // A running App may predate window-state recording. Its visible main
        // window can still be restored on the first upgrade to this version.
        guard let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else { return false }
        let pids = Set(apps.map { $0.processIdentifier })
        return windows.contains { window in
            guard let pid = window[kCGWindowOwnerPID as String] as? NSNumber, pids.contains(pid.int32Value),
                  (window[kCGWindowLayer as String] as? NSNumber)?.intValue == 0,
                  let bounds = window[kCGWindowBounds as String] as? [String: Any],
                  let width = bounds["Width"] as? NSNumber, let height = bounds["Height"] as? NSNumber else { return false }
            return width.doubleValue >= 420 && height.doubleValue >= 480
        }
    }

    static func report(_ phase: String, _ error: String? = nil) {
        var value = ["phase": phase]
        if let error { value["error"] = error }
        if let data = try? JSONSerialization.data(withJSONObject: value) {
            FileHandle.standardOutput.write(data)
            FileHandle.standardOutput.write(Data([10]))
        }
    }

    static func backendLifetime(fileDescriptor: Int32 = STDIN_FILENO, onEnd: @escaping @Sendable () -> Void = { _exit(0) }) -> DispatchSourceRead {
        let source = DispatchSource.makeReadSource(fileDescriptor: fileDescriptor, queue: .global())
        source.setEventHandler {
            var byte: UInt8 = 0
            let count = Darwin.read(fileDescriptor, &byte, 1)
            if count == 0 || (count < 0 && errno != EINTR && errno != EAGAIN) { onEnd() }
        }
        source.resume()
        return source
    }

    static func run() async {
        guard getuid() != 0, Bundle.main.bundleURL.path == installedPath,
              let index = CommandLine.arguments.firstIndex(of: "--restart-before"), index + 1 < CommandLine.arguments.count,
              let seconds = Double(CommandLine.arguments[index + 1]), seconds.isFinite, seconds > 0 else {
            report("failed", "Invalid remote App restart request")
            return
        }
        // The backend owns the input pipe. Cancel a superseded confirmation
        // even while NSAlert is running its nested modal loop.
        let lifetime = backendLifetime()
        defer { lifetime.cancel() }
        let force = CommandLine.arguments.contains("--force-app-restart")
        let before = Date(timeIntervalSince1970: seconds)
        var apps = applications(before: before)
        guard !apps.isEmpty else { report("not_running"); return }
        let wasInUse = inUse(apps)
        let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
        DesktopNotifications.shared.prepare()
        let authorization = await UNUserNotificationCenter.current().notificationSettings()
        if !force && authorization.authorizationStatus == .notDetermined { _ = await DesktopNotifications.shared.authorize() }
        await DesktopNotifications.shared.send(title: L("RoutePilot 已更新至 {version}", ["version": version]),
            body: L(force ? "管理端已确认强制重启 App，即将关闭并重新打开管理窗口。网络服务继续运行。" : wasInUse ? "管理窗口正在使用，请在远端确认是否重启 App。网络服务已更新。" : "App 正在后台运行，即将自动重启以加载新版本。网络服务已更新。"),
            identifier: "routepilot:remote-update:" + version)
        // Allow the notification to appear and recheck in case the user opened
        // the window during the handoff. Confirmation has no automatic approval.
        try? await Task.sleep(nanoseconds: 2_000_000_000)
        apps = applications(before: before)
        guard !apps.isEmpty else { report("not_running"); return }
        if !force && (wasInUse || inUse(apps)) {
            report("awaiting_confirmation")
            let alert = NSAlert()
            alert.messageText = L("远端应用已更新")
            alert.informativeText = L("RoutePilot 已更新至 {version}。管理窗口正在使用，是否现在重启 App？选择稍后可继续使用当前窗口。网络服务已更新，不会再次重启。", ["version": version])
            alert.addButton(withTitle: L("立即重启")).keyEquivalent = ""
            alert.addButton(withTitle: L("稍后重启")).keyEquivalent = "\r"
            NSApp.activate(ignoringOtherApps: true)
            guard alert.runModal() == .alertFirstButtonReturn else { report("deferred"); return }
        }
        // Recheck exact identities after the dialog; a manually reopened new
        // process is excluded by launch time and is never stopped.
        apps = applications(before: before)
        guard !apps.isEmpty else { report("not_running"); return }
        let reopenManagement = managementOpen(apps)
        report("restarting")
        for app in apps where !app.terminate() && (!force || !app.forceTerminate()) {
            report("failed", "Remote App refused to quit; reopen RoutePilot on the remote Mac")
            return
        }
        let deadline = Date().addingTimeInterval(35)
        let forceAfter = Date().addingTimeInterval(3)
        while apps.contains(where: { !$0.isTerminated }) {
            guard Date() < deadline else {
                report("failed", "Remote App did not quit; reopen RoutePilot on the remote Mac")
                return
            }
            if force && Date() >= forceAfter {
                for app in apps where !app.isTerminated { _ = app.forceTerminate() }
            }
            try? await Task.sleep(nanoseconds: 250_000_000)
        }
        // Avoid duplicates if the user independently reopened the new App.
        if NSRunningApplication.runningApplications(withBundleIdentifier: "com.routepilot.app").contains(where: {
            $0.processIdentifier != getpid() && !$0.isTerminated && $0.executableURL?.path == installedPath + "/Contents/MacOS/RoutePilot"
        }) { report("restarted"); return }
        do {
            let configuration = NSWorkspace.OpenConfiguration()
            configuration.createsNewApplicationInstance = true
            configuration.activates = reopenManagement
            configuration.arguments = reopenManagement ? ["--open-management"] : []
            let app = try await NSWorkspace.shared.openApplication(at: URL(fileURLWithPath: installedPath), configuration: configuration)
            guard app.processIdentifier != getpid(), !app.isTerminated else {
                report("failed", "Remote App did not relaunch; reopen RoutePilot on the remote Mac")
                return
            }
            report("restarted")
        } catch { report("failed", "Remote App did not relaunch; reopen RoutePilot on the remote Mac") }
    }
}
