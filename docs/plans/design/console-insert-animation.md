# Design: inserting a cartridge or disc in WittyPlay

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/51), in WittyPlay's own train.

For WittyPlay 0.4.3 to 0.4.5 ([the timeline](../modules/WittyPlay.md#043--insert-animations-the-engine--u)). The animation is interface, like a loading screen, drawn by the client while the server loads the game: no game code runs on the client, which keeps to the rule that every game runs on the server. The owner:

> Just like our animation with WittyDVD and WittyBluray, allow there to be an animation of inserting a cartridge or disc into the game console that is being emulated. Make the look of the consoles mostly-match the existing console design as well as the media that is being inserted. Make the cartridge look similar to the actual consoles cartridge. Allow the ability to customize this, and build a custom build option as well as documentation for people who want to make their own.

Not built. Not tested.

---

## What we build on

WittyDVD and WittyBluray already have an **insert effect**: a tray animation and drive sound when a disc is started, optional per disc, skippable with a click (`docs/DVD.md`, `docs/BLURAY.md`). WittyPlay already has per-console tables (`Consoles` in `consoles.go`), console icons through the optional logo service, and a data-only settings file format with strict limits (`custom.go`, 256 KB, fuzz-tested). This design reuses all three ideas.

---

## What a person sees

1. They press **Play** on a game.
2. A drawing of the console appears, with the game's media in front of it: a cartridge with the game's label art on it, or a disc with its disc art.
3. The media goes in the way it does on that console: pushed into a front slot and pressed down, dropped into a top slot, slid into a handheld's back, laid in a tray that closes, or set under a lid that shuts.
4. The power light comes on, the drawing fades, and the game's first frame is there.

**The animation hides loading.** The game starts loading the moment Play is pressed, in parallel. The animation is timed to end when the first frame is ready, with a minimum (so it isn't a flicker) and a maximum (so a slow load isn't held hostage to an animation): if the core is ready first, the animation speeds up its last step; if the core is slow, the animation holds on "power light on" with a quiet spinner on the light.

**It never costs time.** Any button, a click, a tap or Escape skips it. A game that fails to start ends the animation at once and shows the error.

---

## Looks: "mostly match", without copying anything protected

The owner asked for consoles and media that mostly match the real ones, and in the same file, for the disc icons, said: "Do not accidentally make a copyrighted logo or anything." Both apply here.

**Allowed:**

- Overall proportions and silhouette: a wide flat box with a front slot; a top-loading slot; a handheld with a cartridge slot at the back.
- The general colour scheme (grey and dark grey, black, purple, white), from simple flat shapes.
- How the media goes in (top, front, back, tray, lid).
- Cartridge *shapes* close to the real thing (a tall rectangle with grip ridges; a short wide cartridge; a small card), because a cartridge's shape is what makes it recognisable.
- The **game's own label or disc art**, from the person's own metadata, shown on the media.

**Never:**

- Logos, wordmarks, emblems, "seal" marks, or any text from the console or its maker.
- Tracing a photograph or a press image, or copying moulding details exactly.
- Boot sounds, start-up jingles or any sound recorded from the real console. The sounds are our own: generic clicks, a slot, a spring, a lid, a hum.
- The console maker's name on screen in the animation. (The console's name is on the game's page anyway.)

**A question for the private legal review** (kept outside the repository, per the standing rule): how close a drawn console shape may come to the real product's look. Until answered, err towards simpler shapes. The drawings are labelled as our own artwork, credited to the project, under the module's licence.

---

## Mechanisms by console

From WittyPlay's console table. Each console uses one of a small set of animation types, so a new console only needs its drawing.

| Type | Movement | Consoles |
|---|---|---|
| `front-loader` | Cartridge slides into a front door, then is pressed down; door closes | NES / Famicom (the Famicom drawing uses top slot instead) |
| `top-slot` | Cartridge lowered into a slot in the top; a power switch slides | SNES / Super Famicom, Nintendo 64, Mega Drive / Genesis, Master System, PC Engine (HuCard: `card-slot`), Atari 2600, Atari 7800 |
| `back-slot` | Handheld turns away; cartridge slides into its back or top edge | Game Boy / Color, WonderSwan, Neo Geo Pocket, Atari Lynx, Virtual Boy |
| `bottom-slot` | Cartridge slides into the bottom edge | Game Boy Advance |
| `card-slot` | A small card slides into a thin slot | Nintendo DS, Nintendo 3DS, PC Engine HuCard |
| `lid` | Lid swings open, disc is set on the spindle, lid closes, disc spins under the window | PlayStation, Dreamcast |
| `tray` | Tray slides out, disc set in, tray slides back | PlayStation 2 (the later slim models had a lid: offer both drawings) |
| `door` | A small door opens, a disc in a shell slides in | PSP |

Multi-disc games show the disc they start on. A disc change during play (see [WittyPlay](../modules/WittyPlay.md#0410--changing-discs-while-playing--s-i)) can play a short eject-and-insert, off by default.

---

## How it's built

### A console pack is data

```
packs/
  ps1/
    pack.json          what's in it, timings, which type
    console.svg        the drawing, in named layers
    media.svg          the disc or cartridge, with a named label area
    sounds/
      open.opus  set.opus  close.opus  power.opus
```

`pack.json`:

```json
{
  "wittyplayPack": 1,
  "console": "psx",
  "name": "Grey console, lid",
  "type": "lid",
  "layers": { "body": "#body", "lid": "#lid", "light": "#power", "window": "#window" },
  "media": { "shape": "disc", "label": "#label", "labelFit": "cover" },
  "timing": { "minMs": 1200, "maxMs": 4000, "skippable": true },
  "sounds": { "open": "sounds/open.opus", "set": "sounds/set.opus", "close": "sounds/close.opus", "power": "sounds/power.opus" },
  "credits": "Drawn for WittyPlay. Apache-2.0."
}
```

The *movement* belongs to the type (`lid`, `tray` and so on), built into WittyPlay's web part. A pack chooses a type and supplies drawings, named layers, timings and sounds. **Nothing in a pack runs.** That's what makes custom packs safe to share.

### Rendering

- SVG layers moved with CSS transforms and the Web Animations API. No canvas, no WebGL, no 3D library. That runs on old TV browsers and low-end boxes, and keeps each pack small.
- **A size budget:** a stock pack is under 60 KB compressed, sounds included. Packs load only when that console's game is started, and are cached.
- The label area takes the game's own picture, already on the server, through a clip path.
- `prefers-reduced-motion`: a short cross-fade with the media shown in place, no movement. Also a setting.
- Screen readers hear one line: "Starting <game>". The animation is hidden from the accessibility tree.

### Settings

**Settings → Games → Starting a game** (server default) and per profile:

| Setting | Choices |
|---|---|
| **Insert animation** | **Only while loading** (default: it never makes a start slower) · Always · Off |
| **Sound** | On (default) · Off. Follows the game's volume |
| **Per console** | Stock pack · Simple (a plain shape, for low-end devices) · Off · Any custom pack installed |

Per-game override on the game's page, like the per-disc insert effect on DVDs.

### Custom packs

**Who:** administrators add packs. Profiles choose among installed ones.

**How a pack is checked**, the same way the custom settings files are:

- A `.zip`, 2 MB at most, 64 files, no folders that climb out, no links (the same zip rules as custom cores).
- `pack.json` parsed with limits (size, depth, value counts), unknown keys listed and ignored.
- **SVG cleaned before it's stored:** only an allow-list of elements and attributes; no `<script>`, no `foreignObject`, no event attributes, no external references (`href` only to `#ids` in the same file), no CSS `url()` to anything outside, no `@import`. Then re-serialised by our own code, so what's stored is what the cleaner produced, not what was uploaded.
- Sounds: Opus or WAV only, sniffed by content, decoded and re-encoded on the server with FFmpeg, length limited to 3 seconds each.
- Served with the strict content security policy the game page already has.
- Errors are shown with where they are, as the settings file screen does.

### The custom build option

The owner asked for "a custom build option as well as documentation for people who want to make their own".

1. **`wittyplay-pack`**, a small command in the WittyPlay repository (`cmd/wittyplay-pack`), for every server platform:
   - `wittyplay-pack new <console> <type> <folder>`: a starter pack with placeholder layers already named.
   - `wittyplay-pack check <folder>`: every check the server does, with the same messages.
   - `wittyplay-pack preview <folder> [game-art.png]`: writes a short animated preview (APNG or WebM through FFmpeg) and a contact sheet of key frames, so a maker can see it without a server.
   - `wittyplay-pack build <folder> -o pack.zip`: the zip the server accepts.
2. **A template repository** (to add to `docs/REPOSITORIES.md` for the owner to create, if wanted): `WittyPlayPackTemplate`, with one pack per animation type, a README and a CI check that runs `wittyplay-pack check`.
3. **Documentation:**
   - In-app help page: **Help → Games → Insert animations**, for choosing and adding packs.
   - `docs/INSERT-ANIMATIONS.md` in WittyPlay, for makers: the layer names each type needs, how timing works, how to draw for small sizes, how labels are fitted, the cleaner's allow-list, the size limits, the licence to choose, and what's never allowed (logos, traced photos, recorded console sounds).
   - Every stock pack doubles as an example.
4. **Later (after 0.4.5):** an editor in the browser to arrange layers and try timings live. It would write the same data.

### Native apps

The pack is data, so native apps can draw it later with their own vector drawing (Android vector drawables, Apple Core Animation). For 0.4.0 they show the game's picture with a short fade, and say so in their release notes.

---

## Order of work

1. The types and timing engine in WittyPlay's web part, with the "only while loading" coupling to the session's first frame.
2. Drawings for the eight types, then each console. The most-played consoles first (by the library's play counts on a typical server: start with NES, SNES, N64, PlayStation, PS2, GBA, DS).
3. Our own sounds.
4. Settings, per-game override, reduced motion.
5. The pack checker, upload and `wittyplay-pack`.
6. Documentation and the template.
7. Accessibility, controllers (skip with any button), low-end check (a 2 GB Android TV box in the emulator, and the oldest browser supported).

## Done when

Every console in the table has a stock pack; a game starts no slower with the animation on than off (measured from Play to first frame); a custom pack made with `wittyplay-pack new` passes `check`, uploads, and plays; a pack with a script, an external link or a 10 MB file is refused with a clear message.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
