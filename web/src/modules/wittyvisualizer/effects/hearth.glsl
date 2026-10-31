/*wittyvisualizer
{
  "format": 1, "id": "hearth", "name": "Hearth",
  "description": "A pixel fire in the old demo style, fed from below by the music: bass burns in the middle, and the highs at the edges.",
  "tags": ["fire", "feedback", "classic"], "cost": 1, "motion": "gentle", "feedback": true
}
*/
vec4 effect(vec2 uv) {
  float rise = dt * 0.62;
  vec2 cell = 1.0 / resolution;
  float sway = (noise(vec2(uv.x * 12.0, uv.y * 7.0 - time * 1.6)) - 0.5) * 0.008;
  vec2 q = vec2(uv.x + sway, uv.y - rise);
  float heat = prev(q).a * 0.5
    + prev(q + vec2(cell.x * 2.0, 0.0)).a * 0.17
    + prev(q - vec2(cell.x * 2.0, 0.0)).a * 0.17
    + prev(q - vec2(0.0, cell.y * 2.0)).a * 0.16;
  float cool = (0.5 + 1.3 * noise(vec2(uv.x * 28.0, uv.y * 18.0 - time * 3.0))) * dt * 0.86;
  heat = max(0.0, heat - cool * (0.6 + uv.y * 1.3));
  // The fuel: what's playing, with the lows in the middle.
  float fromMiddle = abs(uv.x - 0.5) * 2.0;
  float fuel = spectrum(fromMiddle * 0.85) * (0.85 + 0.35 * noise(vec2(uv.x * 40.0, time * 5.0))) * (0.55 + 0.75 * intensity);
  fuel = max(fuel, 0.12 * noise(vec2(uv.x * 14.0, time)));
  heat = mix(heat, max(heat, fuel), smoothstep(0.03, 0.0, uv.y));
  // Black, through a colour of the moment, to orange, yellow and white.
  vec3 tint = mix(vec3(0.9, 0.16, 0.03), palette(0.05 + flow * 0.01), 0.3);
  vec3 c = tint * smoothstep(0.03, 0.42, heat);
  c = mix(c, vec3(1.0, 0.56, 0.08), smoothstep(0.32, 0.62, heat));
  c = mix(c, vec3(1.0, 0.92, 0.5), smoothstep(0.58, 0.85, heat));
  c = mix(c, vec3(1.0), smoothstep(0.85, 1.0, heat));
  return vec4(c, heat);
}
