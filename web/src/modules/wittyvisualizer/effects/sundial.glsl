/*wittyvisualizer
{
  "format": 1, "id": "sundial", "name": "Sun Dial",
  "description": "The spectrum wrapped around a disc of light: every note a ray, mirrored into a slowly turning flower.",
  "tags": ["spectrum", "radial"], "cost": 1, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  p = rot(flow * 0.045) * p;
  float r = length(p);
  float a = atan(p.y, p.x) / TAU + 0.5;
  // Mirrored, with the low notes in four petals and the highs between them.
  float folded = abs(fract(a * 2.0) * 2.0 - 1.0);
  float rays = 64.0;
  float cell = (floor(folded * rays) + 0.5) / rays;
  float across = fract(folded * rays);
  float h = spectrum(pow(cell, 1.4));
  float inner = 0.26 + 0.045 * bass * intensity + 0.025 * beat;
  float outer = inner + 0.06 + h * 0.72;
  float slot = smoothstep(0.1, 0.22, across) * smoothstep(0.9, 0.78, across);
  vec3 tint = palette(cell * 0.7 + flow * 0.03);
  float along = clamp((r - inner) / max(outer - inner, 0.001), 0.0, 1.0);
  vec3 c = tint * slot * step(inner, r) * step(r, outer) * (0.45 + 0.75 * along);
  float cap = smoothstep(0.012, 0.004, abs(r - (inner + 0.06 + peak(pow(cell, 1.4)) * 0.72)));
  c += vec3(1.0, 0.96, 0.88) * cap * slot * step(0.02, peak(pow(cell, 1.4)));

  // The disc itself, with the sound's shape as its rim.
  float rim = inner - 0.03 + 0.02 * wave(folded) * intensity;
  float disc = smoothstep(rim + 0.006, rim - 0.006, r);
  c = mix(c, palette(0.08 + flow * 0.02) * (0.18 + 0.55 * smoothstep(rim, 0.0, r) * (0.5 + 0.6 * level)), disc);
  c += palette(0.1 + flow * 0.02) * 0.09 * exp(-max(r - rim, 0.0) * 3.2) * (0.5 + level);
  return vec4(c, 1.0);
}
