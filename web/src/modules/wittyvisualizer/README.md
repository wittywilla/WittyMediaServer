<h1 align="center">WittyVisualizer</h1>

<p align="center">
  <b>Light shows that follow your music.</b><br>
  Bars, scopes, plasma, tunnels, starfields, feedback and fire, in full screen, on the web and on the desktop.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="WebGL 2 and Qt 6" src="https://img.shields.io/badge/runs%20on-WebGL%202%20·%20Qt%206-8b6cff.svg">
</p>

<p align="center">
  <a href="#the-effects">The effects</a> ·
  <a href="#how-its-built">How it's built</a> ·
  <a href="#safety">Safety</a> ·
  <a href="#use-it-in-an-app">Use it in an app</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Developed in `web/src/modules/wittyvisualizer` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).

Runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer)'s audio player and
[WittyMediaPlayer](../../../../apps/wittymediaplayer), the desktop app, from one set of effect files.

It's in the spirit of the music visualizers people loved on their desktops around the turn of the century,
written from scratch for this project. Nothing here comes from any other program's effects, presets, names or code.

## The effects

| Effect | What it is | Motion |
|---|---|---|
| **Skyline** | The classic spectrum: stacks of lights with falling caps and a wet reflection | gentle |
| **Phosphor Trace** | An oscilloscope: left and right, drawn by a beam on a lab screen with an afterglow | gentle |
| **Lantern Knots** | A vector scope: the sound against itself, as a looping line of light | gentle |
| **Molten Glass** | Sine plasma with a glassy sheen | gentle |
| **Wormhole** | A twisting tunnel whose walls carry the last second of the music | lively |
| **Warp Field** | A 3D starfield that stretches into streaks when the bass pushes | lively |
| **Ink Drift** | Feedback: each picture is the last one pulled in and stirred | gentle |
| **Prism Wheel** | A kaleidoscope that gains and loses sides | lively |
| **Ember Fountain** | Thousands of sparks, with a fresh burst on every beat | lively |
| **Sun Dial** | The spectrum wrapped around a disc | gentle |
| **Lava Lamp** | Soft blobs that merge and split | gentle |
| **Hearth** | A pixel fire fed by the music | gentle |
| **Horizon Run** | A wireframe landscape made of the last second of sound | lively |
| **Cascade** | A waterfall of sound | gentle |
| **Fractal Bloom** | A Julia fractal steered by the music | gentle |
| **Northern Veil** | Curtains of light over a starry sky | gentle |
| **Ripple Tank** | A pool where each beat is a drop | gentle |

Colours move with the beat through eight families. Effects change by a fade, a dissolve, an iris or a sweep.
Calm mode leaves out the lively ones.

## How it's built

**One set of effects, two hosts.** An effect is one text file: a JSON description in a comment at the top, then
GLSL. Both hosts read those same files. That was the point of the design, so here's what was weighed:

| Option | Verdict |
|---|---|
| **GLSL files with a small description, compiled by each host** | **Chosen.** A fragment shader is short, runs on the graphics card, and is the one language both WebGL 2 and Qt's QRhi take in. A cheap machine still runs it, and the file is plain text a person can read and write |
| The picture in JavaScript, with the desktop app showing a web view | Rejected. The desktop app has no browser in it, and adding one for this would cost it more than everything else it does |
| The picture in C++ compiled to WebAssembly | Rejected. Slower than shaders where it matters, and it's far harder for anyone to write an effect |
| Native code on each side, written twice | Rejected. They'd drift apart within a month |

