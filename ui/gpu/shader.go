package gpu

// One uber-shader draws everything:
//   - mode 0: textured quad. Samples the atlas alpha (glyphs; solid fills use
//     the reserved white texel).
//   - mode 1: analytic rounded rectangle via a signed distance field, which
//     gives anti-aliased edges and corners with no textures or tessellation.
//   - mode 6: image. Samples the group-1 texture (premultiplied) clipped to
//     the rounded rect.
//
// Output is premultiplied alpha (blend: One, OneMinusSrcAlpha).
const shaderWGSL = `
struct Uniforms {
    viewport: vec2<f32>,
    linear_out: f32,
    _pad: f32,
};

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var atlas_tex: texture_2d<f32>;
@group(0) @binding(2) var atlas_smp: sampler;
@group(1) @binding(0) var image_tex: texture_2d<f32>;
@group(1) @binding(1) var image_smp: sampler;

struct VertexIn {
    @location(0) pos: vec2<f32>,
    @location(1) uv: vec2<f32>,
    @location(2) color: vec4<f32>,
    @location(3) local: vec2<f32>,
    @location(4) params: vec4<f32>,
    @location(5) extra: vec4<f32>,
};

struct VertexOut {
    @builtin(position) clip: vec4<f32>,
    @location(0) uv: vec2<f32>,
    @location(1) color: vec4<f32>,
    @location(2) local: vec2<f32>,
    @location(3) params: vec4<f32>,
    @location(4) extra: vec4<f32>,
};

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
    var out: VertexOut;
    let ndc = in.pos / u.viewport * 2.0 - vec2<f32>(1.0, 1.0);
    out.clip = vec4<f32>(ndc.x, -ndc.y, 0.0, 1.0);
    out.uv = in.uv;
    out.color = in.color;
    out.local = in.local;
    out.params = in.params;
    out.extra = in.extra;
    return out;
}

fn srgb_to_linear(c: vec3<f32>) -> vec3<f32> {
    let lo = c / 12.92;
    let hi = pow((c + vec3<f32>(0.055, 0.055, 0.055)) / 1.055, vec3<f32>(2.4, 2.4, 2.4));
    // mix+step instead of a vector select(): portable to every naga backend.
    return mix(hi, lo, step(c, vec3<f32>(0.04045, 0.04045, 0.04045)));
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
    // Sample unconditionally (explicit LOD keeps it valid in any control flow).
    let texel = textureSampleLevel(atlas_tex, atlas_smp, in.uv, 0.0);
    let img = textureSampleLevel(image_tex, image_smp, in.uv, 0.0);
    var coverage = texel.a;
    let mode = in.params.w;
    if (mode > 0.5) {
        // Signed distance to the rounded rect (negative inside).
        let half_size = in.params.xy;
        let r = in.params.z;
        let q = abs(in.local) - half_size + vec2<f32>(r, r);
        let d = length(max(q, vec2<f32>(0.0, 0.0))) + min(max(q.x, q.y), 0.0) - r;
        if (mode < 1.5) {
            // fill
            coverage = clamp(0.5 - d, 0.0, 1.0);
        } else if (mode < 2.5) {
            // soft shadow
            let b = max(in.extra.x, 0.5);
            coverage = 1.0 - smoothstep(-b, b, d);
        } else if (mode < 3.5) {
            // stroke of width extra.y, inside the edge
            let w = in.extra.y;
            let dr = abs(d + w * 0.5) - w * 0.5;
            coverage = clamp(0.5 - dr, 0.0, 1.0);
        } else if (mode > 5.5) {
            // image: clip to the rounded rect; color comes from the texture
            coverage = clamp(0.5 - d, 0.0, 1.0);
        } else if (mode > 4.5) {
            // ripple: circle (center extra.xy, radius extra.z) inside the rounded rect
            let inShape = clamp(0.5 - d, 0.0, 1.0);
            let dc = length(in.local - in.extra.xy) - in.extra.z;
            coverage = inShape * clamp(0.5 - dc, 0.0, 1.0);
        } else {
            // arc: ring of width extra.y from angle extra.z sweeping extra.w
            let w = in.extra.y;
            let rad = half_size.x - w * 0.5;
            let ring = abs(length(in.local) - rad) - w * 0.5;
            var cov = clamp(0.5 - ring, 0.0, 1.0);
            let tau = 6.2831853;
            var a = atan2(in.local.y, in.local.x) - in.extra.z;
            a = a - floor(a / tau) * tau;
            let sweep = in.extra.w;
            // Signed angular distance to the nearest arc end (positive inside),
            // converted to pixels for anti-aliased ends.
            let inside = min(a, sweep - a);
            let outside = -min(a - sweep, tau - a);
            let sd = select(outside, inside, a <= sweep);
            cov = cov * clamp(0.5 + sd * rad, 0.0, 1.0);
            coverage = cov;
        }
    }
    var rgb = in.color.rgb;
    var alpha = in.color.a;
    if (mode > 5.5) {
        // Filtered premultiplied texel -> straight color (exact filtering).
        rgb = img.rgb / max(img.a, 0.0001);
        alpha = alpha * img.a;
    }
    if (u.linear_out > 0.5) {
        rgb = srgb_to_linear(rgb);
    }
    let a = alpha * coverage;
    return vec4<f32>(rgb * a, a);
}
`
