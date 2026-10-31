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
- [BD-J (Java) menus](#bd-j-java-menus)
- [Java and licensing](#java-and-licensing)
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
anything. Make decrypted images of discs you own with a tool that is legal where you live. A
decrypted image that still has its AACS folder plays normally, because detection looks at the
video itself.

AVCHD discs and camcorder cards, and BDAV recordings, are read too.

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

**Pop-up menus:** when the film's playlist carries a pop-up menu, the **Menu** key (or **m**)
opens it over the picture while the film keeps playing; the arrows and OK work it, and **Esc**
or Menu closes it. Discs use it for chapters, audio and subtitle choices with their own artwork.
Buttons whose pictures animate (a pulsing highlight) animate here too.

Discs whose menus are written in Java need the [BD-J setting](#bd-j-java-menus). Without it they
open on the feature instead, and chapters, audio and subtitles are still available from the
player's own controls.

Menu buttons make the disc's own sounds, pages play their transitions, and menus that move on by
themselves after a while now do. Region, parental limits and country are in **Settings → Transcoding**.
A disc can forbid seeking or skipping at some points, as on a disc player; to ignore that, turn off
**Disc restrictions** in the player's viewing settings.

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

- **3D (MVC) discs** play their 2D (base) view. The disc is marked 3D in the library.
- **Side-by-side and over-under 3D video** (discs or files made that way): the player's viewing
  settings have a **3D video** choice (side by side or over-under) and **Show 3D as**: one eye's
  view flat in 2D, or red/cyan anaglyph for the paper glasses. The choice is remembered per disc
  or for everything.
- **4K UHD discs** (2160p HEVC) are read like any other. By default they are sent as 1080p,
  because encoding 4K in software is beyond most machines and most screens that play it are
  1080p; set a maximum height of 2160 in Settings to send full 4K to a machine that can.
- **HDR** (HDR10, HDR10+, and the HDR10 layer of Dolby Vision) is tone-mapped to SDR for the
  browser with FFmpeg (`zscale` + `tonemap`, present in the bundled builds). When an FFmpeg
  without those filters is used, HDR plays unmapped: watchable, but flat.

## BD-J (Java) menus

Many commercial Blu-rays write their menus in Java. To show them:

1. Install any Java 8 or newer on the server, with its graphics libraries. [Eclipse
   Temurin](https://adoptium.net) is a free build of OpenJDK. On Linux avoid "headless" packages
   only if they lack `java.desktop`, and install `fontconfig`. In Docker, add both to the image.
2. In **Settings → Transcoding**, turn on **Run Blu-ray Java menus**. The page shows which Java it
   found; set **Java location** if it found none.
3. Insert the disc.

Each viewer's disc starts its own small Java process (192 MB at most), which ends with the session.
The disc's graphics are sent to the player as pictures at up to 12 a second, so fast animation looks
less smooth than on a hardware player.

If an application can't run, the disc plays its main feature instead. This has been tested on Java 8,
21 and 25 with our own test discs and with open-source sample applications, **not yet on a retail
disc**: none of the discs tried so far has Java menus. Please report discs that fail, with the output
of `wms bluray bdj <disc> 20`.

**Safety.** A BD-J disc contains code. It runs in a sandbox with no network and no access to files
outside the disc's own areas, and a hostile test disc tries to break out every time the tests run.
If a disc uses too much memory, threads or processor time, its Java process is ended and the feature
plays. That is still not an absolute guarantee, so this stays off until you turn it on. [The sandbox
in detail](BLURAY-QUIRKS.md#bd-j).

## Java and licensing

This section explains how BD-J support is arranged. It is not legal advice; if you plan to sell or
distribute this software commercially, ask a lawyer.

- **Java is not shipped.** WittyMediaServer contains no Java runtime. Whoever runs the server
  installs one, under that Java's own licence, and the server starts it as a separate program.
  OpenJDK builds such as Temurin are licensed under GPLv2 with the Classpath Exception. Oracle's own
  builds have their own terms, which are for the installer to check.
- **The runtime is ours.** `runtime.jar` is original code under Apache-2.0, with no code from Sun,
  Oracle or other players. It provides the classes disc applications link against (JavaTV, JMF, HAVi,
  DVB and BD-J names) by re-implementing their published interfaces, for interoperability with discs.
  In the US, [*Google v. Oracle*](https://www.jonesday.com/ja/insights/2021/04/supreme-court-google-wins-copyright-battle)
  (2021) held that such re-implementation of the Java API was fair use, without deciding whether
  APIs are copyrightable. In the EU, *SAS Institute v. World Programming* treats functionality and
  interfaces as outside copyright's protection. Neither ruling covers every country or every case.
- **It is not certified.** The runtime has not passed any compatibility test and makes no claim to.
  It does not call itself Java, and "BD-J" and "Blu-ray" here only describe which discs it plays.
- **Format licences are separate.** Blu-ray Disc has its own licensing for commercial players, and the
  specifications behind these APIs have their own terms. That applies to this project with or
  without Java, and it is the main thing to raise with a lawyer before any commercial use.
- **Copy protection is untouched.** AACS and BD+ are never decrypted or run.

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
wms bluray bdj    THE_LORAX.iso 20 a.png down enter # BD-J titles; with Java, run them for 20 seconds, pressing those keys
```

The same commands exist as a standalone tool, `wittybluray`, from the
[WittyBluray repository](https://github.com/wittywilla/WittyBluray). When reporting a disc
problem, attach the output of `info` and `walk`; never the disc image.

## Limits

- AACS-encrypted discs are refused, and BD+ is not run.
- MVC 3D plays as 2D (the dependent view is not decoded).
- Secondary audio and picture-in-picture are not mixed in.
- BD-J graphics reach the player at up to 12 pictures a second.
- Tested on custom-authored discs. Retail discs use more of the format: BD-J, MVC 3D and Ultra HD HDR
  are implemented, and tested only on synthetic data or not at all.

[Everything the format can do, and what happens with each](BLURAY-QUIRKS.md).

---
<sub>WittyMediaServer documentation</sub>
