// render-appicon.swift — one-off generator for the app icon and README art.
//
// Renders the lanfirst logomark (the same geometric house + three-node network
// as the menu-bar icons) in white on a macOS-style rounded-rect gradient tile,
// then packages it as:
//
//   assets/AppIcon.icns            app icon for lanfirst.app (via iconutil)
//   assets/logo.png                256px tile for the README header
//   assets/readme/state-<name>.png 64px tiles of the four menu-bar states, so
//                                  the README can show them on GitHub's light
//                                  AND dark themes (the committed template PNGs
//                                  are black-on-transparent and vanish on dark)
//
// Glyph geometry is copied from assets/render-icons.swift — keep the two in
// sync when the mark changes. Run once after changing the glyphs or colors:
//     swift assets/render-appicon.swift

import AppKit

// ---- Brand colors (tweak here) ----
let gradientTop = CGColor(red: 0.09, green: 0.22, blue: 0.38, alpha: 1)    // deep navy
let gradientBottom = CGColor(red: 0.05, green: 0.55, blue: 0.52, alpha: 1) // teal
let stateTileColor = CGColor(red: 0.16, green: 0.19, blue: 0.25, alpha: 1) // slate
let ink = CGColor(gray: 1, alpha: 1)                                       // white glyph

// ---- Glyph geometry, unit square (0..1, y-up) — keep in sync with render-icons.swift ----
let houseBaseY: CGFloat = 0.07
let houseEaveY: CGFloat = 0.50
let houseApex = CGPoint(x: 0.50, y: 0.93)
let houseLeftX: CGFloat = 0.10
let houseRightX: CGFloat = 0.90
let nodeApex = CGPoint(x: 0.50, y: 0.60)
let nodeLeft = CGPoint(x: 0.30, y: 0.235)
let nodeRight = CGPoint(x: 0.70, y: 0.235)
let nodeR: CGFloat = 0.095
let linkW: CGFloat = 0.075

func pt(_ p: CGPoint, _ S: CGFloat) -> CGPoint { CGPoint(x: p.x * S, y: p.y * S) }

func circleRect(_ c: CGPoint, _ r: CGFloat, _ S: CGFloat) -> CGRect {
    CGRect(x: (c.x - r) * S, y: (c.y - r) * S, width: 2 * r * S, height: 2 * r * S)
}

func housePath(_ S: CGFloat) -> CGPath {
    let p = CGMutablePath()
    p.move(to: pt(CGPoint(x: houseLeftX, y: houseBaseY), S))
    p.addLine(to: pt(CGPoint(x: houseLeftX, y: houseEaveY), S))
    p.addLine(to: pt(houseApex, S))
    p.addLine(to: pt(CGPoint(x: houseRightX, y: houseEaveY), S))
    p.addLine(to: pt(CGPoint(x: houseRightX, y: houseBaseY), S))
    p.closeSubpath()
    return p
}

func drawSolidHouse(_ ctx: CGContext, _ S: CGFloat) {
    ctx.setFillColor(ink)
    ctx.setStrokeColor(ink)
    ctx.addPath(housePath(S))
    ctx.fillPath()
    ctx.addPath(housePath(S))
    ctx.setLineWidth(0.05 * S)
    ctx.setLineJoin(.round)
    ctx.strokePath()
}

func carveNetwork(_ ctx: CGContext, _ S: CGFloat) {
    ctx.setBlendMode(.clear)
    ctx.setLineWidth(linkW * S)
    ctx.setLineCap(.round)
    ctx.move(to: pt(nodeLeft, S))
    ctx.addLine(to: pt(nodeApex, S))
    ctx.addLine(to: pt(nodeRight, S))
    ctx.strokePath()
    for n in [nodeApex, nodeLeft, nodeRight] {
        ctx.fillEllipse(in: circleRect(n, nodeR, S))
    }
    ctx.setBlendMode(.normal)
}

func drawOn(_ ctx: CGContext, _ S: CGFloat) {
    drawSolidHouse(ctx, S)
    carveNetwork(ctx, S)
}

func drawPublic(_ ctx: CGContext, _ S: CGFloat) {
    ctx.setStrokeColor(ink)
    ctx.setLineWidth(0.085 * S)
    ctx.setLineJoin(.round)
    ctx.addPath(housePath(S))
    ctx.strokePath()
    ctx.setFillColor(ink)
    for n in [nodeApex, nodeLeft, nodeRight] {
        ctx.fillEllipse(in: circleRect(n, 0.085, S))
    }
}

