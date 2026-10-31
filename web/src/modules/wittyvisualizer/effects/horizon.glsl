/*wittyvisualizer
{
  "format": 1, "id": "horizon", "name": "Horizon Run",
  "description": "A wireframe landscape made of the last second of sound: what you hear is at your feet, and the past rolls away to a banded sun.",
  "tags": ["spectrum", "3d", "classic"], "cost": 1, "motion": "lively",
  "geometry": { "quads": 2380 }
}
*/
#ifdef WV_QUAD
const float WIDE = 34.0, DEEP = 34.0;
vec3 land(float x, float row) {
  float z = 0.32 + (row / DEEP) * (row / DEEP) * 3.2;
  float h = history(abs(x / WIDE - 0.5) * 2.0 * 0.75, row / DEEP);
  return vec3((x / WIDE - 0.5) * 3.1, -0.62 + h * h * 0.8 * (0.6 + 0.6 * intensity), z);
}
vec2 view(vec3 p) { return vec2(p.x / p.z, (p.y + 0.0) / p.z + 0.26); }
void quad(int i, inout vec2 c, out vec2 pos, out vec4 color) {
  vec3 a, b;
  float rows = WIDE * (DEEP + 1.0);
  float f = float(i);
  if (f < rows) { // lines across
    float row = floor(f / WIDE), x = mod(f, WIDE);
    a = land(x, row); b = land(x + 1.0, row);
  } else { // lines going away
    float g = f - rows;
    float x = mod(g, WIDE + 1.0), row = floor(g / (WIDE + 1.0));
    a = land(x, row); b = land(x, row + 1.0);
  }
  vec2 sa = view(a), sb = view(b);
  vec2 along = sb - sa;
  float len = length(along) + 0.00001;
  vec2 side = vec2(-along.y, along.x) / len;
  float w = 0.0065 / max(a.z, 0.3) + 0.0012;
  vec2 p = sa + along * (c.x * 0.5 + 0.5) + side * c.y * w;
  pos = vec2(p.x / aspect * 1.25, p.y);
  c.x = 0.0;
  float high = (a.y + b.y) * 0.5 + 0.62;
  vec3 tint = mix(palette(0.55 + flow * 0.015), palette(0.05 + flow * 0.015) + 0.3, smoothstep(0.02, 0.5, high));
  color = vec4(tint, 1.15 * smoothstep(3.5, 1.0, a.z));
}
#else
vec4 effect(vec2 uv) {
  float sky = uv.y - 0.63;
  vec3 c = mix(vec3(0.015, 0.02, 0.05), palette(0.62 + flow * 0.015) * 0.42, exp(-abs(sky) * 7.0));
  if (sky < 0.0) c *= 0.3;
  vec2 p = (uv - vec2(0.5, 0.74)) * vec2(aspect, 1.0);
  float r = length(p), size = 0.17 + 0.02 * bass * intensity;
  float bands = step(0.0, sin(p.y * 92.0 + time * 1.2) + (p.y + 0.05) * 18.0);
  vec3 sun = mix(vec3(1.0, 0.86, 0.45), palette(0.02 + flow * 0.015) + 0.1, smoothstep(0.12, -0.15, p.y));
  float disc = smoothstep(size + 0.004, size - 0.004, r) * bands * step(0.0, sky);
  c = mix(c, sun, disc);
  c += sun * 0.09 * exp(-r * 4.5) * step(0.0, sky);
  return vec4(c, 1.0);
}
#endif
