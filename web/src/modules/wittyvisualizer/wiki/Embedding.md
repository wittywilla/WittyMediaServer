# Embedding

## In WittyMediaServer's web app

```tsx
import { VisualizerView } from '../../audio/visualizer'   // from web/src/...

const [viz, setViz] = useState<'off' | 'panel' | 'full'>('off')

{viz !== 'off' && (
  <VisualizerView media={() => audioRef.current} place={viz} onPlace={(p) => setViz(p === 'full' ? 'full' : 'panel')} onClose={() => setViz('off')} />
)}
```

That's all a player needs, WittyCD's included. Put a parent of the size you want around it for `panel`. `V` and the button are the
player's own: start it only from a press. Settings, the notice, the keys and full screen are taken care of, and it stays out of the
main script until the first time someone opens it. Only one is on the page at a time: a second place takes over the canvas.

`media` may return a different element each time it opens. If that element plays from another address, it follows a pattern, and
the music is left alone.

## In another web app

Import `VisualizerStage` with `import()` so it costs nothing until opened, and give it a `VisualizerHost` with `loadSettings` and
`saveSettings`. It needs React 18 or later and `react-dom`. It takes its colours from CSS variables (`--bg-2`, `--fg`, `--accent` and so
on), with fallbacks for a page that has none.

## In Qt

```cmake
add_subdirectory(path/to/wittyvisualizer/qt wittyvisualizer)
target_link_libraries(yourapp PRIVATE wittyvisualizer)
```

```cpp
// The audio thread, where you hand samples to the device (16-bit stereo). Never blocks.
tap.write(samples, frameCount);

// The window. It starts and stops with being shown.
tap.setRate(rate);
tap.setDelayFrames(framesTheDeviceHolds);
auto *viz = new wv::VisualizerWidget(&tap, &qsettings, parent);
if (viz->confirmFirstUse())   // shows the notice the first time
    viz->show();
```

Pass keys to `viz->handleKey(key)`, and connect `closeRequested` and `fullscreenRequested`. It needs Qt 6.7 or later with
GuiPrivate, for QRhi, and ShaderTools at build time. `apps/wittymediaplayer` is a complete example: `src/ui/mainwindow.cpp`, `setVisualizer`.

## On other kinds of app

The native client apps (Android, Apple and others) don't have it yet. Any of them can use the same files:

- **Android:** GLES 3 takes the shaders nearly as they are, with the web version's first lines. The sound comes from
  ExoPlayer through an `AudioProcessor`. `analysis.ts` needs porting, and `tests/fixtures/analysis-golden.json` checks the port.
- **Apple:** Metal doesn't read GLSL, so the shaders go through Qt's `qsb` or SPIRV-Cross at build time. The sound comes from
  `AVAudioEngine` tap or `MTAudioProcessingTap`.
- **TVs:** TV browsers with WebGL 2 can run the web version. Older ones get the plain canvas, or nothing.
