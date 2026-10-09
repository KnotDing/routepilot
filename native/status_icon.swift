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
    func image(brand: NSImage?) -> NSImage? {
        let source = symbol.flatMap { NSImage(systemSymbolName: $0, accessibilityDescription: nil) } ?? brand
        guard let source else { return nil }
        let configured = source.withSymbolConfiguration(NSImage.SymbolConfiguration(pointSize: 16, weight: .medium)) ?? source
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { rect in
            let size = configured.size
            guard size.width > 0, size.height > 0 else { return false }
            let scale = min(rect.width / size.width, rect.height / size.height)
            let target = NSRect(x: (rect.width - size.width * scale) / 2, y: (rect.height - size.height * scale) / 2, width: size.width * scale, height: size.height * scale)
            configured.draw(in: target)
            return true
        }
        image.isTemplate = true
        return image
    }
}
