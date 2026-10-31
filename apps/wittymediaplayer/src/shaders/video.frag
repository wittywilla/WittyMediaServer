#version 440
// Turns decoded pictures into colour on the graphics card, where it costs almost
// nothing: that is what keeps playback smooth on small machines.
layout(location = 0) in vec2 v_uv;
layout(location = 0) out vec4 fragColor;
layout(std140, binding = 0) uniform buf {
    mat4 mvp;
    mat4 yuv2rgb;
    vec4 params; // x: chroma in one texture, y: HDR picture to map down
};
layout(binding = 1) uniform sampler2D texY;
layout(binding = 2) uniform sampler2D texU;
layout(binding = 3) uniform sampler2D texV;

// HDR10 to an ordinary screen: PQ to light, a gentle roll-off, BT.2020 to sRGB.
vec3 pqToLight(vec3 v)
{
    const float m1 = 0.1593017578125, m2 = 78.84375, c1 = 0.8359375, c2 = 18.8515625, c3 = 18.6875;
    vec3 p = pow(max(v, vec3(0.0)), vec3(1.0 / m2));
    return pow(max(p - c1, vec3(0.0)) / (c2 - c3 * p), vec3(1.0 / m1));
}

vec3 mapDown(vec3 pq)
{
    vec3 light = pqToLight(pq) * 100.0; // 1.0 = SDR white at 100 nits
    const mat3 bt2020to709 = mat3(1.6605, -0.1246, -0.0182, -0.5876, 1.1329, -0.1006, -0.0728, -0.0083, 1.1187);
    vec3 c = max(bt2020to709 * light, vec3(0.0));
    c = c / (1.0 + c / 4.0); // Reinhard, with room for highlights
    return pow(c, vec3(1.0 / 2.2));
}

void main()
{
    float y = texture(texY, v_uv).r;
    vec2 uv = params.x > 0.5 ? texture(texU, v_uv).rg : vec2(texture(texU, v_uv).r, texture(texV, v_uv).r);
    vec3 rgb = (yuv2rgb * vec4(y, uv, 1.0)).rgb;
    if (params.y > 0.5)
        rgb = mapDown(clamp(rgb, 0.0, 1.0));
    fragColor = vec4(rgb, 1.0);
}
