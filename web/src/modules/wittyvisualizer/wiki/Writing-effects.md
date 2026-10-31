# Writing effects

An effect is one text file: a description in a comment, then a shader. It's valid GLSL, so a code editor colours it.

```glsl
/*wittyvisualizer
{
  "format": 1, "id": "soft-rings", "name": "Soft Rings",
  "description": "Rings that open with the bass.",
  "author": "Your name", "license": "MIT",
  "tags": ["rings"], "cost": 1, "motion": "gentle", "feedback": true
}
*/
vec4 effect(vec2 uv) {
  vec2 p = centred(uv);
  float rings = sin(length(p) * 12.0 - flow * 3.0 + bass * 2.0);
  vec3 c = palette(length(p) * 0.5 + flow * 0.05) * (0.3 + 0.5 * rings);
  return vec4(max(c, prev(uv).rgb * 0.8), 1.0);
}
```

`effect` is called for each pixel. `uv` runs from 0,0 at the bottom left to 1,1 at the top right.

## The description

| Field | Meaning |
|---|---|
| `format` | Always 1 for now |
| `id` | Up to 40 lower-case letters, numbers and dashes, unique among effects |
| `name` | Up to 40 characters, shown in the list |
| `description`, `author`, `license` | Optional. Please say the licence if you share it |
| `tags` | Optional, up to 8 short words |
| `cost` | 1 to 3: how hard it works the graphics card. 3 is left out on weak machines |
| `motion` | `gentle`, or `lively`, which calm mode leaves out. Be honest: people rely on it |
| `feedback` | `true` to read your last picture with `prev(uv)` |

## What you have to work with

| Name | What it is |
|---|---|
| `time`, `flow`, `dt` | Seconds; seconds that run faster with the music, so motion follows it; seconds since the last frame |
| `bass`, `mid`, `treble`, `level` | 0 to 1, each following its own loudness |
| `beat`, `beatCount` | 1 at a beat and falling away; how many so far |
| `iBeats` | Seconds since the last four beats, as a `vec4` |
| `spectrum(x)`, `peak(x)` | 0 to 1 across low to high notes; the same with slowly falling peaks |
| `history(x, age)` | The spectrum as it was: age 0 is now, age 1 about a second ago |
| `wave(x)`, `waveL(x)`, `waveR(x)` | The sound's shape, -1 to 1 |
| `palette(t)` | The colours of the moment, which move by themselves |
| `prev(uv)` | Your last picture (`feedback` only). Its alpha is yours to keep numbers in |
| `centred(uv)`, `rot(a)`, `aspect`, `resolution` | y from -1 to 1 with x in shape; a rotation; the screen's shape and size |
| `hash(x)`, `hash2(p)`, `noise(p)`, `fbm(p)` | Noise |
| `intensity`, `calm`, `seed` | The person's strength setting; 1 in calm mode; a number that differs each time |
| `PI`, `TAU` | As you'd think |

Don't name your own variables after any of these. They're macros.

## What isn't allowed

A file you add is untrusted, so it's held to a small part of GLSL, and it's refused with the reason if it steps outside:

- Only functions and constants at the top level. A function can call only those above it.
- No `#` directives, arrays, structs, `while`, `do`, `switch`, or textures except through the helpers.
- Loops look like `for (int i = 0; i < 16; i++) { ... }` with plain numbers, run at most 64 times, and have a `{ }` body.
- The worst case for one pixel has to be under 40,000 steps, counting loops within loops and calls within calls.
- Up to 16 KB, plain ASCII. Up to 8 effects, kept on that device.

It's then compiled and drawn small in a throwaway graphics context, and refused if it won't compile or is far too slow. While running,
one that upsets the graphics driver or stays far too slow is taken off the list.

Whatever you write goes through the flash guard, so a flash you draw won't reach the screen as a flash.

Built-in effects can also draw geometry (`#ifdef WV_QUAD`, as in `effects/embers.glsl`), up to 8192 quads. That isn't open to
added effects, as a vertex shader is much harder to bound.
