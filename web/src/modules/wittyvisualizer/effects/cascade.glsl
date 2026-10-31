/*wittyvisualizer
{
  "format": 1, "id": "cascade", "name": "Cascade",
  "description": "A waterfall of sound: a second of the music laid out in light, mirrored from the middle and falling toward you.",
  "tags": ["spectrum", "waterfall"], "cost": 1, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  float away = pow(uv.y, 1.25);
  float wide = mix(1.0, 0.55, uv.y);
  float x = abs((uv.x - 0.5) / wide) * 2.0;
  if (x > 1.0) return vec4(0.0, 0.0, 0.0, 1.0);
  float heat = history(x * 0.92, 1.0 - away);
  heat = pow(heat, 1.5);
  vec3 c = palette(heat * 0.8 + x * 0.1 + flow * 0.02) * smoothstep(0.02, 0.5, heat);
  c = mix(c, vec3(1.0, 0.98, 0.9), smoothstep(0.65, 1.0, heat));
  c *= smoothstep(1.0, 0.6, uv.y) * 0.75 + 0.25;
  c *= smoothstep(1.0, 0.86, x);
  // A line of light marks now.
  float now = smoothstep(0.012, 0.0, abs(uv.y - 0.985));
  c += palette(flow * 0.02) * now * 0.5;
  return vec4(c + vec3(0.006, 0.008, 0.016), 1.0);
}
