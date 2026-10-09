import AppKit

let directory = CommandLine.arguments[1]
func render(_ pixels: Int, background: Bool) throws -> Data {
    let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    let context = NSGraphicsContext(bitmapImageRep: bitmap)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = context
    let scale = NSAffineTransform(); scale.scale(by: CGFloat(pixels) / 64); scale.concat()
    if !background {
        let zoom = NSAffineTransform(); zoom.translateX(by: -11.2, yBy: -11.2); zoom.scale(by: 1.35); zoom.concat()
    }
    if background {
        let shape = NSBezierPath(roundedRect: NSRect(x: 1, y: 1, width: 62, height: 62), xRadius: 15, yRadius: 15)
        NSGradient(starting: NSColor(calibratedRed: 0.216, green: 0.412, blue: 0.929, alpha: 1), ending: NSColor(calibratedRed: 0.145, green: 0.282, blue: 0.741, alpha: 1))!.draw(in: shape, angle: -45)
    }
    let path = NSBezierPath(); path.lineWidth = background ? 4.5 : 5.5; path.lineCapStyle = .round; path.lineJoinStyle = .round
    path.move(to: NSPoint(x: 17, y: 32)); path.line(to: NSPoint(x: 27, y: 32))
    path.curve(to: NSPoint(x: 42, y: 45), controlPoint1: NSPoint(x: 35, y: 32), controlPoint2: NSPoint(x: 34, y: 45)); path.line(to: NSPoint(x: 47, y: 45))
    path.move(to: NSPoint(x: 27, y: 32)); path.curve(to: NSPoint(x: 42, y: 19), controlPoint1: NSPoint(x: 35, y: 32), controlPoint2: NSPoint(x: 34, y: 19)); path.line(to: NSPoint(x: 47, y: 19))
    (background ? NSColor.white : NSColor.black).setStroke(); path.stroke()
    for (x,y) in [(17.0,32.0),(47.0,45.0),(47.0,19.0)] {
        let color = background && y == 45 ? NSColor(calibratedRed: 0.541, green: 0.949, blue: 0.859, alpha: 1) : background ? .white : NSColor.black
        color.setFill(); NSBezierPath(ovalIn: NSRect(x: x-5, y: y-5, width: 10, height: 10)).fill()
    }
    NSGraphicsContext.restoreGraphicsState()
    return bitmap.representation(using: .png, properties: [:])!
}
for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let suffix = scale == 2 ? "@2x" : ""
        try render(size * scale, background: true).write(to: URL(fileURLWithPath: directory + "/icon_\(size)x\(size)\(suffix).png"))
    }
}
if CommandLine.arguments.count > 2 { try render(40, background: false).write(to: URL(fileURLWithPath: CommandLine.arguments[2])) }
