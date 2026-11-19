<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (C) 2026 WittyWilla and contributors -->
# Porting the 0.8.1 player work to the apps: iPhone, iPad, Apple TV and Android TV

Written 2026-11-18 on the Windows PC, for Claude on the owner's MacBook after 0.8.1-alpha is out. The web client
(this repository, `web/src`) is the reference: everything below was built and checked there first. This file says
**what** each piece does, to the number, **why**, **what the apps must read and write**, and **how** to do it the way
each platform expects. Work through it in order, tick the boxes, commit the ticks, and say in each PR what was run
on a real device or simulator and what wasn't.

Where the apps live (WittyMediaClient): Apple in `apple/App` (SwiftUI screens) and `apple/WittyKit` (shared rules,
already holding `MusicLook.swift` and `MediaCanvas.swift`); Android in `android/app/src/main/kotlin/.../player`
(`PlayerScreen.kt`, `CanvasRules.kt`, `KaraokeStems.kt` and friends). Keep the same split: **rules** (what to show,
when) in the shared, unit-tested layer, **drawing** in the platform's own UI layer.

The server work this ports: PR wittywilla/WittyMediaServer#462 (video player, issue #457),
wittywilla/WittyMediaServer#464 (music and karaoke on a phone, #463), and the Look and Canvas studio PR (#465). The
audit that found it all is under #438.

---

## 0. Three rules that decide everything else

1. **The account is the source of truth for looks; the server for media.** A look chosen on a computer shows on the
   phone and the TV. Canvas videos, their loop points and their crop are on the server. An app never keeps its own
   copy of a choice the server or the account holds; it reads it each time the screen opens (and after an edit).
2. **Advanced settings are walled off on phones, tablets and TVs.** The owner's rule: when something is complicated,
   don't squeeze it in. Show a message saying to use a computer (web browser or desktop app). **What was set on a
   computer still applies on these devices.** See section 6 for the list and the exact words.
3. **Each platform's own idiom wins over looking like the web.** tvOS uses its transport bar, focus engine and menus;
   iOS uses sheets with detents; Android TV uses its focus and D-pad conventions. Match the web's *behaviour and
   choices*, not its pixels, where the platform has a convention.

---

## 1. Data the apps must read and write

### 1.1 The music look: `/me/ui` → `musicPlayer.look` and `musicPlayer.songLooks`

Saved by the web's full player (`web/src/audio/look.ts`) inside the account's UI settings:

```jsonc
// GET /api/v1/me/ui
{
  "musicPlayer": {
    "viz": false,            // the visualizer is behind the player
    "canvas": true,          // Canvas video chosen (when not viz)
    "look": {                // every song's look; missing fields = defaults below
      "art": "square",       // square | circle | vinyl | cd | polaroid | tilt | drift
      "backdrop": "glow",    // glow | blur | cover | plain
      "canvasPlace": "behind", // behind | art | both
      "reflection": true,
      "pulse": false
    },
    "songLooks": {           // a song's own look: only what was changed for it
      "1445": { "art": "circle", "canvasPlace": "art", "show": "canvas" }
    }
    // ...the player's other keys (style, slow, karArt, karHide, …): keep them as they are
  }
}
```

- **Precedence:** `songLooks[id]` over `look` over the defaults `{square, glow, behind, true, false}`.
  `show: "art" | "canvas"` in a song's look turns Canvas on or off for that song alone (it beats `canvas`).
- **Unknown values are ignored**, field by field (a future style must not break an older app): fall back to the
  default for that field. Ids are decimal strings. At most 400 songs keep a look; the newest win.
- **Writing:** the web reads the whole `/me/ui`, changes `musicPlayer`, and PUTs it all back, one write at a time.
  Do the same, and never drop keys you don't know. If the phone app only *reads* looks at first, that's fine:
  the Look panel is not an advanced setting, but it can come in a second step.
