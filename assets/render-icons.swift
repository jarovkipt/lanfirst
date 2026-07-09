// render-icons.swift — one-off generator for the menu-bar template icons.
//
// Draws the lanfirst logomark — a geometric house with a three-node network
// carved inside — as monochrome (black-on-TRANSPARENT) PNG files. menuet
// forces NSStatusItem .template = true at runtime, so the menu bar derives the
// template MASK from each PNG's alpha channel and tints it per light/dark mode.
//
// All four states are variants of the same mark so the app stays recognizable
// at a glance (see iconName in cmd/lanfirst/main.go):
//
//   lan-on      solid house, network carved out       (a domain routing to LAN)
//   lan-public  outlined house, nodes disconnected    (all domains on public DNS)
//   lan-off     the solid mark behind a diagonal slash (routing disabled)
//   lan-error   the solid mark with an ! badge         (daemon unreachable)
//
// PNG (not PDF) is deliberate: a CGContext-written PDF has no real alpha channel,
// so AppKit treats its whole page as opaque and the icon renders as a solid box.
// PNG carries unambiguous per-pixel alpha, so only the glyph is inked.
//
// Run once after changing the glyphs:
//     swift assets/render-icons.swift
// Output: assets/icons/<name>.png (22px tall) and <name>@2x.png (44px, Retina).
// Both reps are committed and copied into lanfirst.app/Contents/Resources/ by
// install.sh; [NSImage imageNamed:] loads the @2x companion automatically.

import AppKit

let variants = ["lan-on", "lan-public", "lan-off", "lan-error"]
let outDir = "assets/icons"
try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

// Keep the @1x rep at exactly 22px: menuet's imageWithHeight only downsizes
// when height > 22, so 22 stays untouched (crisp) and the @2x rep handles Retina.
let baseSize = 22

// ---- Geometry, in a unit square (0..1, y-up). Scaled by S at draw time. ----
// House: a symmetric pentagon, wider-stanced and flatter-roofed than Apple's
// house.fill so the silhouette reads as ours even before the carved network.
let houseBaseY: CGFloat = 0.07
let houseEaveY: CGFloat = 0.50
let houseApex = CGPoint(x: 0.50, y: 0.93)
let houseLeftX: CGFloat = 0.10
let houseRightX: CGFloat = 0.90

// Network: two base nodes linked up to an apex node — a node triangle echoing
// the roof pitch. Carved as negative space out of the solid house.
let nodeApex = CGPoint(x: 0.50, y: 0.60)
let nodeLeft = CGPoint(x: 0.30, y: 0.235)
let nodeRight = CGPoint(x: 0.70, y: 0.235)
let nodeR: CGFloat = 0.095
let linkW: CGFloat = 0.075

let black = CGColor(gray: 0, alpha: 1)

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

// Solid house with softly rounded corners (fill + round-join stroke of the
// same path), then the node network punched out as negative space.
func drawSolidHouse(_ ctx: CGContext, _ S: CGFloat) {
    ctx.setFillColor(black)
    ctx.setStrokeColor(black)
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

// Outline house, nodes present but disconnected: "the network isn't reachable
// from home" — everything is resolving over public DNS.
func drawPublic(_ ctx: CGContext, _ S: CGFloat) {
    ctx.setStrokeColor(black)
    ctx.setLineWidth(0.085 * S)
    ctx.setLineJoin(.round)
    ctx.addPath(housePath(S))
    ctx.strokePath()
    ctx.setFillColor(black)
    for n in [nodeApex, nodeLeft, nodeRight] {
        ctx.fillEllipse(in: circleRect(n, 0.085, S))
    }
}

// The solid mark behind a diagonal slash, with a clear gap under the slash so
// it separates from the house (standard macOS "disabled" treatment).
func drawOff(_ ctx: CGContext, _ S: CGFloat) {
    drawOn(ctx, S)
    ctx.setBlendMode(.clear)
    ctx.setLineWidth(0.20 * S)
    ctx.setLineCap(.butt)
    ctx.move(to: pt(CGPoint(x: 0.04, y: 0.96), S))
    ctx.addLine(to: pt(CGPoint(x: 0.96, y: 0.04), S))
    ctx.strokePath()
    ctx.setBlendMode(.normal)
    ctx.setStrokeColor(black)
    ctx.setLineWidth(0.085 * S)
    ctx.setLineCap(.round)
    ctx.move(to: pt(CGPoint(x: 0.10, y: 0.90), S))
    ctx.addLine(to: pt(CGPoint(x: 0.90, y: 0.10), S))
    ctx.strokePath()
}

// The solid mark with an exclamation badge over the bottom-right corner. The
// badge's clear gap ring eats the right node — a broken network, fittingly.
func drawError(_ ctx: CGContext, _ S: CGFloat) {
    drawOn(ctx, S)
    let c = CGPoint(x: 0.80, y: 0.20)
    let rBadge: CGFloat = 0.19
    ctx.setBlendMode(.clear)
    ctx.fillEllipse(in: circleRect(c, 0.25, S))
    ctx.setBlendMode(.normal)
    ctx.setFillColor(black)
    ctx.fillEllipse(in: circleRect(c, rBadge, S))
    ctx.setBlendMode(.clear)
    ctx.fill(CGRect(x: (c.x - 0.03) * S, y: 0.175 * S, width: 0.06 * S, height: 0.14 * S))
    ctx.fillEllipse(in: circleRect(CGPoint(x: c.x, y: 0.10), 0.034, S))
    ctx.setBlendMode(.normal)
}

func draw(_ name: String, _ ctx: CGContext, _ S: CGFloat) {
    switch name {
    case "lan-on": drawOn(ctx, S)
    case "lan-public": drawPublic(ctx, S)
    case "lan-off": drawOff(ctx, S)
    case "lan-error": drawError(ctx, S)
    default: fatalError("unknown variant \(name)")
    }
}

// Render a variant into a transparent ARGB bitmap of the given pixel size.
func renderPNG(_ name: String, pxSize: Int) -> (data: Data, rep: NSBitmapImageRep)? {
    guard let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: pxSize, pixelsHigh: pxSize,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
    ) else { return nil }
    guard let gctx = NSGraphicsContext(bitmapImageRep: rep) else { return nil }
    let ctx = gctx.cgContext
    ctx.clear(CGRect(x: 0, y: 0, width: pxSize, height: pxSize))
    draw(name, ctx, CGFloat(pxSize))
    guard let data = rep.representation(using: .png, properties: [:]) else { return nil }
    return (data, rep)
}

