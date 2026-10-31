/*wittyvisualizer
{
  "format": 1, "id": "lissajous", "name": "Lantern Knots",
  "description": "Left against right: the stereo image drawn as a looping line of light that knots and opens with the mix.",
  "tags": ["scope", "stereo"], "cost": 1, "motion": "gentle", "feedback": true,
  "geometry": { "quads": 511 }
}
*/
#ifdef WV_QUAD
vec2 knot(float x) {
  // Left against a moment later on the right: loops open up even in a mono recording.
  vec2 s = vec2(waveL(x), waveR(fract(x + 0.07)));
  return rot(flow * 0.07) * (s * (1.4 + 0.8 * intensity));
}
void quad(int i, inout vec2 c, out vec2 pos, out vec4 color) {
  float f = float(i) / 511.0;
  vec2 a = knot(f), b = knot(f + 1.0 / 511.0);
  vec2 along = b - a;
  float len = length(along);
  vec2 dir = len > 0.0001 ? along / len : vec2(1.0, 0.0);
  vec2 side = vec2(-dir.y, dir.x);
  float w = 0.011;
  vec2 p = a + along * (c.x * 0.5 + 0.5) + side * c.y * w + dir * c.x * w * 0.5;
  pos = vec2(p.x / aspect, p.y);
  c.x = 0.0;
  // A real beam spends less time on a long stroke, so it's dimmer there.
  float bright = 0.2 / (1.0 + len * 22.0) + 0.03;
  color = vec4(palette(f * 0.8 + flow * 0.04), bright);
}
#else
vec4 effect(vec2 uv) {
  vec2 q = (uv - 0.5) * (0.992 - 0.01 * bass * intensity) + 0.5;
  vec3 old = prev(q).rgb * (0.88 - 0.03 * calm);
  return vec4(old + vec3(0.002, 0.002, 0.004), 1.0);
}
#endif
