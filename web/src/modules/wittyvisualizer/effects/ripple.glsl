/*wittyvisualizer
{
  "format": 1, "id": "ripple", "name": "Ripple Tank",
  "description": "A pool of water seen from above. Each beat is a drop that sends rings across it, and they bend the light on the bottom.",
  "tags": ["water", "calm"], "cost": 2, "motion": "gentle"
}
*/
float surface(vec2 p) {
  float h = 0.0;
  for (int i = 0; i < 4; i++) {
    float age = iBeats[i];
    if (age > 6.0) continue;
    float id = beatCount - float(i);
    vec2 at = (vec2(hash(id * 3.71 + seed), hash(id * 9.13 + seed)) - 0.5) * vec2(1.6 * aspect, 1.6);
    float d = length(p - at), front = age * 0.55;
    h += sin((d - front) * 13.0) * exp(-abs(d - front) * 2.6) * exp(-age * 0.55);
  }
  return h + 0.12 * sin(p.x * 3.1 + flow * 0.25) * sin(p.y * 2.7 - flow * 0.2);
}

vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  float e = 0.012;
  float h = surface(p);
  float hx = surface(p + vec2(e, 0.0)), hy = surface(p + vec2(0.0, e));
  float hx2 = surface(p - vec2(e, 0.0)), hy2 = surface(p - vec2(0.0, e));
  vec2 slope = vec2(hx - hx2, hy - hy2) / (2.0 * e);
  // Water gathers light under a hollow and spreads it under a crest.
  float gather = (hx + hx2 + hy + hy2 - 4.0 * h) / (e * e) * 0.012;
  vec2 bent = p + slope * 0.035;
  vec3 deep = palette(0.55 + flow * 0.012) * 0.3 + vec3(0.0, 0.05, 0.09);
  vec2 cell = abs(fract(bent * 3.0 + 0.5) - 0.5);
  float tile = smoothstep(0.47, 0.5, max(cell.x, cell.y));
  vec3 c = deep * (1.0 - 0.3 * tile);
  float caustic = fbm(bent * 2.4 + vec2(flow * 0.1, flow * 0.07));
  c += palette(0.5 + flow * 0.012) * pow(caustic, 2.5) * 0.55;
  c *= 1.0 + clamp(gather, -0.6, 1.6);
  // The sky on the surface.
  float glint = pow(max(0.0, dot(normalize(vec3(-slope * 0.16, 1.0)), normalize(vec3(0.3, 0.45, 0.84)))), 40.0);
  c += vec3(1.0, 0.98, 0.92) * glint * (0.25 + 0.55 * level * intensity);
  return vec4(c, 1.0);
}
