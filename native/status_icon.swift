import AppKit

enum StatusBarTrafficBranch: CaseIterable, Hashable {
    case direct, surge
    var endpointY: CGFloat { self == .direct ? 45 : 19 }
}

// Packet routing is separate from drawing, so its probability and mid-flight
// stability can be checked with a deterministic random source.
struct StatusBarPacketFlow {
    private(set) var directProbability = 0.5
    private var cycle = -1
    private var branch: StatusBarTrafficBranch = .direct

    mutating func setTraffic(direct: Double, surge: Double) {
        let direct = direct.isFinite ? max(0, direct) : 0
        let surge = surge.isFinite ? max(0, surge) : 0
        let total = direct + surge
        directProbability = total > 0 && total.isFinite ? direct / total : 0.5
    }
    mutating func reset() { cycle = -1 }
    mutating func route(for packetCycle: Int, random: () -> Double = { Double.random(in: 0..<1) }) -> StatusBarTrafficBranch {
        if cycle != packetCycle {
            cycle = packetCycle
            branch = random() < directProbability ? .direct : .surge
        }
        return branch
    }
}

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
    func image(brand: NSImage?, rotation: CGFloat = 0) -> NSImage? {
        let source = symbol.flatMap { NSImage(systemSymbolName: $0, accessibilityDescription: nil) } ?? brand
        guard let source else { return nil }
        let configured = source.withSymbolConfiguration(NSImage.SymbolConfiguration(pointSize: 16, weight: .medium)) ?? source
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { rect in
            let size = configured.size
            guard size.width > 0, size.height > 0 else { return false }
            let scale = min(rect.width / size.width, rect.height / size.height)
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

    // Each packet enters from the left and follows exactly one branch.
    // Only the packet and its trail move; the mark's geometry stays fixed.
    static func flowImage(progress: Double, branch: StatusBarTrafficBranch) -> NSImage {
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { rect in
            let unit = min(rect.width, rect.height) / 18
            func point(_ x: CGFloat, _ y: CGFloat) -> NSPoint {
                NSPoint(x: rect.minX + (1 + (x - 12) * 0.4) * unit,
                        y: rect.minY + (1 + (y - 12) * 0.4) * unit)
            }
            let path = NSBezierPath()
            path.lineWidth = 1.7 * unit
            path.lineCapStyle = .round
            path.lineJoinStyle = .round
            path.move(to: point(17, 32)); path.line(to: point(27, 32))
            for y in [CGFloat(45), CGFloat(19)] {
                path.move(to: point(27, 32))
                path.curve(to: point(42, y), controlPoint1: point(35, 32), controlPoint2: point(34, y))
                path.line(to: point(47, y))
            }
            NSColor.black.withAlphaComponent(0.30).setStroke()
            path.stroke()
            NSColor.black.withAlphaComponent(0.55).setFill()
            for (x, y) in [(CGFloat(17), CGFloat(32)), (47, 45), (47, 19)] {
                let center = point(x, y), radius = 2 * unit
                NSBezierPath(ovalIn: NSRect(x: center.x - radius, y: center.y - radius,
                                           width: radius * 2, height: radius * 2)).fill()
            }
            func packetPoint(_ time: Double, branchY: CGFloat) -> NSPoint {
                if time < 0.25 { return point(17 + 10 * CGFloat(time / 0.25), 32) }
                let branch = CGFloat((time - 0.25) / 0.75)
                if branch >= 0.88 { return point(42 + 5 * ((branch - 0.88) / 0.12), branchY) }
                let t = branch / 0.88, inverse = 1 - t
                let x = inverse * inverse * inverse * 27 + 3 * inverse * inverse * t * 35
                    + 3 * inverse * t * t * 34 + t * t * t * 42
                let y = inverse * inverse * inverse * 32 + 3 * inverse * inverse * t * 32
                    + 3 * inverse * t * t * branchY + t * t * t * branchY
                return point(x, y)
            }
            // Draw the faint tail first, then the brighter moving head.
            for segment in (0..<7).reversed() {
                let time = progress - Double(segment) * 0.025
                guard time >= 0 else { continue }
                NSColor.black.withAlphaComponent(1 - CGFloat(segment) * 0.10).setStroke()
                let trail = NSBezierPath()
                trail.lineWidth = 2.0 * unit
                trail.lineCapStyle = .round
                trail.move(to: packetPoint(max(0, time - 0.025), branchY: branch.endpointY))
                trail.line(to: packetPoint(time, branchY: branch.endpointY))
                trail.stroke()
            }
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
    private var flowFrames: [StatusBarTrafficBranch: [NSImage]] = [:]
    private var packetFlow = StatusBarPacketFlow()
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
        packetFlow.reset()
        updateMotionPreference()
    }

    func setTraffic(direct: Double, surge: Double) {
        packetFlow.setTraffic(direct: direct, surge: surge)
        // Updated odds apply to the next packet, never redirect one mid-flight.
    }

    private func makeFrames(_ state: StatusBarVisualState) -> [NSImage] {
        guard let still = state.image(brand: brand) else { return [] }
        stillImages[state] = still
        switch state {
        case .connecting:
            return (0..<20).map { state.image(brand: brand, rotation: -CGFloat($0) * 18) ?? still }
        case .healthy:
            for branch in StatusBarTrafficBranch.allCases {
                flowFrames[branch] = (0..<24).map {
                    StatusBarVisualState.flowImage(progress: Double($0) / 23, branch: branch)
                }
            }
            return flowFrames[.direct] ?? [still]
        case .warning, .offline, .login:
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
        // Keep the flow animation running while a menu or modal window is open.
        RunLoop.main.add(animation, forMode: .common)
        RunLoop.main.add(animation, forMode: .modalPanel)
        timer = animation
    }

    private func tick() {
        guard !stopped, motionEnabled, !frames.isEmpty else { return }
        guard button != nil else { stop(); return }
        let elapsed = ProcessInfo.processInfo.systemUptime - cycleStarted
        let step = Int(max(0, elapsed) * 10), index = step % frames.count
        let frame: NSImage
        if visual == .healthy {
            let cycle = step / frames.count
            let packetBranch = packetFlow.route(for: cycle)
            frame = (flowFrames[packetBranch] ?? frames)[index]
        } else {
            frame = frames[index]
        }
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
