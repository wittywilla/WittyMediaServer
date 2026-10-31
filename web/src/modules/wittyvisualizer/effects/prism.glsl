/*wittyvisualizer
{
  "format": 1, "id": "prism", "name": "Prism Wheel",
  "description": "A kaleidoscope: shards of colour folded into a wheel that gains and loses sides as the song moves on.",
  "tags": ["kaleidoscope"], "cost": 2, "motion": "lively", "feedback": true
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  float r = length(p);
  float sides = 6.0 + 2.0 * mod(floor(beatCount / 16.0), 4.0);
  float slice = TAU / sides;
  float a = mod(atan(p.y, p.x) + flow * 0.09, slice);
  a = abs(a - slice * 0.5);
  vec2 k = vec2(cos(a), sin(a)) * r;
  vec2 m = rot(flow * 0.11) * k;

  float n = fbm(m * 1.9 + vec2(flow * 0.17, -flow * 0.05));
  float facets = abs(fract(n * 4.5 + 0.08 * bass * intensity) - 0.5) * 2.0;
  vec3 c = palette(n * 1.4 + r * 0.25 + flow * 0.035) * smoothstep(0.15, 0.85, facets);
  c *= 0.38 + 0.7 * mid * intensity;

  // A line of the spectrum runs along the fold of each slice.
  float along = fract(r * 0.75 - flow * 0.05);
  float heard = spectrum(along);
  c += palette(r * 0.5 + flow * 0.05 + 0.3) * heard * heard * smoothstep(0.1, 0.0, k.y) * 1.2;

  vec2 q = k * vec2(0.5 / aspect, 0.5) * (0.97 - 0.02 * bass * intensity) + 0.5;
  vec3 old = prev(q).rgb * 0.84;
  c = max(c, old);
  return vec4(c * smoothstep(1.7, 0.5, r), 1.0);
}