- **Mode** (what's behind the player): `viz ? "viz" : (canvasWanted && song has a canvas) ? "canvas" : "art"`,
  where `canvasWanted = songLook.show ? show == "canvas" : musicPlayer.canvas`.
  Canvas **behind** when mode is canvas and `canvasPlace != "art"`; **in the cover's place** when mode is canvas and
  `canvasPlace != "behind"`.

### 1.2 The video player's bar: per device, not per account

`controls: "auto" | "detailed" | "simple"` (web: `web/src/player/viewPrefs.ts`, Viewing settings, and Settings →
Appearance → Player). It is a **device** choice (a phone and a TV want different things), kept on the device.
Apps: offer the same three words where the platform has a choice of layout at all (iPad, Android tablets); on a
phone use Simple; on tvOS and Android TV use the platform transport UI (section 2.6).

### 1.3 Canvas: `GET /api/v1/media/{id}/canvas`

New fields in 0.8.1: `canCut` (an administrator may cut one: only matters for showing the wall message, section 6),
`loopStartMs` and `loopEndMs` (present only when a loop was chosen). **Apps must honour the loop**:

- iOS/tvOS: `AVQueuePlayer` + `AVPlayerLooper(player:templateItem:timeRange:)` with
  `CMTimeRange(start: loopStart, end: loopEnd)`. AVPlayerLooper loops a time range natively and seamlessly.
  Without a loop, `timeRange: .invalid` loops the whole file. Mute it (`isMuted = true`), never show controls.
- Android: Media3 `ClippingMediaSource(source, startUs, endUs)` + `REPEAT_MODE_ONE`, `volume = 0f`.
- The rules for *whether* to play it are already in `CanvasRules` (WittyKit `MediaCanvas.swift`, Android
  `CanvasRules.kt`): Reduce Motion, Low Power, Low Data / metered, away-limited, switched off. Keep them; a canvas in
  the cover's place obeys the same rules and shows the still cover when it may not play.
- The server now crops letterboxing when it cuts, so a canvas fills a square or a circle; still use
  aspect-fill (`.resizeAspectFill`, `RESIZE_MODE_ZOOM`) and clip to the shape.

### 1.4 Scene pictures for files: `GET /api/v1/items/{id}/scene/{sec}`

A 320-wide JPEG of a video file at a whole second (stream-token route: `?st=` works for image loaders). For each
chapter `i` from `start` to `next` (or the end), ask at `floor(start + clamp((next - start) / 4, 2, 20))`: a quarter
into the scene, past fades and studio logos. The server makes at most two at once and caches them; load them only
for what's on screen (lazy grids), and let a scrolled-away request be cancelled.

---

## 2. The video player

Reference: `web/src/player/Controls.tsx` (bar, `Menu`, `MenuChoice`, `OptionsSheet`), `VideoPlayer.tsx` (touch,
compact, sheet items), `hooks.ts` (`useIdle`), `Remote.tsx` (remote lift), `TrackPicker.tsx`, `player/bar.css`
(everything visual, in its last third).

### 2.1 Simple bar (phones, small tablets, and anyone who picks it)

- [ ] **Top:** back (keeps playing in the corner), the title (one line, truncated), Cast. Watch party and the mini
      player move into Options.
- [ ] **Middle of the picture:** −10, Play/Pause, +30, centred on the **picture's** centre (not the screen's when the
      remote sheet is up, see 2.4). Play 80 pt, the skips 56 pt circles at 55% black, gap `clamp(28, 12vw, 64)`.
      A 32% black shade covers the picture while the controls show, and fades with them.
- [ ] **Bottom row:** the time (`0:10 / 2:32:21`, tabular figures) on the left; WittyRemote (when the title has it),
      Options (gear) and Full screen on the right, 44 pt each.
