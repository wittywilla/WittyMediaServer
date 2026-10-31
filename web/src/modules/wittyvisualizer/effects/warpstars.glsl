/*wittyvisualizer
{
  "format": 1, "id": "warpstars", "name": "Warp Field",
  "description": "A starfield in three dimensions. The stars stretch into streaks when the bass pushes, and turn slowly around you.",
  "tags": ["starfield", "classic"], "cost": 1, "motion": "lively", "feedback": true,
  "geometry": { "quads": 1100 }
}
*/
#ifdef WV_QUAD
vec2 project(vec2 xy, float z) { return xy / max(z, 0.02); }
void quad(int i, inout vec2 c, out vec2 pos, out vec4 color) {
  float f = float(i);
  vec2 xy = (vec2(hash(f * 1.31), hash(f * 2.77)) - 0.5) * 2.4;
  xy = rot(flow * 0.04) * xy;
  float z = fract(hash(f * 0.73) - flow * 0.14);
  z = max(z, 0.002);
  float stretch = 0.012 + 0.05 * bass * intensity + 0.03 * beat;
  vec2 head = project(xy, z), tail = project(xy, z + stretch);
  vec2 along = head - tail;
  float len = length(along) + 0.0001;
  vec2 dir = along / len;
  vec2 side = vec2(-dir.y, dir.x);
  float w = (0.003 + 0.007 * (1.0 - z)) * (1.0 + 0.5 * treble);
  vec2 p = tail + along * (c.x * 0.5 + 0.5) + side * c.y * w + dir * c.x * w;
  pos = vec2(p.x / aspect * 0.6, p.y * 0.6);
  c.x *= 0.45;
  float near = smoothstep(1.0, 0.72, z);
  color = vec4(mix(vec3(1.0), palette(f * 0.013 + flow * 0.02), 0.8), near * (0.6 + 0.8 * level));
}
#else
vec4 effect(vec2 uv) {
  vec3 old = prev(uv).rgb * (0.62 + 0.1 * calm);
  vec2 p = centred(uv);
  float haze = fbm(rot(flow * 0.02) * p * 1.1 + flow * 0.015);
  vec3 c = palette(0.55 + flow * 0.012) * pow(haze, 3.0) * 0.17;
  return vec4(max(old, c), 1.0);
}
#endif
