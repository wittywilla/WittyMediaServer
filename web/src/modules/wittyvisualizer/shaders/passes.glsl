// Every shader that isn't an effect, in sections. Each host cuts this file at the
// //@ lines (assembly in shaders.ts and qt/CMakeLists.txt), so there is one copy.

//@ effect-head
// What an effect sees before its own code.
#ifdef WV_FEEDBACK
WV_SAMPLER(2) iPrev;
// This effect's own last picture, at the same place on the screen.
vec4 prev(vec2 uv) { return texture(iPrev, vec2(uv.x, iHost.x > 0.5 ? 1.0 - uv.y : uv.y)); }
#else
vec4 prev(vec2 uv) { return vec4(0.0); }
#endif

//@ effect-tail
WV_IN(0) vec2 vUv;
WV_OUT(0) vec4 fragColor;
void main() { fragColor = effect(vUv); }

//@ fullscreen-vert
// A triangle over the whole target, with no vertex data.
WV_VOUT(0) vec2 vUv;
void main() {
  vec2 p = vec2(WV_VERTEX_ID == 1 ? 3.0 : -1.0, WV_VERTEX_ID == 2 ? 3.0 : -1.0);
  vUv = p * 0.5 + 0.5;
  gl_Position = vec4(p.x, p.y * iHost.y, 0.0, 1.0);
}

//@ quad-vert
// Runs an effect's quad() for each corner of each of its quads, with no vertex data.
WV_VOUT(0) vec2 vTex;
WV_VOUT(1) vec4 vColor;
void main() {
  int corner = WV_VERTEX_ID % 6;
  vec2 c = vec2(corner == 1 || corner == 2 || corner == 4 ? 1.0 : -1.0, corner == 2 || corner == 4 || corner == 5 ? 1.0 : -1.0);
  vec2 pos;
  vec4 color;
  vec2 tex = c; // an effect sets tex.x to 0 for a line that doesn't fade along its length
  quad(WV_VERTEX_ID / 6, tex, pos, color);
  vTex = tex;
  vColor = color;
  gl_Position = vec4(pos.x, pos.y * iHost.y, 0.0, 1.0);
}

//@ quad-frag
// A soft-edged spot of light: quads add up, so overlaps glow.
WV_IN(0) vec2 vTex;
WV_IN(1) vec4 vColor;
WV_OUT(0) vec4 fragColor;
void main() {
  float d = max(0.0, 1.0 - dot(vTex, vTex));
  fragColor = vec4(vColor.rgb * vColor.a * d * d, 1.0);
}

//@ composite
// Where one effect becomes the next.
WV_SAMPLER(2) iA;
WV_SAMPLER(3) iB;
WV_SAMPLER(4) iGuard;
vec2 wv_read(vec2 uv) { return vec2(uv.x, iHost.x > 0.5 ? 1.0 - uv.y : uv.y); }
vec3 composite(vec2 uv) {
  vec3 a = texture(iA, wv_read(uv)).rgb;
  float t = iMix.x;
  if (t <= 0.0) return a;
  vec3 b = texture(iB, wv_read(uv)).rgb;
  float k = t;
  if (iMix.y > 2.5) { // a sweep from one side
    k = smoothstep(uv.x, uv.x + 0.2, t * 1.2);
  } else if (iMix.y > 1.5) { // an iris from the middle
    float r = length((uv - 0.5) * vec2(iView.z, 1.0)) / (0.5 * sqrt(iView.z * iView.z + 1.0));
    k = smoothstep(r, r + 0.25, t * 1.25);
  } else if (iMix.y > 0.5) { // a dissolve through soft clouds
    float n = 0.5 + 0.25 * sin(uv.x * 9.0 + uv.y * 4.0) + 0.25 * sin(uv.y * 13.0 - uv.x * 3.0);
    k = smoothstep(n, n + 0.3, t * 1.3);
  }
  return mix(a, b, k);
}

//@ guard-frag
// The flash guard's first half. It looks at the picture in a few tiles of the screen,
// and for each tile remembers how bright that tile has lately been at its darkest. It
// lets a tile rise above that only by a small step, and lets the memory itself climb only
// slowly. So a repeated swing of the picture's brightness, however the music or the
// effect try to make it, can't be both large and quick:
//
//   swing within any 1/3 second <= dead band + rise / 3 < 0.1 of relative luminance
//
// and 0.1 is the size below which WCAG 2.3.1 doesn't count a change as a flash at all, at
// any rate. safety.ts has the same sums as a pure function, and the tests check it.
//
// Output: r, g = the tile's remembered brightness to 16 bits, b = gain for present-frag.
WV_IN(0) vec2 vUv;
WV_OUT(0) vec4 fragColor;

float flashMetric(vec3 srgb) {
  vec3 c = pow(max(srgb, vec3(0.0)), vec3(2.2));
  float lum = dot(c, vec3(0.2126, 0.7152, 0.0722));
  // WCAG treats saturated red as worse: a change of 20/320 in R - G - B is a flash.
  float sum = c.r + c.g + c.b;
  float red = sum > 0.0 && c.r / sum >= 0.8 ? (c.r - c.g - c.b) * 1.6 : 0.0;
  return max(lum, red);
}

void main() {
  vec2 tile = vec2(8.0, 5.0);
  // Fixed places within a tile, so a still picture measures still.
  vec3 sum = vec3(0.0);
  for (int y = 0; y < 3; y++) {
    for (int x = 0; x < 4; x++) {
      vec2 f = vec2(float(x) + (y == 1 ? 0.75 : 0.25), float(y) + 0.5) / vec2(4.0, 3.0);
      sum += composite((floor(vUv * tile) + f) / tile);
    }
  }
  float cur = flashMetric(sum / 12.0);

  vec4 was = texelFetch(iGuard, ivec2(gl_FragCoord.xy), 0);
  float memory = (was.r * 255.0 * 256.0 + was.g * 255.0) / 65535.0;
  memory = min(cur, memory + iHost.w * iTimes.z);
  float allowed = memory + iHost.z;
  // A plain limit: smoothing this would itself make the picture swing.
  float gain = cur > allowed ? allowed / cur : 1.0;
  float m = floor(memory * 65535.0 + 0.5);
  fragColor = vec4(floor(m / 256.0) / 255.0, mod(m, 256.0) / 255.0, gain, 1.0);
}

//@ present-frag
// What reaches the screen: the picture, turned down wherever the flash guard says.
WV_IN(0) vec2 vUv;
WV_OUT(0) vec4 fragColor;
void main() {
  vec3 c = composite(vUv);
  float gain = texture(iGuard, wv_read(vUv)).b;
  // The gain is for light, and these colours are as the screen shows them.
  fragColor = vec4(c * pow(gain, 1.0 / 2.2), 1.0);
}
