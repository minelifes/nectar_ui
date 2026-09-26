package gpu

// One uber-shader draws everything:
//   - mode 0: textured quad. Samples the atlas alpha (glyphs; solid fills use
//     the reserved white texel).
//   - mode 1: analytic rounded rectangle via a signed distance field, which
//     gives anti-aliased edges and corners with no textures or tessellation.
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

struct VertexIn {
    @location(0) pos: vec2<f32>,
    @location(1) uv: vec2<f32>,
    @location(2) color: vec4<f32>,
    @location(3) local: vec2<f32>,
    @location(4) params: vec4<f32>,
};

struct VertexOut {
    @builtin(position) clip: vec4<f32>,
    @location(0) uv: vec2<f32>,
    @location(1) color: vec4<f32>,
    @location(2) local: vec2<f32>,
    @location(3) params: vec4<f32>,
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
    var coverage = texel.a;
    if (in.params.w > 0.5) {
        let half_size = in.params.xy;
        let r = in.params.z;
        let q = abs(in.local) - half_size + vec2<f32>(r, r);
        let d = length(max(q, vec2<f32>(0.0, 0.0))) + min(max(q.x, q.y), 0.0) - r;
        coverage = clamp(0.5 - d, 0.0, 1.0);
    }
    var rgb = in.color.rgb;
    if (u.linear_out > 0.5) {
        rgb = srgb_to_linear(rgb);
    }
    let a = in.color.a * coverage;
    return vec4<f32>(rgb * a, a);
}
`
