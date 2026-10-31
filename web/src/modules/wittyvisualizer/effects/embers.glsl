/*wittyvisualizer
{
  "format": 1, "id": "embers", "name": "Ember Fountain",
  "description": "Thousands of sparks. A steady fountain rises from below, and every beat throws a fresh burst into the air.",
  "tags": ["particles"], "cost": 1, "motion": "lively", "feedback": true,
  "geometry": { "quads": 1800 }
}
*/
#ifdef WV_QUAD
void quad(int i, inout vec2 c, out vec2 pos, out vec4 color) {
  float f = float(i);
  vec2 p;
  float glow, size, tone;
  if (i < 1000) {
    // The fountain: each spark loops on its own clock.
    float life = 1.5 + hash(f * 0.91) * 1.5;
    float run = time / life + hash(f * 1.37);
    float cycle = floor(run), u = fract(run);
    float s = f + cycle * 13.7;
    float where = (hash(s * 0.31) - 0.5) * 1.2 * aspect;
    float ang = (hash(s * 0.77) - 0.5) * 1.1 + 0.25 * sin(flow * 0.3 + where);
    float speed = 1.1 + 1.5 * hash(s * 1.71);
    float age = u * life;
    p = vec2(where, -1.05) + vec2(sin(ang), cos(ang)) * speed * age - vec2(0.0, 0.62 * age * age);
    p.x += 0.05 * sin(age * 3.0 + f);
    glow = (1.0 - u) * (0.4 + 0.9 * level);
    size = 0.012 + 0.012 * hash(f * 3.3);
    tone = hash(f * 0.17) * 0.35 + u * 0.3;
  } else {
    // The bursts: the last four beats, each from its own place.
    int b = (i - 1000) / 200;
    float age = iBeats[b];
    float id = beatCount - float(b);
    vec2 centre = (vec2(hash(id * 3.17 + seed), hash(id * 7.31 + seed)) - 0.5) * vec2(1.5 * aspect, 1.2);
    float ang = hash(f * 1.13 + id) * TAU;
    float speed = 0.35 + 1.25 * hash(f * 2.11 + id);
    float slow = (1.0 - exp(-age * 2.4)) / 2.4;
    p = centre + vec2(cos(ang), sin(ang)) * speed * slow - vec2(0.0, 0.3 * age * age);
    glow = exp(-age * 2.2) * (0.5 + 0.8 * intensity);
    size = 0.012 + 0.01 * hash(f);
    tone = hash(id * 0.61) + age * 0.15;
  }
  p += c * size;
  pos = vec2(p.x / aspect, p.y);
  color = vec4(palette(tone + flow * 0.02) + vec3(0.25, 0.15, 0.05), glow);
}
#else
vec4 effect(vec2 uv) {
  vec3 old = prev(uv + vec2(0.0, -0.0012)).rgb * (0.74 + 0.08 * calm);
  vec3 c = palette(0.05) * 0.03 * (1.0 - uv.y) * (0.5 + bass);
  return vec4(max(old, c), 1.0);
}
#endif
