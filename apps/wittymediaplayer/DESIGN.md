# WittyMediaPlayer design note

The decisions behind the player, and why. Written when it was first built, in October 2026.

## Decisions

| Question | Decision | Why |
|---|---|---|
| Interface | **Qt 6 Widgets** (6.10), linked as shared libraries | The owner asked for Qt 6. Widgets need far less memory and start faster than a declarative interface on small machines, and they bring keyboard use and screen-reader support with them. |
| Getting Qt | `aqtinstall` from Qt's own servers, into a folder in the user's home | No account, no administrator rights, nothing installed system-wide. It can't yet read the repository layout Qt uses from 6.11 on, which is why this is 6.10. |
| Compiler | **MinGW 13** from Qt's tools | The same way: no administrator rights and no sign-in. Visual Studio would need both. |
| Qt licence | **LGPL v3**: shared libraries beside the program, so they can be replaced, with notices in `licenses/` | What the owner asked for. No static linking, and no modules that are GPL-only or commercial. |
| Decoding | **FFmpeg's libraries**, built here with only what a player needs | See below. |
| Picture | **QRhi** (`QRhiWidget`), which uses Direct3D 11 on Windows, Metal on Apple and Vulkan or OpenGL on Linux | Colour conversion and scaling happen on the graphics card, where they cost almost nothing. It's the most important thing for low-end machines, and it isn't tied to one platform. |
| Hardware decoding | D3D11VA for H.264, HEVC, VC-1, VP9 and AV1, copied back to memory | Cheap to add and much lighter than software decoding at 1080p and 4K. Passing the decoded picture straight to the screen without the copy would save more, and is a later step. Other systems' decoders come with their releases. |
| Sound | `QAudioSink` in a thread of its own | Part of Qt, on every platform. It is also the clock the picture follows. Qt's sound library won't load without `Qt6Network.dll`, so that file ships, but the player doesn't link to it, nothing calls it, and Qt's network plugins are left out. |
| Go to Qt | A **helper process** over a pipe (`wittymediaplayer-engine`), speaking the WittyModule helper protocol | See below. |

### Why a helper process, not a shared library

The disc engines are Go. Building them as a C library would need cgo and a second copy of the Go
runtime inside the window's process. A helper keeps them apart, which brings three things:

- **Safety:** discs are untrusted. A damaged or hostile one can only take down the helper, and the
  window says so plainly.
- **Portability:** the helper is pure Go. It cross-compiles for every platform with no C compiler.
- **Simplicity:** the protocol already exists in `modules/wittymodule`. The helper has one job and
  needs nothing but its stdin and stdout. A test checks it contains no network code at all.

### Why this FFmpeg, and not a ready-made one

The prebuilt LGPL Windows builds of FFmpeg include `libbluray`, which looks for `libaacs` and `libbdplus` at
run time. The player would never reach that code, but a program that promises not to load decryption
libraries shouldn't be carrying a loader for them. So `scripts/setup-toolchain.sh` builds FFmpeg with:

- only the decoders, demuxers and parsers a player needs (`--disable-everything`, then a list);
- `--disable-network`, so there's no network code to misuse;
- no `libbluray`, `libdvdread` or `libdvdnav`;
- no GPL or non-free parts.

The libraries come out a small fraction of the size of a general build, which also helps a small
machine. `scripts/check-package.sh` stops the build if any of this slips.

### Why own playback code, and not `QMediaPlayer`

`QMediaPlayer` would be less code, but a disc needs what it doesn't offer: choosing audio and subtitle
streams by the id the disc gives them, reading a title from byte ranges with clocks that restart between
cells, and deinterlacing (DVDs look wrong without it). It would also accept whatever a file points to,
including playlists with web addresses. The player here has a fixed list of formats it opens, and files
are read by the `file` protocol and nothing else.

## How a disc plays

