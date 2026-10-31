/*wittyvisualizer
{
  "format": 1, "id": "inkdrift", "name": "Ink Drift",
  "description": "Each picture is made from the last, pulled inward and stirred, so a ring of sound leaves trails of ink that curl away.",
  "tags": ["feedback", "warp", "classic"], "cost": 2, "motion": "gentle", "feedback": true
}
*/
vec4 effect(vec2 uv) {
  vec2 p = (uv - 0.5) * vec2(aspect, 1.0);
  float push = bass * intensity;
  vec2 stir = vec2(noise(p * 3.0 + vec2(flow * 0.14, 3.0)), noise(p * 3.0 + vec2(9.0, -flow * 0.12))) - 0.5;
  vec2 q = p * (1.012 + 0.012 * push);
  q = rot(0.012 + 0.016 * mid * intensity) * q + stir * (0.006 + 0.012 * level);
  vec3 old = prev(q / vec2(aspect, 1.0) + 0.5).rgb;
  old = old * 0.985 - 0.001;

  float at = atan(p.y, p.x) / TAU + 0.5;
  float shape = sin(at * PI);
  float ring = 0.34 + 0.04 * beat + 0.12 * wave(at) * shape * intensity;
  float d = abs(length(p) - ring);
  vec3 ink = palette(at * 0.5 + flow * 0.05) * smoothstep(0.012, 0.003, d) * (0.6 + 0.9 * level);
  // A wider, slower ring at the edge, led by the highs, feeds the outer part of the swirl.
  float outerAt = 0.72 + 0.04 * sin(at * TAU * 3.0 + flow) + 0.1 * waveR(at) * shape * intensity;
  ink = max(ink, palette(at * 0.5 + flow * 0.05 + 0.33) * smoothstep(0.01, 0.002, abs(length(p) - outerAt)) * (0.25 + 1.1 * treble));
  return vec4(max(old, ink), 1.0);
}