- [ ] **Seek bar under it:** a 36 pt tall touch area, the knob always visible (85% size; 125% while dragging).
- [ ] **Options** opens a sheet (iOS: `.sheet` with `.presentationDetents([.medium, .large])` and a grabber;
      Android: `ModalBottomSheet`). Rows, in this order, each with its current value on the right:
      Audio (only with 2+ tracks) · Subtitles (Off or the track) · Video version (angles) · Quality (`1080p`) ·
      Playback speed (`Normal`, `1.5×`) · Scenes (the current scene's name) · Watch party (`3 watching`) ·
      Keep playing in a corner · Viewing settings · How this is playing.
      Speed opens a page of the sheet with a back arrow (0.5× to 4×, `may be muted` beside speeds over 4×).
- [ ] Viewing settings, Quality and Scenes open as sheets too, never as corner pop-overs on a phone.

### 2.2 Detailed bar (iPad landscape, Android tablets wide)

One row, never wrapped: time and volume left; Prev, −10, Play, +30, Next centred; Audio, Subtitles, the quality
chip, the speed chip, Chapters, WittyRemote, More and Full screen right. Words beside icons only where all fit
(the web: a bar 1500 px wide); otherwise icons alone. Two ragged rows were the web's bug; don't port it.

### 2.3 Menus: behaviour (all platforms)

- [ ] Opening a menu puts focus on **the choice in use** (VoiceOver/TalkBack read "selected").
- [ ] The chosen row is **bold with a tick** at its end. A focused or pressed row gets a lighter fill. The two must
      never look alike (the owner's report: the old white bar on the chosen row looked bad and hover looked the same).
- [ ] Back / Esc / Menu button closes **the menu**, not the player. A tap outside closes it and does nothing else
      (it must not pause).
- [ ] The controls don't fade while a menu, a sheet or a picker is open.
- [ ] Track names: the language, then the track's own name **only if it isn't just the language again**; sound as
      **Mono, Stereo, 5.1, 7.1**; subtitle kind as **Text, Styled text** (ASS/SSA) or **Pictures** (PGS, DVD, DVB) —
      never `SUBRIP` or `6ch`.

### 2.4 Touch and the remote

- [ ] A tap on the picture shows the controls, or hides them if they're up. It never pauses (the play button does).
- [ ] A double tap on the left or right third skips 10 s back or on; in the middle third it toggles full screen.
- [ ] WittyRemote has its own button on the simple bar (owner's request: not inside Options).
- [ ] **Upright, with WittyRemote's sheet open, the picture moves up** to the middle of the space left above the
      sheet, and the middle buttons with it (the web measures the sheet and moves the picture's bottom edge up by its
      height, with a 0.3 s ease). Landscape keeps the picture where it is.

### 2.5 Scenes

- [ ] The scene picker shows a picture for every chapter of a file (1.4), its start time, its name, and "Now
      playing" on the current one. On a phone it takes the screen; elsewhere it's a panel over the bar.
- [ ] The seek bar's preview shows the picture of the scene under the finger/pointer.

### 2.6 tvOS and Android TV: use the platform's player UI

- [ ] **tvOS:** keep `AVPlayerViewController`. Put Audio, Subtitles (AVKit does these from the media options when
      it can), **Quality**, **Speed** and **Scenes** in `transportBarCustomMenuItems` as `UIMenu`s with
      `.singleSelection` children (the tick comes for free). Give **chapters with pictures** through
      `navigationMarkerGroups` (`AVNavigationMarkersGroup` of `AVTimedMetadataGroup`s carrying the title and the
      artwork from 1.4): the Siri Remote then scrubs by chapter with thumbnails, the tvOS way.
- [ ] **Android TV:** Media3 `PlayerView` with a TV control layout, or Leanback `PlaybackSupportFragment`.
      **Down** on the picture shows the controls and moves focus to them (TV volume is the TV's: don't map Up/Down
      to volume on a TV — the web's TV mode should follow this too, listed in #438). Thumbnails while seeking come
      from `PlaybackSeekDataProvider` fed by 1.4. Back closes an open menu first, then the controls, then the player.
- [ ] Focus rings: the platform's (tvOS lift and parallax; Android TV's focus highlight). Every target ≥ 66 dp/pt.

### 2.7 iPhone/iPad only

- [ ] **Picture in Picture** (`AVPictureInPictureController`): the web can't do it well, the apps should. The web's
      "Keep playing in a corner" maps to PiP on iOS.
- [ ] Respect the Dynamic Type sizes in the sheet rows (values truncate with an ellipsis, labels wrap).

---

## 3. The music player

Reference: `web/src/audio/NowPlaying.tsx`, `look.ts`, `ArtView.tsx`, `LookPanel.tsx`, `look.css`, `MiniPlayer.tsx`.

### 3.1 The looks, to the number

Every look fills a square box (the cover's box). Movement happens **only while playing** and **never with Reduce
Motion** (iOS `UIAccessibility.isReduceMotionEnabled`; Android `Settings.Global.ANIMATOR_DURATION_SCALE == 0`
or the TV's "remove animations").

| Look | Drawing | Motion |
|---|---|---|
| Cover (`square`) | Cover, radius = the theme's large radius, the player's shadow | none |
| Round (`circle`) | Cover clipped to a circle | none |
| Record (`vinyl`) | Black disc (#141416) with grooves: concentric rings every ~2.4 px alternating #121214/#1d1d21; a faint ring at 34%; the cover as the **label**, a circle inset 31% on each side; a centre hole 3.2% wide; a **static** sheen (a conic gradient, two soft white wedges ~10% and 7%) that does *not* turn | label + grooves turn 1 rev / 1.8 s (33⅓ rpm); in the bar 3.6 s |
| Disc (`cd`) | Cover clipped to a circle with a 13% transparent hub ring; a rainbow conic overlay (screen blend, 35%); a clear hub 26% wide; a static sheen | turns 1 rev / 3.2 s |
| Snapshot (`polaroid`) | Off-white frame (#f6f4ef), padding 5.5% sides/top, 17% bottom, rotated −2.5°; the song title handwritten in the bottom band (system handwriting font) | none |
| Float (`tilt`) | Cover with a big shadow and a moving soft light | leans up to 14° (Y) and 10° (X) towards the pointer, eased (each frame closes 12% of the gap); idle sway ±1° over 9 s. **tvOS:** lean with the Siri Remote's touch surface (`GCController.microGamepad` dpad values) or use the focus engine's own parallax (`.hoverEffect(.lift)` / `UIFocusEffect`); **iPhone:** device motion (`CMMotionManager`, gently) or idle sway only |
| Drift (`drift`) | Cover inside its frame | Ken Burns: scale 1.06 → 1.18 → 1.12 with small pans (−4%, 3%) → (3%, −3%), 26 s, alternate, ease-in-out |

Small previews (the Look panel's tiles) are drawn the same way, still: Float is shown leaning (10°, −18°), Drift
zoomed (1.3, pan −7%/5%).

**Reflection** (under Cover, Float and Drift only): the cover flipped, 26–64 pt tall, faded out with a gradient mask.

**Pulse with the beat:** read the low end of the sound once a frame while the art is on screen. Web: an analyser of
FFT size 2048, bins 1–7 averaged (0–255), `level = max(0, (avg − 120) / 135)`, rising 55% of the way per frame and
falling 12% per frame; scale = `1 + level × 0.035`.
- iOS: an `MTAudioProcessingTap` on the `AVPlayerItem`'s audio mix, or `AVAudioEngine` if the app already plays through
  it; compute RMS of a low-passed signal instead of FFT bins (cheaper) and map it to the same 0–1 curve.
- Android: **not** `android.media.audiofx.Visualizer` (it needs RECORD_AUDIO). Use a Media3 `TeeAudioProcessor`
  (or a custom `AudioProcessor`) to read PCM and compute the level. Stop reading when the art isn't on screen.

### 3.2 What's behind the player

| Backdrop | Drawing |
|---|---|
| Glow | the existing colour glow from the cover (radial, top centre) |
| Blurred cover | the cover full screen, blur 48 pt, saturation 1.35, 62% opacity, scaled 112%, under a black gradient 35% → 60% |
| Whole cover | the cover full screen, 50% opacity, under the same gradient |
| Plain | nothing (the player's black) |

Plus Canvas (behind everything, under a veil) and the visualizer, as before. iOS: `.blur(radius:)` on an `Image` in
a `ZStack` is fine; for performance render the blurred cover **once per song** to a small bitmap (CIGaussianBlur on a
256 px copy) and scale it up. Android: `RenderEffect.createBlurEffect` (API 31+) or a pre-blurred small bitmap
(Toolkit/RenderScript replacement) on older TVs. Android TV boxes are slow: pre-blur, never blur live.

### 3.3 Canvas in the cover's place (every view)

When mode is canvas and `canvasPlace` is `art` or `both`, the cover's box plays the canvas instead of the picture,
**clipped to the look's shape** (circle, record label, disc, snapshot frame). Views: the full player, the **bar /
mini player** (a 52 pt square; only the shape changes there, never Float or Drift), the **lock screen artwork stays
the still cover** (artwork for `MPNowPlayingInfoCenter` / MediaSession must be an image), and **karaoke's cover
beside the words**. The still cover shows until the video really plays (no black flash), and again if it can't.

### 3.4 The Look panel

Web: a side panel (a sheet on a phone) titled **Look**, with:
1. **Every song | This song** (a dot when this song has its own look; "Use every song's look" forgets it). Opens
   on **This song** when the song has its own.
2. **Cover:** seven tiles, each a little picture of the look *drawn with this song's cover*, with its name.
3. **Behind the player:** Glow, Blurred cover, Whole cover, Plain, Canvas video ("Make one" for an administrator
   when the song has none), Visualizer — little scenes of each.
4. **Canvas video:** Behind everything | In the cover's place | Both, and (administrators) "Edit the Canvas video…".
5. **Touches:** Reflection under the cover · Pulse with the beat (switches).

- [ ] **iPhone/iPad:** a `.sheet` with `.medium/.large` detents; tiles in a `LazyVGrid` (adaptive 84 pt); the
      selected tile has a 2 pt ring and `accessibilityAddTraits(.isSelected)`.
- [ ] **tvOS:** a full-height panel on the right (like the Lyrics panel), tiles in a focusable grid; the focused tile
      lifts; Select chooses. "Edit the Canvas video…" shows the wall message (6).
- [ ] **Android TV:** the same panel, D-pad between tiles, Back closes it.
- [ ] Changes apply at once and write `/me/ui` (1.1).

### 3.5 Other changes to carry over

- [ ] **Lyrics and Read are one button.** The panel has a switch at its top: **In time** | **Read** (Read shows the
      words at your own pace and the album's booklet, with the booklet's count). L opens it the way it was last read.
- [ ] **Speed is in More** (0.75× to 3×, a 4-column grid of buttons), on every screen. Away from normal, a chip under
      the controls shows the speed; pressing it goes back to normal.
- [ ] On a phone the top row drops Volume and Fill the screen, and the sleep timer moves into More.

---

## 4. Karaoke

Reference: `web/src/audio/Karaoke.tsx`, `nowplaying.css` (end of file).

- [ ] **Phone:** the words have the screen; nothing scrolls them away. The bar: previous line, play, next line and
      **Mix and more**, a sheet with: the Voice / Background music / Track levels (each a row: name, a slider taking
      the rest, the percentage; the names in one 9.5 rem column so the sliders line up), Voice off, Guide voice,
      Reset, Key, Words sooner/later, Voice settings, the controls chooser, Follow along | Read, what's behind the
      words, Album art beside the words, Fill the screen. Back closes the sheet first. Title and close share one row.
- [ ] **The cover beside the words** follows the song's look (a turning record beside the words is the showpiece),
      and plays the canvas in its place when the look says so (3.3).
- [ ] **Behind the words** follows "Behind the player" (art → blurred cover for Glow/Blurred, the whole cover for
      Whole cover, nothing for Plain; Canvas; Visualizer).
- [ ] **TV:** the mix and key in a side panel reached with Down from the transport; the words stay big.
- [ ] Voice settings (echo, reverb, the 15 controls and presets) is an **advanced setting**: wall it off on phones
      and TVs (6) but keep the presets the person chose on a computer working.

---

## 5. The Canvas studio (computers only)

Reference: `web/src/canvas/CanvasStudio.tsx`, `LoopEditor.tsx`, `studio.css`; server
`internal/server/api_canvas_studio.go`, `api_canvas_generate.go`. Routes: `POST /media/{id}/canvas/generate`
(`fromId`, `startSec`, `durationSec`, `crossfade`), `PUT /media/{id}/canvas/source` (a clip ≤ 512 MB),
`PUT|DELETE /media/{id}/canvas/loop`. All administrator-only.

**The apps do not port the studio.** They show the wall message (6) where the web has the studio, and play what it
made (1.3). The macOS desktop app is the web client and gets it as it is. If, later, the owner wants a phone-sized
"trim" (start and length only, no filmstrip), it is a separate request.

---

## 6. Walled-off advanced settings

The owner's rule (2026-11-18): *don't be afraid to wall off advanced settings on phone and TV platforms; if
something is complicated, show a message saying to use a desktop client or web browser. The settings made on a
computer must still apply there.*

**The message** (web: `web/src/ui/advanced.tsx`), with the thing's name in it:

> **A Canvas video is made on a computer**
> A Canvas video needs a bigger screen and a mouse or trackpad. Open WittyMediaServer in a web browser on a
> computer, or in the desktop app, to make changes. What you choose there shows here too.
> [OK]

Walled off on phones, tablets and TVs (and in the web's TV mode and on touch screens up to 1100 px):

| Setting | Where the web has it | What still applies on the device |
|---|---|---|
| Canvas studio (cut, upload a clip, loop editor) | Look → Edit the Canvas video…; Canvas on title and album pages | the canvas, its loop and crop (1.3) |
| Quality → Advanced (rate cap, codec, sound channels) | the video quality menu | the device's quality choice; the account's limits |
| Karaoke Voice settings (effects chain) | karaoke bar | the chosen effects and preset |
| Visualizer effect sliders (shader presets, intensity, speed) | full player → Effects | the chosen effect and its settings |

Not walled off (they are choices, not settings): the Look panel, the control bar choice, speed, tracks, scenes,
the mix levels and key in karaoke.

- [ ] Apple: one `AdvancedOnComputer` view (an `.alert` on iPhone, a full-screen message on tvOS with one focused OK).
- [ ] Android: one dialog composable, focused OK on TV.

---

## 7. Test plan on the Mac

Use real media (the owner's library over the network) with `saveMetadata` off on the test server.

- [ ] **iPhone 15 / SE simulator, portrait and landscape:** the simple bar; tap shows/hides, never pauses; double-tap
      skips; Options rows and values; speed page; scenes with pictures; WittyRemote lifts the picture (portrait only);
      Back closes sheets before the player.
- [ ] **iPad, both orientations:** the detailed bar on one row; Simple chosen by hand works too.
- [ ] **Apple TV 4K simulator:** transport bar menus (Quality, Speed, Scenes), chapter markers with pictures;
      focus never lost after closing a menu; Menu button order (menu → controls → player).
- [ ] **Music:** every look in the full player and the bar, with and without a cover; Record/Disc stop when paused;
      Reduce Motion stops all movement; Low Data Mode shows the still cover; a song with its own look and one without;
      a look changed on the web shows on the phone after reopening the player.
- [ ] **Canvas:** behind, in the cover's place (round and record), both; a canvas with loop points loops exactly that
      part with no flash at the seam; a letterboxed film's canvas fills the circle.
- [ ] **Karaoke:** phone sheet; the cover beside the words in the song's look; the wall on Voice settings.
- [ ] **Accessibility:** VoiceOver reads menus as "selected" on the current choice; Dynamic Type XXL in sheets;
      Switch Control reaches every control; TalkBack and D-pad on Android TV.
- [ ] **Performance:** an older Apple TV HD and a low-end Android TV box: the record's turning and the blurred
      backdrop at 60 fps (pre-blurred bitmap), the pulse costing under 2% CPU.

---

## 8. Known gaps (not done anywhere yet)

- The web has no browser Picture in Picture; the "mini player" is in-page. The apps should use the system's.
- The web's TV mode still maps Up/Down on the picture to volume (2.6 says not to on a TV); fix it on the web too.
- The quality menu offers sizes whose rate is above the original's (1080p at 8 Mbps for a 6.4 Mbps file).
- The episode scanner turned "80's Cats" into "'s Cats" (the number read as an episode).
