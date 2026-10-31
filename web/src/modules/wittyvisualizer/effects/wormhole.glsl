/*wittyvisualizer
{
  "format": 1, "id": "wormhole", "name": "Wormhole",
  "description": "Falling down a twisting tunnel whose walls carry the music: what you hear now is at the mouth, and what you just heard is deep inside.",
  "tags": ["tunnel"], "cost": 1, "motion": "lively"
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  p += 0.22 * vec2(sin(flow * 0.37), cos(flow * 0.29));
  float r = length(p) + 0.001;
  float a = atan(p.y, p.x);
  // Rings at the same size everywhere, however far in: the log makes it self-similar.
  float depth = -log(r);
  float z = depth * 2.4 + flow * 1.3;
  // Whole numbers of cells around the wall, so it meets itself where the angle wraps.
  float twist = a * 6.0 + depth * 2.5 * sin(flow * 0.13) + flow * 0.5;
  float cells = sin(twist) * sin(z * PI);
  float pattern = 0.16 + 0.7 * smoothstep(-0.25, 0.75, cells);
  float around = abs(fract(a / TAU + 0.5) * 2.0 - 1.0);
  // What's playing now is at the mouth, and what has just played is further in.
  float age = clamp(1.25 - r, 0.0, 1.0);
  float heard = history(around * 0.8, age);
  float tone = z * 0.045 + 0.08 * cos(a) + flow * 0.01;
  vec3 c = palette(tone) * pattern;
  c = mix(c, palette(tone + 0.4) * 1.1, heard * heard * 0.8);
  // Far away is dark.
  c *= smoothstep(0.02, 0.55, r);
  return vec4(c, 1.0);
}
