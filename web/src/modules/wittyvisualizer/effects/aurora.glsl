/*wittyvisualizer
{
  "format": 1, "id": "aurora", "name": "Northern Veil",
  "description": "Curtains of light moving over a starry sky, brightening with the mids. The gentlest of the set.",
  "tags": ["calm", "sky"], "cost": 3, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  vec2 p = vec2(uv.x * aspect, uv.y);
  vec3 c = mix(vec3(0.01, 0.02, 0.05), vec3(0.015, 0.05, 0.09), uv.y);
  vec2 star = floor(p * 170.0);
  float twinkle = step(0.994, hash(star)) * (0.5 + 0.5 * sin(time * (1.0 + hash(star + 3.0) * 3.0) + hash(star) * 40.0));
  c += vec3(0.75) * twinkle * smoothstep(0.1, 0.5, uv.y);

  for (int i = 0; i < 9; i++) {
    float n = float(i) / 9.0;
    float height = 0.26 + n * 0.5;
    float x = p.x * 1.15 + n * 1.2;
    float fold = noise(vec2(x * 1.6 + flow * 0.07, n * 3.0 + flow * 0.03));
    float curtain = noise(vec2(x * 3.8 + fold * 2.8 - flow * 0.05, n * 5.0));
    float rise = uv.y - height + 0.13 * (fold - 0.5);
    float shape = exp(-rise * rise * 42.0) * smoothstep(-0.06, 0.03, rise + 0.03);
    float heard = spectrum(fract(x * 0.3) * 0.7);
    vec3 tint = mix(palette(0.32 + n * 0.3 + flow * 0.01), vec3(0.15, 0.95, 0.5), 0.38);
    c += tint * shape * curtain * curtain * (0.18 + 0.75 * mid * intensity + 0.65 * heard * heard);
  }
  float ground = smoothstep(0.1 + 0.03 * noise(vec2(p.x * 7.0, 2.0)), 0.085 + 0.03 * noise(vec2(p.x * 7.0, 2.0)), uv.y);
  c = mix(c, vec3(0.004, 0.007, 0.012), ground);
  return vec4(c, 1.0);
}
