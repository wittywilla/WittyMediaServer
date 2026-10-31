/*wittyvisualizer
{ "format": 1, "id": "strobe", "name": "Strobe", "feedback": false }
*/
// The worst thing an effect could do: the whole screen, black to white. Only for tests, to
// show the flash guard holds. This file isn't in effects/ and never ships.
vec4 effect(vec2 uv) {
  return vec4(vec3(step(0.5, fract(time * 8.0))), 1.0);
}