func drawOff(_ ctx: CGContext, _ S: CGFloat) {
    drawOn(ctx, S)
    ctx.setBlendMode(.clear)
    ctx.setLineWidth(0.20 * S)
    ctx.setLineCap(.butt)
    ctx.move(to: pt(CGPoint(x: 0.04, y: 0.96), S))
    ctx.addLine(to: pt(CGPoint(x: 0.96, y: 0.04), S))
    ctx.strokePath()
    ctx.setBlendMode(.normal)
    ctx.setStrokeColor(ink)
    ctx.setLineWidth(0.085 * S)
    ctx.setLineCap(.round)
    ctx.move(to: pt(CGPoint(x: 0.10, y: 0.90), S))
    ctx.addLine(to: pt(CGPoint(x: 0.90, y: 0.10), S))
    ctx.strokePath()
}

func drawError(_ ctx: CGContext, _ S: CGFloat) {
    drawOn(ctx, S)
    let c = CGPoint(x: 0.80, y: 0.20)
    ctx.setBlendMode(.clear)
    ctx.fillEllipse(in: circleRect(c, 0.25, S))
    ctx.setBlendMode(.normal)
    ctx.setFillColor(ink)
    ctx.fillEllipse(in: circleRect(c, 0.19, S))
    ctx.setBlendMode(.clear)
    ctx.fill(CGRect(x: (c.x - 0.03) * S, y: 0.175 * S, width: 0.06 * S, height: 0.14 * S))
    ctx.fillEllipse(in: circleRect(CGPoint(x: c.x, y: 0.10), 0.034, S))
    ctx.setBlendMode(.normal)
}

func drawGlyph(_ name: String, _ ctx: CGContext, _ S: CGFloat) {
    switch name {
    case "on": drawOn(ctx, S)
    case "public": drawPublic(ctx, S)
    case "off": drawOff(ctx, S)
    case "error": drawError(ctx, S)
    default: fatalError("unknown glyph \(name)")
    }
}

// ---- Tile rendering ----

// The glyph is drawn inside a transparency layer so its .clear carving punches
// through the white ink only, letting the tile's gradient show through the
// network — NOT through to the canvas.
func drawGlyphLayer(_ ctx: CGContext, name: String, boxOrigin: CGPoint, boxSize: CGFloat) {
    ctx.saveGState()
    ctx.beginTransparencyLayer(auxiliaryInfo: nil)
    ctx.translateBy(x: boxOrigin.x, y: boxOrigin.y)
    drawGlyph(name, ctx, boxSize)
    ctx.endTransparencyLayer()
    ctx.restoreGState()
}

func makeRep(_ px: Int) -> (NSBitmapImageRep, CGContext)? {
    guard let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
    ) else { return nil }
    guard let gctx = NSGraphicsContext(bitmapImageRep: rep) else { return nil }
    let ctx = gctx.cgContext
    ctx.clear(CGRect(x: 0, y: 0, width: px, height: px))
    return (rep, ctx)
}

// App icon: gradient tile on Apple's icon grid (tile ≈ 82.4% of canvas,
// corner radius ≈ 22.37% of tile), lan-on glyph in white at 62% of the tile.
func renderAppIcon(_ px: Int) -> NSBitmapImageRep? {
    guard let (rep, ctx) = makeRep(px) else { return nil }
    let S = CGFloat(px)
    let inset = S * 0.088
    let tile = CGRect(x: inset, y: inset, width: S - 2 * inset, height: S - 2 * inset)
    let radius = tile.width * 0.2237
    let path = CGPath(roundedRect: tile, cornerWidth: radius, cornerHeight: radius, transform: nil)

    ctx.saveGState()
    ctx.addPath(path)
    ctx.clip()
    let grad = CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(),
                          colors: [gradientTop, gradientBottom] as CFArray,
                          locations: [0, 1])!
    ctx.drawLinearGradient(grad,
                           start: CGPoint(x: S / 2, y: S),
                           end: CGPoint(x: S / 2, y: 0),
                           options: [])
    ctx.restoreGState()

    let g = tile.width * 0.62
    drawGlyphLayer(ctx, name: "on",
                   boxOrigin: CGPoint(x: (S - g) / 2, y: (S - g) / 2), boxSize: g)
    return rep
}

