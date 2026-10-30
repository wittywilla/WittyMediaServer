# Blu-rays and the WittyBluray engine

WittyMediaServer plays Blu-ray images (`.iso`, `.img`) and BDMV folders **with their real
menus**, in a browser. The part that does this is **WittyBluray**: an original implementation of
the Blu-ray Disc navigation model, the same logic a player runs. It is its own Go module,
`modules/wittybluray` here and
[github.com/wittywilla/WittyBluray](https://github.com/wittywilla/WittyBluray) on its own.

- [Adding discs](#adding-discs)
- [Playing a disc](#playing-a-disc)
- [Audio, subtitles and chapters](#audio-subtitles-and-chapters)
- [3D, 4K and HDR](#3d-4k-and-hdr)
- [Performance on small machines](#performance-on-small-machines)
- [How WittyBluray works](#how-wittybluray-works)
- [Diagnosing a disc](#diagnosing-a-disc)
- [Limits](#limits)

## Adding discs

Put `.iso` or `.img` images, or folders that contain `BDMV`, in a Movies library. The scan
recognises a Blu-ray by its structure, not its name, and records its label, the feature's
length, resolution (up to 2160p, with HDR10, HDR10+ or Dolby Vision noted), codec, languages,
chapters, and whether it has menus. 3D discs get a "3D" genre.

Encrypted (AACS) discs are detected and shown as unplayable: WittyMediaServer never decrypts
anything. Make decrypted images of discs you own with a tool that is legal where you live.

## Playing a disc

On a Blu-ray's page:

| Button | What happens |
|---|---|
| **Insert Blu-ray** | starts the disc like a player: the first-play program (studio intros, warnings), then the top menu. Optionally with the tray animation and drive sound, as for DVDs |
| **Play movie** / **Resume** | straight into the feature, or back to where you stopped |
| **Main menu** | the disc's top menu, skipping the intros |

In the menu, the arrow keys, the on-screen remote, a game controller or a TV remote move
between buttons and **OK** presses one; the disc's own program then runs. Pages change, the
feature starts, and so on, as the disc's authors intended. The player's **Top menu** key returns
to the top menu from the film, and **Resume** continues the film.

Discs whose menus are written in BD-J (Java) can't run them; they open on the feature instead,
with a note. Chapters, audio and subtitles are still available from the player's own controls.

## Audio, subtitles and chapters

- **Audio:** every track in the feature is listed: DTS-HD Master Audio, TrueHD, DTS, AC-3,
  E-AC-3, LPCM, with its language and channels. Surround tracks are sent as 5.1 when the
  settings allow, else stereo.
- **Subtitles:** the disc's own subtitle pictures (PG), drawn over the video at their exact
  positions and times, in any of the disc's languages. Forced subtitles (translations of
  on-screen text) show even with subtitles off.
- **Chapters:** the playlist's chapter marks, with thumbnails on the disc page.
- **Angles:** multi-angle scenes play the selected angle (the *Angle* key).

## 3D, 4K and HDR

- **3D (MVC) discs** play their 2D (base) view. The disc is marked 3D in the library. Discs
  made from side-by-side or over-under 3D video play as they are.
- **4K UHD discs** (2160p HEVC) are read like any other. By default they are sent as 1080p,
  because encoding 4K in software is beyond most machines and most screens that play it are
  1080p; set a maximum height of 2160 in Settings to send full 4K to a machine that can.
- **HDR** (HDR10, HDR10+, and the HDR10 layer of Dolby Vision) is tone-mapped to SDR for the
  browser with FFmpeg (`zscale` + `tonemap`, present in the bundled builds). When an FFmpeg
  without those filters is used, HDR plays unmapped: watchable, but flat.

## Performance on small machines

- Only the bytes being played are read; nothing is copied or remuxed on disk.
- Menus stream their short video loop at a low-latency preset; the film streams at the
  settings' quality and height.
- The **Low power** setting uses the cheapest deinterlacer and the fastest encoder preset.
- Subtitle pictures are decoded in two-minute windows as the viewer reaches them, from the
  clip's entry-point map, so a 20 GB film is never scanned whole.

## How WittyBluray works

```
disc image ──► WittyDisc (UDF 2.50) ──► index / movie objects / playlists / clips ──► navigator + HDMV VM ──► what to play
                                                                                       │                      ├─ menu: playlist video + button pages
                                                                                       │                      └─ title: playlist video + subtitle pictures
                                                                                       └─ transport-stream demuxer, IG and PG decoders
```

1. The image is read directly (no mounting) by WittyDisc, which understands the UDF 2.50
   metadata partition Blu-rays use.
2. The disc's program is parsed: `index.bdmv`, `MovieObject.bdmv`, the playlists and the clip
   information.
3. The **HDMV virtual machine** runs the disc's commands (4096 general and 128 player status
   registers; branch, compare, set and system commands).
4. The **navigator** follows first play → menus → titles exactly as the commands dictate,
   including calls, resumes and timers.
5. For a menu it reads the **interactive graphics** stream (pages, buttons, pictures, commands)
   and renders the page and each button's highlight; for a film it reads the **presentation
   graphics** stream for subtitles.
6. The video of each playlist is converted to HLS by FFmpeg from the clip's byte ranges and
   shown in the browser with the button and subtitle layers on top (`internal/bdsession`).

Everything is written from the public Blu-ray structure; no code from other players is used,
and AACS discs are refused.

## Diagnosing a disc

```bash
wms bluray info   THE_LORAX.iso          # titles, program, playlists, clips, streams, 3D/HDR flags
wms bluray walk   THE_LORAX.iso          # simulate inserting the disc; the menu's buttons and commands
wms bluray walk   THE_LORAX.iso 2 5      # ...then press buttons 2 and 5
wms bluray render THE_LORAX.iso out/     # the menu's pages as PNG files
```

The same commands exist as a standalone tool, `wittybluray`, from the
[WittyBluray repository](https://github.com/wittywilla/WittyBluray). When reporting a disc
problem, attach the output of `info` and `walk`; never the disc image.

## Limits

- AACS-encrypted discs are refused.
- BD-J (Java) menus don't run; the disc plays its titles.
- 3D plays as 2D; pop-up menus during the film are not shown yet; menu animations and page
  transitions are not played (the first picture of each button state is shown).
- 4K and HDR handling has not yet been tested on a real UHD disc.

---
<sub>WittyMediaServer documentation</sub>
