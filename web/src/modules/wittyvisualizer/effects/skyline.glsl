/*wittyvisualizer
{
  "format": 1, "id": "skyline", "name": "Skyline",
  "description": "The classic spectrum: stacks of lights rising with each note, with falling caps and a wet reflection.",
  "tags": ["spectrum", "classic"], "cost": 1, "motion": "gentle"
}
*/
vec4 effect(vec2 uv) {
  float bars = 56.0, floorAt = 0.17, segs = 26.0;
  float cx = (floor(uv.x * bars) + 0.5) / bars;
  float across = fract(uv.x * bars);
  float gap = smoothstep(0.07, 0.14, across) * smoothstep(0.93, 0.86, across);
  float h = pow(spectrum(cx), 1.0 + 0.4 * (1.0 - intensity));
  float cap = peak(cx);
  float y = uv.y - floorAt;
  float up = abs(y) / (1.0 - floorAt) * 1.06;
  float s = fract(up * segs);
  float seg = smoothstep(0.08, 0.16, s) * smoothstep(0.92, 0.84, s);
  float on = step(up, h);
  vec3 tint = palette(cx * 0.65 + flow * 0.025);
  vec3 lit = mix(tint, vec3(1.0, 0.95, 0.82), smoothstep(0.55, 1.0, up) * 0.75);
  vec3 c = vec3(0.012, 0.016, 0.03) + tint * 0.05 * (1.0 - uv.y) * (0.5 + level);
  float capMark = smoothstep(0.018, 0.008, abs(up - cap - 0.012)) * step(0.02, cap);
  if (y >= 0.0) {
    c += tint * 0.045 * gap * seg;
    c += lit * on * gap * seg * (0.75 + 0.35 * beat);
    c += vec3(1.0, 0.98, 0.9) * capMark * gap;
    c += lit * 0.06 * on * (1.0 - gap);
  } else {
    float fade = smoothstep(1.0, 0.0, -y / floorAt) * 0.42;
    float ripple = sin(uv.y * 150.0 + time * 3.0) * 0.004;
    float rx = (floor((uv.x + ripple) * bars) + 0.5) / bars;
    float rh = spectrum(rx);
    float deep = -y / (1.0 - floorAt) * 1.06 * 1.6;
    c += palette(rx * 0.65 + flow * 0.025) * step(deep, rh) * fade * gap;
  }
  return vec4(c, 1.0);
}