// Fraction of pixels with meaningful ink — guards against both an empty image
// and the opaque-box failure (a template mask that inks the whole square).
func inkFraction(_ rep: NSBitmapImageRep) -> Double {
    var inked = 0
    for y in 0..<rep.pixelsHigh {
        for x in 0..<rep.pixelsWide {
            if (rep.colorAt(x: x, y: y)?.alphaComponent ?? 0) > 0.5 { inked += 1 }
        }
    }
    return Double(inked) / Double(rep.pixelsWide * rep.pixelsHigh)
}

var failures: [String] = []

for name in variants {
    for (suffix, px) in [("", baseSize), ("@2x", baseSize * 2)] {
        guard let (data, rep) = renderPNG(name, pxSize: px) else {
            failures.append("\(name)\(suffix) (render failed)")
            continue
        }
        let cornerA = rep.colorAt(x: 0, y: 0)?.alphaComponent ?? -1
        let ink = inkFraction(rep)
        let path = "\(outDir)/\(name)\(suffix).png"
        try? data.write(to: URL(fileURLWithPath: path))
        print(String(format: "wrote %-26@  %dx%d  corner.alpha=%.2f ink=%.2f",
                     path as NSString, rep.pixelsWide, rep.pixelsHigh, cornerA, ink))
        if cornerA > 0.01 { failures.append("\(name)\(suffix): opaque corner (would box)") }
        if ink < 0.03 { failures.append("\(name)\(suffix): near-empty image") }
        if ink > 0.85 { failures.append("\(name)\(suffix): near-solid image (boxing)") }
    }
}

// Contact sheet on RED so the result is unambiguous: black mark on red = good,
// black rectangle on red = still boxing. (Black-on-black or -white would hide it.)
let sheetH = 44, pad = 12
let cell = sheetH + pad * 2
let sheetW = cell * variants.count
if let sheet = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: sheetW, pixelsHigh: cell,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
    colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0),
    let gctx = NSGraphicsContext(bitmapImageRep: sheet) {
    let ctx = gctx.cgContext
    ctx.setFillColor(CGColor(red: 0.9, green: 0.2, blue: 0.2, alpha: 1))
    ctx.fill(CGRect(x: 0, y: 0, width: sheetW, height: cell))
    for (i, name) in variants.enumerated() {
        ctx.saveGState()
        ctx.translateBy(x: CGFloat(i * cell + pad), y: CGFloat(pad))
        draw(name, ctx, CGFloat(sheetH))
        ctx.restoreGState()
    }
    if let d = sheet.representation(using: .png, properties: [:]) {
        try? d.write(to: URL(fileURLWithPath: "/tmp/lanfirst-icons-contactsheet.png"))
        print("wrote /tmp/lanfirst-icons-contactsheet.png")
    }
}

if !failures.isEmpty {
    FileHandle.standardError.write("FAILURES: \(failures.joined(separator: "; "))\n".data(using: .utf8)!)
    exit(1)
}
