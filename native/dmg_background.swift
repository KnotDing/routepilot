import AppKit
import CoreGraphics

// Finder-space layout: 660x420 window bounds, icons centered at (180, 165)
// and (480, 165) with 128px icons. Keep the vector artwork aligned with the
// positions stored in scripts/dmg/DS_Store.
let width: CGFloat = 660
let height: CGFloat = 420
func finderY(_ y: CGFloat) -> CGFloat { height - y }

let outputURL = URL(fileURLWithPath: CommandLine.arguments[1]) as CFURL
var mediaBox = CGRect(x: 0, y: 0, width: width, height: height)
guard let pdf = CGContext(outputURL, mediaBox: &mediaBox, nil) else {
    fatalError("Could not create DMG background PDF")
}
pdf.beginPDFPage(nil)
let graphics = NSGraphicsContext(cgContext: pdf, flipped: false)
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = graphics

let canvas = NSRect(x: 0, y: 0, width: width, height: height)
NSColor(calibratedRed: 0.95, green: 0.962, blue: 0.98, alpha: 1).setFill()
NSBezierPath(rect: canvas).fill()
let brand = NSColor(calibratedRed: 0.216, green: 0.412, blue: 0.929, alpha: 1)
let accent = NSColor(calibratedRed: 0.98, green: 0.63, blue: 0.26, alpha: 1)
let arrowY = finderY(165)
let arrow = NSBezierPath()
arrow.lineWidth = 8
arrow.lineCapStyle = .round
arrow.move(to: NSPoint(x: 264, y: arrowY - 8))
arrow.curve(to: NSPoint(x: 378, y: arrowY + 2), controlPoint1: NSPoint(x: 294, y: arrowY + 42), controlPoint2: NSPoint(x: 345, y: arrowY + 46))
brand.setStroke()
arrow.stroke()

// Angle the arrowhead along the curve's final tangent.
let head = NSBezierPath()
head.move(to: NSPoint(x: 390, y: arrowY - 15))
head.line(to: NSPoint(x: 387, y: arrowY + 8))
head.line(to: NSPoint(x: 369, y: arrowY - 4))
head.close()
brand.setFill()
head.fill()

// A couple of light motion dots and a small sparkle make the gesture feel less mechanical.
func dot(_ x: CGFloat, _ y: CGFloat, radius: CGFloat, color: NSColor) {
    color.setFill()
    NSBezierPath(ovalIn: NSRect(x: x - radius, y: y - radius, width: radius * 2, height: radius * 2)).fill()
}
dot(254, arrowY - 13, radius: 3.5, color: NSColor(calibratedRed: 0.48, green: 0.66, blue: 0.98, alpha: 1))
dot(246, arrowY - 19, radius: 2, color: NSColor(calibratedRed: 0.68, green: 0.79, blue: 1, alpha: 1))
dot(354, arrowY + 38, radius: 3, color: NSColor(calibratedRed: 0.36, green: 0.76, blue: 0.72, alpha: 1))
let sparkle = NSBezierPath()
sparkle.move(to: NSPoint(x: 326, y: arrowY + 55))
sparkle.line(to: NSPoint(x: 329, y: arrowY + 46))
sparkle.line(to: NSPoint(x: 338, y: arrowY + 43))
sparkle.line(to: NSPoint(x: 329, y: arrowY + 40))
sparkle.line(to: NSPoint(x: 326, y: arrowY + 31))
sparkle.line(to: NSPoint(x: 323, y: arrowY + 40))
sparkle.line(to: NSPoint(x: 314, y: arrowY + 43))
sparkle.line(to: NSPoint(x: 323, y: arrowY + 46))
sparkle.close()
accent.setFill()
sparkle.fill()

func centered(_ text: String, finderCenterY: CGFloat, font: NSFont, color: NSColor) {
    let attributes: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color]
    let size = (text as NSString).size(withAttributes: attributes)
    (text as NSString).draw(at: NSPoint(x: (width - size.width) / 2, y: finderY(finderCenterY) - size.height / 2), withAttributes: attributes)
}
centered("将 RoutePilot.app 拖入 Applications 安装", finderCenterY: 315, font: .systemFont(ofSize: 22, weight: .semibold), color: NSColor(calibratedWhite: 0.16, alpha: 1))
centered("Drag RoutePilot.app into Applications to install", finderCenterY: 349, font: .systemFont(ofSize: 14), color: NSColor(calibratedWhite: 0.42, alpha: 1))

NSGraphicsContext.restoreGraphicsState()
pdf.endPDFPage()
pdf.closePDF()
