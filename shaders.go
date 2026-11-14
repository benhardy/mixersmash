package main

const glowShaderSrc = `//kage:unit pixels
package main

var Center vec2
var InnerRadius float
var OuterRadius float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
    dist := length(dstPos.xy - Center)
    alpha := 1.0 - (dist-InnerRadius)/(OuterRadius-InnerRadius)
    alpha *= clamp(dist-(InnerRadius-1.0), 0.0, 1.0)
    alpha = clamp(alpha, 0.0, 1.0)
    return vec4(alpha, alpha, alpha, alpha) // RGB values premultiplied by alpha
}
`

// gelShaderSrc is a Kage fragment shader that renders an Aqua-style gel body.
// srcPos is in pixel coordinates (0,0)→(Size.x,Size.y) because of //kage:unit pixels.
// Images[0] is the base image (fill + glyph, fully opaque); the shader samples it
// for the base colour and applies the gel lighting over the sampled pixel.
// Output is premultiplied RGBA.
const gelShaderSrc = `//kage:unit pixels
package main

var Size vec2
var Hover float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
    // local converts atlas-relative srcPos to 0..Size coordinates so that
    // geometry (center, radius, p) is independent of base's atlas placement.
    local := srcPos - imageSrc0Origin()
    center := Size * 0.5
    radius := min(center.x, center.y)
    p := (local - center) / radius
    dist := length(p)
    pixelWidth := 1.5 / radius
    alpha := 1.0 - smoothstep(1.0-pixelWidth, 1.0, dist)
    if alpha <= 0.0 { return vec4(0.0) }
    // Sample the base image (fill + glyph). imageSrc0At returns premultiplied
    // RGBA. The base image is fully opaque (A=255 everywhere), so premultiplied
    // rgb == straight rgb and no division is needed.
    baseColor := imageSrc0At(srcPos).rgb
    t := clamp((p.y+1.0)*0.5, 0.0, 1.0)
    topColor := baseColor * 0.35
    bottomColor := mix(baseColor, vec3(1.0), 0.35)
    col := mix(topColor, bottomColor, pow(t, 1.3))
    bottomGlow := smoothstep(0.1, 0.95, p.y) * (1.0 - smoothstep(0.65, 1.0, dist))
    col += mix(baseColor, vec3(1.0), 0.65) * bottomGlow * (0.55 + Hover*0.2)
    glossP := vec2(p.x*1.18, (p.y+0.44)*1.85)
    glossDist := length(glossP)
    glossMask := 1.0 - smoothstep(0.72, 0.78, glossDist)
    glossFade := clamp(1.0-(p.y+0.92)*1.85, 0.0, 1.0)
    col = mix(col, vec3(1.0), glossMask*glossFade*0.85)
    rim := smoothstep(0.90, 0.98, dist) * (1.0 - smoothstep(0.98, 1.0, dist))
    col += mix(baseColor, vec3(1.0), 0.5) * rim * 0.45
    return vec4(col*alpha, alpha)
}
`