// README state tile: full-bleed flat slate rounded square, glyph at 74%.
func renderStateTile(_ name: String, _ px: Int) -> NSBitmapImageRep? {
    guard let (rep, ctx) = makeRep(px) else { return nil }
    let S = CGFloat(px)
    let tile = CGRect(x: 0, y: 0, width: S, height: S)
    let path = CGPath(roundedRect: tile, cornerWidth: S * 0.25, cornerHeight: S * 0.25, transform: nil)
    ctx.setFillColor(stateTileColor)
    ctx.addPath(path)
    ctx.fillPath()
    let g = S * 0.74
    drawGlyphLayer(ctx, name: name,
                   boxOrigin: CGPoint(x: (S - g) / 2, y: (S - g) / 2), boxSize: g)
    return rep
}

func writePNG(_ rep: NSBitmapImageRep, _ path: String) {
    guard let data = rep.representation(using: .png, properties: [:]) else {
        FileHandle.standardError.write("PNG encode failed: \(path)\n".data(using: .utf8)!)
        exit(1)
    }
    try? data.write(to: URL(fileURLWithPath: path))
    print("wrote \(path)  \(rep.pixelsWide)x\(rep.pixelsHigh)")
}

// The carve must expose the TILE, not the canvas: at the apex node's centre the
// pixel has to be opaque (gradient showing through), or the transparency-layer
// isolation is broken and the icon has see-through holes.
func checkCarve(_ rep: NSBitmapImageRep, tileFrac: CGFloat, glyphFrac: CGFloat, what: String) {
    let S = CGFloat(rep.pixelsWide)
    let g = S * tileFrac * glyphFrac
    let ox = (S - g) / 2, oy = (S - g) / 2
    let x = Int(ox + nodeApex.x * g), y = Int(oy + (1 - nodeApex.y) * g) // colorAt is top-down
    let c = rep.colorAt(x: x, y: y)
    let alpha = c?.alphaComponent ?? -1
    let white = c.map { ($0.redComponent + $0.greenComponent + $0.blueComponent) / 3 } ?? -1
    if alpha < 0.99 || white > 0.9 {
        FileHandle.standardError.write(
            "FAIL \(what): carve punched through the tile (alpha=\(alpha) brightness=\(white))\n"
                .data(using: .utf8)!)
        exit(1)
    }
}

// ---- Outputs ----

let fm = FileManager.default
try? fm.createDirectory(atPath: "assets/readme", withIntermediateDirectories: true)

// App icon .icns via iconutil
let iconsetDir = NSTemporaryDirectory() + "lanfirst-AppIcon.iconset"
try? fm.removeItem(atPath: iconsetDir)
try! fm.createDirectory(atPath: iconsetDir, withIntermediateDirectories: true)

for base in [16, 32, 128, 256, 512] {
    for (suffix, px) in [("", base), ("@2x", base * 2)] {
        guard let rep = renderAppIcon(px) else {
            FileHandle.standardError.write("render failed at \(px)px\n".data(using: .utf8)!)
            exit(1)
        }
        if px >= 256 { checkCarve(rep, tileFrac: 0.824, glyphFrac: 0.62, what: "app icon \(px)px") }
        writePNG(rep, "\(iconsetDir)/icon_\(base)x\(base)\(suffix).png")
    }
}

let iconutil = Process()
iconutil.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
iconutil.arguments = ["-c", "icns", iconsetDir, "-o", "assets/AppIcon.icns"]
try! iconutil.run()
iconutil.waitUntilExit()
guard iconutil.terminationStatus == 0 else {
    FileHandle.standardError.write("iconutil failed\n".data(using: .utf8)!)
    exit(1)
}
print("wrote assets/AppIcon.icns")

// README header logo
if let rep = renderAppIcon(256) {
    checkCarve(rep, tileFrac: 0.824, glyphFrac: 0.62, what: "logo")
    writePNG(rep, "assets/logo.png")
}

// README state tiles
for name in ["on", "public", "off", "error"] {
    guard let rep = renderStateTile(name, 128) else {
        FileHandle.standardError.write("state tile render failed: \(name)\n".data(using: .utf8)!)
        exit(1)
    }
    if name == "on" { checkCarve(rep, tileFrac: 1.0, glyphFrac: 0.74, what: "state tile") }
    writePNG(rep, "assets/readme/state-\(name).png")
}
