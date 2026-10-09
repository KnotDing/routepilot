import Foundation
import UserNotifications

private enum NotificationSelection {
    private struct Event: Decodable { let id: String; let name: String; let prefix: Bool }
    private static let events: [Event] = {
        guard let url = Bundle.main.url(forResource: "notification-events", withExtension: "json"),
              let data = try? Data(contentsOf: url),
              let events = try? JSONDecoder().decode([Event].self, from: data) else { return [] }
        return events
    }()
    static func selected(_ name: String, settings: [String: Any]) -> Bool {
        if let errors = settings["errors"] as? [String] {
            guard let event = events.first(where: { $0.prefix ? name.hasPrefix($0.name) : name == $0.name }) else { return false }
            return errors.contains(event.id)
        }
        let category = name.hasPrefix("Surge") ? "surge" : name.hasPrefix("BGP") || name.contains("routing") ? "bgp" : name.contains("DNS") ? "dns" : "controller"
        return settings[category] as? Bool == true
    }
}

struct AppPreferences: Codable {
    var automaticCheck = true
    var automaticDownload = false
    var updatePrompts = true
    var checkIntervalHours = 12
    var desktopNotifications = false
    var notificationSound = true

    static var current: AppPreferences {
        get {
            guard let data = UserDefaults.standard.data(forKey: "appPreferences"),
                  let preferences = try? JSONDecoder().decode(Self.self, from: data) else { return Self() }
            return preferences
        }
        set { if let data = try? JSONEncoder().encode(newValue) { UserDefaults.standard.set(data, forKey: "appPreferences") } }
    }
}

final class DesktopNotifications: NSObject, UNUserNotificationCenterDelegate {
    static let shared = DesktopNotifications()
    func prepare() { UNUserNotificationCenter.current().delegate = self }
    func authorize() async -> Bool {
        (try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound])) ?? false
    }
    func send(title: String, body: String, identifier: String) async {
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        guard settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional else { return }
        let content = UNMutableNotificationContent()
        content.title = String(title.prefix(180))
        content.body = String(body.prefix(2000))
        if AppPreferences.current.notificationSound { content.sound = .default }
        try? await center.add(UNNotificationRequest(identifier: identifier, content: content, trigger: nil))
    }
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list, .sound])
    }
}

@MainActor
final class ServiceNotifications {
    private struct Alarm { var down = false; var lastAlert = Date.distantPast }
    private var alarms: [String: [String: Alarm]] = [:]
    func reset() { alarms.removeAll() }

    func observe(scope: String, name: String, health: [[String: Any]], settings: [String: Any]) {
        guard AppPreferences.current.desktopNotifications, settings["enabled"] as? Bool == true else {
            alarms[scope] = nil
            return
        }
        let cooldown = max(0, min(86400, settings["cooldown_seconds"] as? Int ?? 300))
        let recovery = settings["recovery"] as? Bool == true
        let now = Date()
        var previous = alarms[scope] ?? [:]
        var seen = Set<String>(), failures: [String] = [], recovered: [String] = []
        for component in health.prefix(256) {
            guard let key = component["name"] as? String, let state = component["state"] as? String else { continue }
            guard NotificationSelection.selected(key, settings: settings), state != "DISABLED" else { continue }
            seen.insert(key)
            var alarm = previous[key] ?? Alarm()
            if state == "DOWN", (!alarm.down || cooldown > 0), now.timeIntervalSince(alarm.lastAlert) >= Double(cooldown) {
                failures.append(L(key)); alarm.down = true; alarm.lastAlert = now
            } else if state == "UP" {
                if alarm.down && recovery { recovered.append(L(key)) }
                alarm.down = false
            }
            previous[key] = alarm
        }
        previous = previous.filter { seen.contains($0.key) }
        alarms[scope] = previous
        for (items, title, suffix) in [(failures, "服务异常", "down"), (recovered, "服务已恢复", "up")] where !items.isEmpty {
            Task { await DesktopNotifications.shared.send(title: "RoutePilot · " + name + " · " + L(title), body: items.sorted().joined(separator: "、"), identifier: scope + ":" + suffix) }
        }
    }
}

@MainActor
final class RemoteNotificationWatch {
    let id: String
    let scope: String
    let name: String
    let client: ControllerClient
    var settings: [String: Any]
    var health: [[String: Any]] = []
    var failures = 0
    init(id: String, scope: String, name: String, base: URL, cookie: HTTPCookie, settings: [String: Any]) {
        self.id = id; self.scope = scope; self.name = name; self.settings = settings
        self.client = ControllerClient(base)
        self.client.session.configuration.httpCookieStorage?.setCookie(cookie)
    }
}
