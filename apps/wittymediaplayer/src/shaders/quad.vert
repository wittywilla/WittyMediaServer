#version 440
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 uv;
layout(location = 0) out vec2 v_uv;
layout(std140, binding = 0) uniform buf {
    mat4 mvp;
    mat4 yuv2rgb;
    vec4 params;
};
void main()
{
    v_uv = uv;
    gl_Position = mvp * vec4(position, 0.0, 1.0);
}