The first lines of a shader differ by host (`#version 300 es` in a browser, `#version 440` for Qt's tools), so those
are added when it's put together: `shaders.ts` at run time for the web, and `qt/CMakeLists.txt` at build time for
Qt, which bakes each effect for Direct3D, Metal, Vulkan and OpenGL. The uniform block, the helpers
(`shaders/lib.glsl`) and every other pass (`shaders/passes.glsl`) are shared.

```
effects/*.glsl, palettes.json    the effects: the one source of truth
shaders/                         what every effect shares, and the guard and present passes
analysis.ts   qt/wvanalysis.cpp  listening: FFT, bands, beats  (a shared test file holds them to the same numbers)
engine.ts     qt/wvengine.cpp    what comes next, colours, frame budget
safety.ts                        the flash guard's sums, and how to count flashes
gl.ts         qt/wvrenderer.cpp  drawing, on WebGL 2 and on QRhi
canvas2d.ts                      a plain canvas, where there's no WebGL 2
tap.ts        qt/wvtap.h         getting at the sound
session.ts, Stage.tsx            the web app's loop and interface
qt/wvwidget.cpp                  the desktop's
format.ts                        reads effect files; refuses unsafe ones someone adds
```

**Listening without touching.** In a browser, the safe route is a copy of the sound (`captureStream`): the player keeps
playing straight to the speakers, and turning the visualizer off leaves no trace. Firefox's version mutes the player and
Safari has none, so there the player goes through an audio context, only once that context is running and only for a
stream on the page's own address. That can't be undone until the page closes, though it costs almost nothing. A stream
from another address hides its sound unless it's asked for differently, which would change how it's fetched, so it
isn't asked: the picture follows a gentle pattern and says so. Stream addresses can carry the sign-in, so they're
compared by origin and never stored or logged.

On the desktop the audio thread copies what it hands the device into a lock-free ring. It never waits, so it can't add
latency, and while the visualizer is off that's one atomic read.

**Cheap by design.** It runs entirely on your device, with no work for the server. Nothing loads until it's opened.
Effects draw at whatever resolution holds the frame rate, down to a quarter of the pixels, and only then drop to 30
frames a second, leaving out the most expensive effect. The loop stops when the tab is hidden, the canvas is off screen or it's
turned off, and the graphics card is told to let go of its memory.

## Safety

Flashing can harm people with photosensitive epilepsy, so:

- **It's off until someone turns it on,** after a notice the first time, and it never starts on its own.
- **Calm mode** is the default when the system asks for reduced motion: slower, softer, gentle effects only.
- **A flash guard sits in the last step of drawing,** where no effect can get around it, including ones people write.
  For each of 40 tiles of the screen it remembers how dark that tile has lately been, allows only a small step above that,
  and lets the memory itself climb slowly. So any swing within a third of a second is under 0.1 of relative
  luminance, which is the size below which WCAG 2.3.1 doesn't count it as a flash at all. Saturated red counts for more, as
  the guideline says. A full-screen black and white strobe comes out at a slow, dim glow: see [the tests](tests/safety.test.mjs).
- **Beats can't come faster** than about four a second, or two in calm mode.

No limiter makes moving light safe for everyone, and the notice says so. It hasn't been assessed by a clinician or with a
professional flash analyser.

The canvas is hidden from screen readers, every control is named, and everything works from a keyboard, a remote or a
game controller.

## Effects you can write

A person can add an effect as a data file on their own device: see [Writing effects](wiki/Writing-effects.md). It's treated
as untrusted: only a small part of GLSL is allowed (no directives, no arrays, and only short loops that can be counted up front),
the worst-case cost has to be under a fixed limit, and it's compiled and tried in a throwaway graphics context before it's used. It
still goes through the flash guard. For now, only the web app can add them: on the desktop, shaders are baked at build time.

## Use it in an app

```tsx
import { VisualizerStage, type VisualizerHost } from '@wittywilla/wittyvisualizer'

const host: VisualizerHost = { loadSettings: async () => saved(), saveSettings: (s) => save(s) }

<VisualizerStage host={host} media={() => audioElement} place="panel" onPlace={setPlace} onClose={turnOff} />
```

`place` is `panel`, `full` (the whole screen) or `mini` (a thumbnail with no controls). There's one visualizer per page, and it
moves between places without restarting. Load it with `import()`: its front door pulls in everything. In the server's
web app, `VisualizerView` in `web/src/audio/visualizer.tsx` does all of this and needs only a media element.

For Qt, add `qt/` with `add_subdirectory`, link to `wittyvisualizer`, write the samples you send to the device to a
`wv::SampleTap`, and show a `wv::VisualizerWidget`. See [Embedding](wiki/Embedding.md).

## Tests

```bash
npm test                                  # analysis, flash guard, file checks, effect order
node tests/gallery-run.mjs sheet out.png  # a picture of every effect (with a Vite dev server)
node tests/gallery-run.mjs flash          # a strobe through the real guard
node tests/player-check.mjs out           # through a real player, on a test server
```

`wvtests`, built with `-DWITTYVISUALIZER_TESTS=ON`, does the same for Qt, with no window.

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyVisualizer, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

<!-- module-kit -->
## Releasing

A version tag builds the bundle for the browser, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.0 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->

## Legal

WittyVisualizer is an independent open-source project. It isn't associated with, sponsored by or
endorsed by any maker of other media players or visualizers. This can show flashing lights and
fast-changing patterns, which may affect people with photosensitive conditions. Any third-party names
here only describe what it works with, and belong to their owners. It's provided “as is”, with no
warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't legal advice.
