#version 440
// Menus and subtitles over the picture, already premultiplied.
layout(location = 0) in vec2 v_uv;
layout(location = 0) out vec4 fragColor;
layout(std140, binding = 0) uniform buf {
    mat4 mvp;
    mat4 yuv2rgb;
    vec4 params;
};
layout(binding = 1) uniform sampler2D tex;
void main()
{
    fragColor = texture(tex, v_uv);
}
