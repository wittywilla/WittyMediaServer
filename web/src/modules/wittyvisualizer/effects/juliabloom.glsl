/*wittyvisualizer
{
  "format": 1, "id": "juliabloom", "name": "Fractal Bloom",
  "description": "A Julia fractal whose shape is steered by the music, opening and folding like a flower on a slow orbit.",
  "tags": ["fractal"], "cost": 3, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  vec2 z = rot(flow * 0.03) * centred(uv) * (1.15 - 0.1 * bass * intensity);
  float angle = 2.72 + 0.32 * sin(flow * 0.11) + 0.03 * (mid - treble) * intensity;
  vec2 k = 0.7885 * vec2(cos(angle), sin(angle));
  float n = 0.0, zz = dot(z, z);
  float trap = 8.0;
  for (int i = 0; i < 28; i++) {
    z = vec2(z.x * z.x - z.y * z.y, 2.0 * z.x * z.y) + k;
    zz = dot(z, z);
    trap = min(trap, abs(z.x * z.y));
    if (zz > 64.0) break;
    n += 1.0;
  }
  if (n >= 28.0) {
    return vec4(palette(trap * 3.0 + flow * 0.02) * (0.2 + 0.5 * smoothstep(0.4, 0.0, trap)), 1.0);
  }
  float s = n + 1.0 - log(log(max(zz, 1.01)) * 0.5) / log(2.0);
  float t = s / 28.0;
  vec3 c = palette(sqrt(t) * 2.6 + flow * 0.025 + beat * 0.03) * smoothstep(0.02, 0.35, t);
  c += vec3(1.0, 0.95, 0.85) * pow(t, 6.0) * 0.6;
  return vec4(c, 1.0);
}
