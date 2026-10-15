import AppKit

// Notifications reference the main App's connection type. This standalone test
// never creates a connection or invokes the restart/notification workflow.
final class ControllerClient {
    let session = URLSession(configuration: .ephemeral)
    init(_ base: URL) {}
}

@main
struct RemoteRestartTests {
    @MainActor
    static func main() throws {
        let before = Date(timeIntervalSince1970: 1000)
        let installed = "/Applications/RoutePilot.app/Contents/MacOS/RoutePilot"
        let previous = "/Applications/.RoutePilot-update-123/previous.app/Contents/MacOS/RoutePilot"
        for (path, launched, terminated, pid, expected) in [
            (installed, Optional(before.addingTimeInterval(-1)), false, pid_t(10), true),
            (previous, Optional(before.addingTimeInterval(-1)), false, pid_t(10), true),
            (installed, Optional(before), false, pid_t(10), false),
            (installed, Optional(before.addingTimeInterval(1)), false, pid_t(10), false),
            (installed, nil, false, pid_t(10), false),
            (installed, Optional(before.addingTimeInterval(-1)), true, pid_t(10), false),
            (installed, Optional(before.addingTimeInterval(-1)), false, pid_t(20), false),
            ("/tmp/RoutePilot.app/Contents/MacOS/RoutePilot", Optional(before.addingTimeInterval(-1)), false, pid_t(10), false),
            ("/Applications/Other.app/Contents/MacOS/RoutePilot", Optional(before.addingTimeInterval(-1)), false, pid_t(10), false)
        ] {
            let actual = RemoteUpdateRestart.matchesProcess(pid: pid, terminated: terminated, launched: launched, path: path, before: before, helperPID: 20)
            try nativeCheck(actual == expected, "App restart selected an unrelated, new or unknown process")
        }
        var descriptors: [Int32] = [0, 0]
        try nativeCheck(pipe(&descriptors) == 0, "Cannot create isolated helper lifetime pipe")
        let ended = DispatchSemaphore(value: 0)
        let lifetime = RemoteUpdateRestart.backendLifetime(fileDescriptor: descriptors[0], onEnd: { ended.signal() })
        close(descriptors[1])
        // Blocks the main thread like a remote modal; EOF must still cancel.
        try nativeCheck(ended.wait(timeout: .now() + 2) == .success, "Waiting helper did not observe backend cancellation")
        lifetime.cancel()
        close(descriptors[0])
        print("PASS native remote restart: exact App paths, launch cutoff, helper exclusion and cancellation during a blocked main thread")
    }
}
