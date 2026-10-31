/*wittyvisualizer
{
  "format": 1, "id": "lavalamp", "name": "Lava Lamp",
  "description": "Soft blobs that swell, merge and split as they drift, lit from below. Slow and easy on the eye.",
  "tags": ["metaballs", "calm"], "cost": 2, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  float field = 0.0;
  vec2 pull = vec2(0.0);
  for (int i = 0; i < 7; i++) {
    float n = float(i);
    vec2 at = vec2(sin(flow * (0.17 + n * 0.031) + n * 2.1) * (0.35 + 0.09 * n) * aspect, sin(flow * (0.13 + n * 0.023) + n * 4.3) * 0.82);
    float size = 0.17 + 0.03 * n + 0.1 * (i < 3 ? bass : i < 5 ? mid : treble) * intensity;
    vec2 d = p - at;
    float w = size * size / (dot(d, d) + 0.002);
    field += w;
    pull -= d * w * w;
  }
  vec3 back = palette(0.55 + flow * 0.01) * (0.05 + 0.18 * smoothstep(1.0, -1.0, p.y));
  float body = smoothstep(0.95, 1.1, field);
  vec2 n = pull / (length(pull) + 0.001);
  float light = 0.55 + 0.45 * dot(n, normalize(vec2(0.1, -1.0))) * smoothstep(5.0, 1.5, field);
  vec3 wax = palette(min(field, 2.6) * 0.14 + p.y * 0.12 + flow * 0.015);
  vec3 c = mix(back, wax * (0.4 + 0.75 * light), body);
  float rim = smoothstep(0.75, 0.97, field) * (1.0 - body);
  c += wax * rim * 0.75;
  float highlight = pow(max(0.0, 1.0 - abs(field - 1.5) * 1.3), 8.0) * max(0.0, dot(n, normalize(vec2(-0.3, -0.9))));
  c += vec3(1.0, 0.95, 0.85) * highlight * 0.28;
  return vec4(c, 1.0);
}
