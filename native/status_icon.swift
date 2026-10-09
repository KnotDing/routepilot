import AppKit

// Shape differences keep service status readable in light and dark menu bars.
enum StatusBarVisualState: CaseIterable, Hashable {
    case healthy, warning, offline, connecting, login

    var label: String {
        switch self {
        case .healthy: return "服务正常"
        case .warning: return "服务异常"
        case .offline: return "后台离线"
        case .connecting: return "正在连接或处理…"
        case .login: return "需要登录管理服务"
        }
    }
    var symbol: String? {
        switch self {
        case .healthy: return nil // Keep RoutePilot's own mark for normal operation.
        case .warning: return "exclamationmark.triangle.fill"
        case .offline: return "xmark.circle"
        case .connecting: return "arrow.triangle.2.circlepath"
        case .login: return "lock.fill"
        }
    }
    func image(brand: NSImage?, scale pulseScale: CGFloat = 1, rotation: CGFloat = 0) -> NSImage? {
        let source = symbol.flatMap { NSImage(systemSymbolName: $0, accessibilityDescription: nil) } ?? brand
        guard let source else { return nil }
        let configured = source.withSymbolConfiguration(NSImage.SymbolConfiguration(pointSize: 16, weight: .medium)) ?? source
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { rect in
            let size = configured.size
            guard size.width > 0, size.height > 0 else { return false }
            let scale = min(rect.width / size.width, rect.height / size.height) * pulseScale
            let target = NSRect(x: (rect.width - size.width * scale) / 2, y: (rect.height - size.height * scale) / 2, width: size.width * scale, height: size.height * scale)
            NSGraphicsContext.saveGraphicsState()
            let transform = NSAffineTransform()
            transform.translateX(by: rect.midX, yBy: rect.midY)
            transform.rotate(byDegrees: rotation)
            transform.translateX(by: -rect.midX, yBy: -rect.midY)
            transform.concat()
            configured.draw(in: target)
            NSGraphicsContext.restoreGraphicsState()
            return true
        }
        image.isTemplate = true
        return image
    }
}

// Animate the icon image, leaving the status item's size, hit area and menu
// highlight untouched. Frames are reused; animation never polls the backend.
@MainActor
final class StatusBarIconAnimator {
    private weak var button: NSStatusBarButton?
    private let brand: NSImage?
    private var visual: StatusBarVisualState = .connecting
    private var stillImages: [StatusBarVisualState: NSImage] = [:]
    private var displayedFrame: NSImage?
    private var motionEnabled = false
    private var frameCache: [StatusBarVisualState: [NSImage]] = [:]
    private var frames: [NSImage] = []
    private var timer: Timer?
    private var stopped = false
    private var accessibilityObserver: NSObjectProtocol?
    private var cycleStarted = ProcessInfo.processInfo.systemUptime

    init(button: NSStatusBarButton, brand: NSImage?) {
        self.button = button
        self.brand = brand
        accessibilityObserver = NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.accessibilityDisplayOptionsDidChangeNotification,
            object: nil, queue: .main
        ) { [weak self] _ in
            Task { @MainActor in self?.updateMotionPreference() }
        }
        setState(.connecting)
    }

    func setState(_ state: StatusBarVisualState) {
        guard state != visual || frames.isEmpty else { return }
        visual = state
        if frameCache[state] == nil { frameCache[state] = makeFrames(state) }
        frames = frameCache[state] ?? []
        cycleStarted = ProcessInfo.processInfo.systemUptime
        updateMotionPreference()
    }

    private func makeFrames(_ state: StatusBarVisualState) -> [NSImage] {
        guard let still = state.image(brand: brand) else { return [] }
        stillImages[state] = still
        switch state {
        case .connecting:
            return (0..<20).map { state.image(brand: brand, rotation: -CGFloat($0) * 18) ?? still }
        case .healthy, .warning:
            let resting = state.image(brand: brand, scale: 0.88) ?? still
            // Two soft beats, then a quiet interval. The mark never disappears.
            func beat(_ time: Double, center: Double) -> Double {
                let distance = abs(time - center) / 0.18
                return distance < 1 ? (1 + cos(distance * .pi)) / 2 : 0
            }
            return (0..<28).map { index in
                let time = Double(index) * 0.1
                let strength = max(beat(time, center: 0.2), 0.8 * beat(time, center: 0.6))
                return strength == 0 ? resting : state.image(brand: brand, scale: CGFloat(0.88 + 0.12 * strength)) ?? still
            }
        case .offline, .login:
            return [still]
        }
    }

    private func updateMotionPreference() {
        guard !stopped else { return }
        timer?.invalidate()
        timer = nil
        motionEnabled = !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion && frames.count > 1
        if !motionEnabled {
            display(stillImages[visual])
            return
        }
        tick()
        let animation = Timer(timeInterval: 0.1, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.tick() }
        }
        animation.tolerance = 0.03
        // Keep the heartbeat alive while a menu or modal window is open.
        RunLoop.main.add(animation, forMode: .common)
        RunLoop.main.add(animation, forMode: .modalPanel)
        timer = animation
    }

    private func tick() {
        guard !stopped, motionEnabled, !frames.isEmpty else { return }
        guard button != nil else { stop(); return }
        let elapsed = ProcessInfo.processInfo.systemUptime - cycleStarted
        let frame = frames[Int(max(0, elapsed) * 10) % frames.count]
        display(frame)
    }

    private func display(_ image: NSImage?) {
        guard displayedFrame !== image else { return }
        displayedFrame = image
        button?.image = image
    }

    func stop() {
        stopped = true
        motionEnabled = false
        timer?.invalidate()
        timer = nil
        if let observer = accessibilityObserver {
            NSWorkspace.shared.notificationCenter.removeObserver(observer)
            accessibilityObserver = nil
        }
    }
}