```
wittymediaplayer.exe                    wittymediaplayer-engine.exe
┌──────────────────────────────┐        ┌────────────────────────────┐
│ DiscController               │  JSON  │ WittyDVD / WittyBluray     │
│  presents, holds, loops  ◄───┼────────┼─►  navigator, VM, menus    │
│ Player                       │  pipe  │ internal/disc              │
│  byte ranges ─► FFmpeg       │        │  steps ─► presentations    │
│ VideoView                    │        │ internal/drives            │
│  picture + menu + subtitles  │        │ internal/imaging           │
└──────────────────────────────┘        └────────────────────────────┘
```

`internal/disc` follows the server's `internal/dvdsession` and `internal/bdsession`, and
`DiscController` follows `web/src/player/DvdPlayer.tsx`. The presentations have the same shape and
the same rules, with one change: where the server starts an HLS stream, the helper hands over byte
ranges and the player decodes them directly. That means no transcoding, no temporary files and no
loopback server.

The Go `internal` rule stops the helper importing the server's session packages, so this is a copy
adapted for direct playback. Moving the shared part into a module both can use is worth doing when
either side next changes much.

**Subtitles** are decoded in the stream as it plays, not by the helper reading the disc a second time. That
matters on a real drive, where a second reader means constant seeking. Blu-ray text subtitles and subtitles in a
sub-path aren't in the main stream, so the helper still provides those.

**Seeking** uses the disc's own structures: a Blu-ray clip's entry-point map (no read at all) and a
search through a DVD cell's NAV packs (a few reads).

## Playing from a drive

- `internal/drives` lists optical drives and what's in them. On Windows a disc appears as files on its
  drive letter, and the engines already read folders, so a disc plays through the file system.
- A read that fails is retried, then skipped and counted: playback carries on, as a disc player does on a
  scratch, and the window says part of the disc was skipped. If the file has gone, the disc was taken out.
- Stopping can't get stuck on a drive retrying a bad sector: the read is cancelled, and clean-up happens off
  the window's thread.

**Copy protection on a drive** is checked three ways, because a drive behaves differently from a file: asking the
drive for the DVD's copyright marking (reads a label, unlocks nothing), sampling the feature for scrambled
packs, and treating a feature that won't read at all as a refusal. Drives withhold protected sectors from a
player that hasn't unlocked them, and this one never does.

## Making disc images

`internal/imaging` copies a disc sector by sector to `.iso` or `.img`, for a WittyMediaServer library. The file
is byte-for-byte what the drive hands over, which is what the server's engines read.

- It refuses protected discs before writing anything, and stops and removes the partial file if a scrambled
  sector appears. It never unlocks, decrypts or works around anything.
- It writes to a `.part` file and takes the final name only when finished. It never overwrites.
- Unreadable sectors become zeros and are counted, with retries first. Past about 100 MB of them it stops.
- When finished, it opens the image with the same engines the server uses and says whether it reads.

The player doesn't talk to the server, since nothing leaves the computer: the viewer puts the file in a
folder the library watches.

## Other platforms

Nothing here closes a door. The window is Qt, the helper is pure Go, and these are the places a port needs work:

| Place | What it needs |
|---|---|
| `Session::preferredHwType` in `player.cpp` | VA-API or VideoToolbox |
| `internal/drives/drives_other.go` | Lists mounted drives on Linux; needs unmounted drives, macOS, whole-disc reads and the drive's copyright query |
| `scripts/` | A toolchain and package script per platform; FFmpeg's flags carry over |

## Known limits of the first release

- **Blu-ray Java menus** don't run. Such a disc plays its titles instead, and the window says so. The server's
  BD-J runner needs an installed Java and is a separate step.
- **Sound outside the main stream** (a browsable slideshow's music) isn't played.
- **Sound is stereo.** Surround tracks are mixed down.
- **DVD subtitles** use FFmpeg's decoder. Colour changes within a subtitle (CHG_COLCON) that the engine's own
  decoder handles aren't shown.
- **Audio CDs** aren't supported.
