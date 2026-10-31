/*wittyvisualizer
{
  "format": 1, "id": "phosphor", "name": "Phosphor Trace",
  "description": "An oscilloscope: the sound's own shape drawn by a glowing beam, left and right, on a lab screen that holds its afterglow.",
  "tags": ["scope", "classic"], "cost": 1, "motion": "gentle", "feedback": true
}
*/
float beam(vec2 uv, float centre, float scale, float side, float px) {
  float e = 0.004;
  float a = side < 0.5 ? waveL(uv.x) : waveR(uv.x);
  float b = side < 0.5 ? waveL(uv.x + e) : waveR(uv.x + e);
  float y = centre + a * scale;
  float slope = (b - a) * scale / e / aspect;
  float d = abs(uv.y - y) / sqrt(1.0 + slope * slope);
  float w = 2.2 * px;
  return exp(-d * d / (w * w)) + 0.22 * exp(-d / (9.0 * px));
}

vec4 effect(vec2 uv) {
  float px = 1.0 / resolution.y;
  vec3 old = prev(uv).rgb * (0.80 + 0.06 * calm);
  vec2 g = abs(fract(uv * vec2(10.0, 8.0) + 0.5) - 0.5) / vec2(10.0, 8.0);
  float grid = smoothstep(1.5 * px, 0.5 * px, min(g.x / aspect, g.y));
  vec2 fromMiddle = (uv - 0.5) * 2.0;
  float tube = 1.0 - 0.45 * dot(fromMiddle, fromMiddle);
  vec3 tint = mix(vec3(0.25, 1.0, 0.45), palette(0.2 + flow * 0.02), 0.45);
  vec3 tintR = mix(vec3(0.3, 0.75, 1.0), palette(0.6 + flow * 0.02), 0.45);
  float scale = 0.2 * (0.7 + 0.5 * intensity);
  vec3 c = tint * beam(uv, 0.70, scale, 0.0, px) + tintR * beam(uv, 0.30, scale, 1.0, px);
  c += vec3(0.05, 0.12, 0.08) * grid;
  return vec4(max(old, c * tube) + vec3(0.005, 0.012, 0.009), 1.0);
}
