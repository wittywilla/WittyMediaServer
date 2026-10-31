/*wittyvisualizer
{
  "format": 1, "id": "molten", "name": "Molten Glass",
  "description": "Sine plasma with a glassy sheen. Bass swells the folds, and the colours roll through it.",
  "tags": ["plasma", "classic"], "cost": 1, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv) * 1.3;
  float t = flow * 0.42;
  float swell = 0.28 + 0.5 * bass * intensity;
  p += swell * 0.5 * vec2(sin(p.y * 2.1 + t), cos(p.x * 1.9 - t * 0.8));
  float v = sin(p.x * 2.8 + t)
    + sin(p.y * 3.4 - t * 1.3)
    + sin((p.x + p.y) * 2.2 + t * 0.7)
    + sin(length(p * vec2(1.3, 0.9) + vec2(sin(t * 0.3), cos(t * 0.4))) * 5.0 - t * 1.7);
  v = v * 0.25 + 0.5;
  vec3 c = palette(v * 1.1 + flow * 0.03 + beat * 0.04);
  float bands = 0.5 + 0.5 * sin(v * TAU * 3.0 + mid * 2.0);
  c *= 0.42 + 0.5 * bands;
  // The sheen is where the folds turn fastest.
  float sheen = pow(0.5 + 0.5 * sin(v * TAU * 6.0 + t), 14.0);
  c += vec3(1.0, 0.95, 0.85) * sheen * (0.14 + 0.5 * treble);
  vec2 m = uv - 0.5;
  c *= 1.0 - 0.9 * dot(m, m);
  return vec4(c, 1.0);
}
