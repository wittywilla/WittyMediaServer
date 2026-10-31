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

## The VLC libraries, as an option

The owner asked for the VLC libraries as a choice in Settings, with clear credit. They're credited in Settings, the
About window, on the start page while they're in use, in the README, the NOTICE and the licences.

**Not shipped.** `src/media/vlcplayback.cpp` loads libVLC 3 from a VLC the viewer has installed, or a folder they name,
the same arrangement as Java for BD-J menus. A VLC install carries `libdvdcss` inside its disc modules, and bundling
those would ship a decryption library, so they stay out of this package.

**The no-bypass rule still holds, because VLC never reads a disc itself:**

- Protected discs are refused by the helper before any playback starts, whichever library plays.
- Everything reaches VLC through this player's own reader, as a plain stream of bytes, so it never opens a drive, a path
  or a web address. Its demuxer is named for every stream (`ps`, `ts`, or a fixed list for files, ending in `none`), so
  its disc modules are never chosen, even for a disc image with an innocent file name. A test checks they aren't loaded.
- On a computer whose VLC has no plugin cache, VLC scans its plugins once by loading them, including the disc modules. Their
  code isn't run. An installed VLC comes with the cache.

**What differs through VLC:** disc menus still come from WittyDVD and WittyBluray. VLC draws subtitles into the picture, so
there's no choice of only forced captions, and DVD subtitles may not have the disc's colours. It finds subtitle files only as
ordinary tracks, not beside a video. The seek bar on a disc follows VLC's place in the stream, so it's approximate. It does
play surround sound.

## Disc modules update on their own

The disc modules follow the same rules as WittyMediaServer's modules. See `docs/MODULES.md` in that repository.

| Module | What it is | Lives in |
|---|---|---|
| `wittydvdplayer` | WittyDVD and its session, as a helper program | `modules/wittydvd/wittydvdplayer` |
| `wittyblurayplayer` | WittyBluray and its session, as a helper program | `modules/wittybluray/wittyblurayplayer` |
| WittyDisc | The shared reader, and the `session` contract | A library: it arrives inside those two |

- **A built-in copy always works.** The same packages are linked into `wittymediaplayer-engine`. With nothing downloaded, nothing
  else runs and no helper starts.
- **A newer signed release runs as a process of its own,** started for an open disc. It uses the same manifest (`module.json`), the same
  version rules, the same release files and the same signing key as a server's module. In a player's module, `minServer` and `maxServer` are
  the versions of WittyMediaPlayer it works with.
- **Installing:** check the signature, the manifest and whether it fits this player; download and match it to the signed list; unpack with no
  links and nothing climbing out; check every file, with nothing extra or missing; start it once and hear it say the right name and version;
  move it into place. Each step leaves what's in use untouched if it fails. Modules live in the viewer's own data folder, not the program's.
- **Every start checks again,** so a file changed afterwards is refused, and the built-in copy carries on, with the reason in Settings.
- **Going back:** three process-level failures in a row take a version out by itself. A viewer can also go back by hand, and only the current and
  previous versions are kept. A disc a parser refuses doesn't count as a failure.
- **Limits on a helper,** on Windows: a low-integrity token (it can read discs and can't write to the viewer's files), a job with a memory cap
  that can't start other programs and ends with the player, and a call time limit.
- **Copy protection is checked by the built-in copy** before a downloaded version sees a disc, and imaging always uses the built-in copy, so
  what a download says about protection is never taken on trust.
- **The network:** only `wittymediaplayer-updater` has network code, and it runs only when the viewer asks, or at start if they've turned that on. CD lookups go through it too (below).
  It talks only to GitHub over HTTPS. A test checks that `wittymediaplayer-engine` has none.

**Releasing one:** in the module's own repository, tag `wittydvdplayer-vX.Y.Z` on a release, with these files from
`wittymodule pack` and `wittymodule sign`, run in that folder. Those tags are separate from the engine's own `vX.Y.Z`, which a server reads.

**Not yet here, on the server's branch:** the signed list of withdrawn versions, choosing an older version, pinning, and forks. They can follow once that
branch is merged, since they build on `wittymodule` 0.2.

## Audio CDs

**Reading.** `engine/internal/cdrip` has one `Source`, by sector: a drive, through the same ioctls any CD player uses
(`IOCTL_CDROM_READ_TOC` and `IOCTL_CDROM_RAW_READ`), or an image, through WittyCD, which already reads every form images
come in. Nothing here issues vendor commands, reads subchannels to get around anything, or treats a failing disc as a
puzzle: a disc that won't read gets `ErrNoRead`'s plain message, and a track that's mostly unreadable stops there.

**Careful reads.** Careful mode reads each chunk (about half a second) twice and compares checksums, without keeping any of
the first pass on disk: only its checksums are held. A chunk that differs is read until two reads in a row agree. A read
that fails is retried, then taken apart sector by sector, and sectors that never read become silence and are counted, so
one scratch costs a short re-read and a true count rather than a lost track. The drive offset shifts which samples are read,
and reaches past the disc's ends as silence. An image can't vary, so it's read once.

**The split.** The helper reads; the window encodes. `--cd-rip` sends JSON lines with the samples between them, so the
helper never writes a music file and the encoders stay in the FFmpeg libraries the window already has. `--cd-image` is the
exception, because a `.bin` needs no encoding: it writes `.part` files and renames them at the end. An image holds a disc's
audio only. A data track on the same disc is left out, since reading it needs a different kind of read, and the image's
sheet is rebased to match.

**Playing.** `--cd-stream` answers requests for ranges of a WAV covering the disc, which `CdStream` hands to the demuxer in
place of a file. So a CD goes through the normal player: seeking, sound, clock and all. A sector that won't read plays as
silence. CDs always use the built-in decoder, even with the VLC option on, since there's no reason to hand that library a
source it would then try to treat as its own.

**Encoders.** FFmpeg gained FLAC, ALAC, AAC and PCM (its own), MP3 (LAME), Opus and Vorbis. The list is still explicit,
and `check-package.sh` fails the build if any are missing or if the licence stops being LGPL. Tags go in the container's
metadata; covers go in as an attached picture, or as the picture tag Ogg files use. WAV takes neither reliably, so it's left
without a cover.

**Lookups.** The helper still has no network code, and a test still checks that. `wittymediaplayer-updater --cd-lookup` asks
MusicBrainz, and `--cd-cover` the Cover Art Archive, only when the viewer presses **Look up names**. Its inputs are checked
to be a disc ID and numbers, so nothing else can go out. Names that come back are untrusted, and `ripRelativePath` is where
they become files: each part is cleaned, so a title can't climb out of the chosen folder or take a name Windows reserves.

**The screen.** `CdPage` is original work: the gel buttons, seven-segment time, disc artwork and glass are all drawn in code,
with its own blue, teal and amber. It evokes that early-desktop feeling without borrowing any product's skin, layout or
artwork. It can follow the app theme instead.

**The visualizer.** Everything reads WittyVisualizer's `SampleTap` in `AudioOutput`, which costs one atomic read when off.
The screen's own `VisualizerView` draws on the processor, small and scaled up, and stops when hidden, so a machine with no
useful graphics card still has something to watch. **More effects** puts the shared `wv::VisualizerWidget` in its place; only
one is ever on, and the one leaving turns the tap off before the next turns it on.

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
- **Built-in sound is stereo.** Surround tracks are mixed down.
- **DVD subtitles** use FFmpeg's decoder. Colour changes within a subtitle (CHG_COLCON) that the engine's own
  decoder handles aren't shown.
- **Audio CDs from a drive** are written for Windows only; images work wherever the helper runs.
