// Helpers every effect can use. Part of the effect format: see wiki/Writing-effects.md.
#define PI 3.14159265
#define TAU 6.28318531
#define time iTimes.x
#define flow iTimes.y
#define dt iTimes.z
#define beatCount iTimes.w
#define bass iBands.x
#define mid iBands.y
#define treble iBands.z
#define level iBands.w
#define beat iStyle.w
#define intensity iStyle.x
#define calm iStyle.y
#define seed iStyle.z
#define aspect iView.z
#define resolution iView.xy

WV_SAMPLER(1) iAudio;
#define WV_AUDIO_ROWS 68.0
float wv_row(float x, float row) { return texture(iAudio, vec2(x, (row + 0.5) / WV_AUDIO_ROWS)).r; }
// x runs 0 to 1: low notes to high notes on the spectrum, oldest to newest on the waves.
float spectrum(float x) { return wv_row(x, 0.0); }
float waveL(float x) { return wv_row(x, 1.0) * 2.0 - 1.0; }
float waveR(float x) { return wv_row(x, 2.0) * 2.0 - 1.0; }
float wave(float x) { return 0.5 * (waveL(x) + waveR(x)); }
// The spectrum's slowly falling peaks.
float peak(float x) { return wv_row(x, 3.0); }
// The spectrum as it was: age 0 is now, age 1 about a second ago.
float history(float x, float age) {
  float rows = WV_AUDIO_ROWS - 4.0;
  float r = floor(fract((iView.w - clamp(age, 0.0, 0.999) * (rows - 1.0) + 0.5) / rows) * rows);
  return wv_row(x, 4.0 + r);
}

vec3 palette(float t) { return iPalA.rgb + iPalB.rgb * cos(TAU * (iPalC.rgb * t + iPalD.rgb)); }

mat2 rot(float a) { float c = cos(a), s = sin(a); return mat2(c, -s, s, c); }
// Centred coordinates: y runs -1 to 1, x keeps the screen's shape.
vec2 centred(vec2 uv) { return (uv - 0.5) * vec2(2.0 * aspect, 2.0); }

float hash(float n) { return fract(sin(n * 127.1) * 43758.5453); }
float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
vec2 hash2(vec2 p) { return fract(sin(vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)))) * 43758.5453); }
float noise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), f.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), f.x), f.y);
}
float fbm(vec2 p) {
  float v = 0.0, a = 0.5;
  for (int i = 0; i < 4; i++) { v += a * noise(p); p = p * 2.03 + vec2(17.0, 9.0); a *= 0.5; }
  return v;
}
