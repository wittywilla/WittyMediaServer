# Third-party notices

WittyMediaServer (Apache-2.0, see LICENSE and NOTICE) bundles or depends on the following open-source components.

**Settings, About & legal** lists every library in the program you're running, with its version, license and full license
text. That list is made from what's really compiled in (`internal/licenses`), and this file is the summary of it.

## Bundled in release archives / the Docker image

| Component | Licence | Notes |
|---|---|---|
| **FFmpeg** (`bin/ffmpeg`, `bin/ffprobe`) | GPL-2.0-or-later (builds include libx264) | Run as a separate process for probing, transcoding and stills. Windows/Linux builds from [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds); macOS builds from [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de); FreeBSD built from source by `scripts/build-ffmpeg-static.sh`. Docker uses Alpine's `ffmpeg` package. Source: <https://ffmpeg.org>. If you need an LGPL-only deployment, point `WMS_FFMPEG` at an LGPL build with `libopenh264` or a hardware encoder; nothing in WittyMediaServer requires GPL components. |
| libx264 (inside FFmpeg) | GPL-2.0-or-later | <https://www.videolan.org/developers/x264.html> |
| LAME, Opus (inside FFmpeg) | LGPL-2.1 / BSD-3-Clause | |

## Go modules (statically linked)

| Module | Licence |
|---|---|
| modernc.org/sqlite (and its modernc.org/libc family) | BSD-3-Clause |
| golang.org/x/crypto (password hashing; the Let's Encrypt client, used only if you choose it), golang.org/x/net, golang.org/x/image (reading WebP pictures, and resizing) | BSD-3-Clause |
| golang.org/x/crypto (password hashing; the Let's Encrypt client, used only if you choose it), golang.org/x/net | BSD-3-Clause |
| golang.org/x/image (decoding WebP images people upload for around a game's picture) | BSD-3-Clause |
| github.com/pion/webrtc and its family (ice, dtls, srtp, sctp, rtp, rtcp, interceptor and others) | MIT |
| github.com/coder/websocket | ISC |
| github.com/bodgit/sevenzip, github.com/bodgit/plumbing, github.com/bodgit/windows (reading .7z game archives) | BSD-3-Clause |
| github.com/nwaples/rardecode (reading .rar game archives) | BSD-2-Clause |
| github.com/klauspost/compress, github.com/ulikunitz/xz, github.com/pierrec/lz4 (decompression, for the above) | BSD-3-Clause |
| github.com/andybalholm/brotli, github.com/stangelandcl/ppmd (decompression, for the above) | MIT |
| github.com/spf13/afero, go4.org/readerutil (used by github.com/bodgit/sevenzip) | Apache-2.0 |
| github.com/hashicorp/golang-lru (used by github.com/bodgit/sevenzip) | MPL-2.0. Unmodified; its source is at <https://github.com/hashicorp/golang-lru> |
| github.com/google/uuid, github.com/dustin/go-humanize, github.com/mattn/go-isatty, github.com/ncruces/go-strftime, github.com/remyoudompheng/bigfft, golang.org/x/sys, golang.org/x/exp, golang.org/x/text (transitive) | BSD / MIT |

Run `go version -m wms` on a binary for the exact module list and versions.

## The game runner (`wittyplay-host`, a separate program)

| Module | Licence |
|---|---|
| github.com/ebitengine/purego | Apache-2.0 |
| golang.org/x/sys | BSD-3-Clause |

## Game PCs

None of these is in the server's release, and no code from them is in WittyMediaServer. See `docs/GAME-RIGS.md`.

| Program | License | How it's used |
|---|---|---|
| **WittyMoonlight Host** | **GPL-3.0** | A separate program, installed separately, that `wms` starts and talks to over pipes. Built from `modules/wittymoonlighthost`, with its complete source offered beside each release |
| [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c) | GPL-3.0 | Moonlight's streaming core, inside WittyMoonlight Host |
| [ENet](https://github.com/cgutman/enet), [nanors](https://github.com/sleepybishop/nanors) | MIT | Inside moonlight-common-c |
| [Mbed TLS](https://github.com/Mbed-TLS/mbedtls) | Apache-2.0 (it's dual-licensed; this is the license chosen) | Inside WittyMoonlight Host |
| [Sunshine](https://github.com/LizardByte/Sunshine) | GPL-3.0 | Installed by whoever owns the PC. Reached over its network protocols only |
| [Moonlight's apps](https://moonlight-stream.org) | GPL-3.0 | Installed by each person on their device, as a fallback. Started as a separate program where the device allows that |

## Games

Libretro cores are **not bundled**. An administrator installs each one on request from the libretro
project's build server, and it stays a separate file, loaded only by the separate runner. Each core's
licence is shown before it is installed; they include GPL-2.0, GPL-3.0, AGPL-3.0, MPL-2.0, MIT and Zlib.
Cores with non-commercial licences are never offered. RetroArch (GPL-3.0) is not bundled, and its
cores folder is read if present. See `docs/GAMING.md` and `docs/WITTYRETROARCH.md`.

No game or BIOS files are bundled, downloaded or linked.

### Game metadata

None of this is in the repository or in a release. Each server fetches it for itself while it runs, from the sources its owner leaves on,
and each game's page credits what it shows.

| Source | Terms | Notes |
|---|---|---|
| [libretro database](https://github.com/libretro/libretro-database) | CC BY-SA 4.0 | Lists downloaded to the server's cache. Built by its contributors from No-Intro, Redump, GameTDB and others |
| [libretro thumbnails](https://github.com/libretro-thumbnails/libretro-thumbnails) | No licence stated | Pictures are their publishers'. Fetched to the owner's own server only, on by default, with its own switch |
| [Wikidata](https://www.wikidata.org) | CC0 | |
| [Wikipedia](https://www.wikipedia.org) | CC BY-SA 4.0 | Summaries, linked to their articles |
| [IGDB](https://www.igdb.com) | IGDB and Twitch terms | With the owner's own key |
| [ScreenScraper](https://www.screenscraper.fr) | CC BY-NC-SA 4.0 | With the owner's own account. For free software only |
| [RAWG](https://rawg.io) | RAWG API terms | With the owner's own key. Linked on every page showing its data |
| [TheGamesDB](https://thegamesdb.net) | No written terms | With the owner's own key |

The reader for the libretro database's files is our own, written from its published format.

## Internet radio

Station details come from [Radio Browser](https://www.radio-browser.info/), a free community database reached through
its public API, which its operators offer for free use without guarantee. Nothing from it is bundled: an administrator's
search reads it at the time. Its server software is AGPL-3.0 and is not used or included here. Station names and logos
belong to their stations.

## Web client (bundled into the binary)

| Package | Licence |
|---|---|
| React, React DOM | MIT |
| react-router-dom | MIT |
| hls.js | Apache-2.0 |
| The map's outline of the world's land (`web/public/world-land.txt`, loaded only when the photo map is opened), made from [Natural Earth](https://www.naturalearthdata.com) 1:110m land via world-atlas (ISC) | Public domain |
| pdf.js (pdfjs-dist; loaded only when a PDF is opened) | Apache-2.0 |
| Vite, TypeScript (build only) | MIT / Apache-2.0 |

## Metadata services

**The Movie Database (TMDB)** — optional. "This product uses the TMDB API but
is not endorsed or certified by TMDB." <https://www.themoviedb.org>

Audio CDs are identified with [MusicBrainz](https://musicbrainz.org), whose core data is CC0, and their covers come from the
[Cover Art Archive](https://coverartarchive.org); the pictures belong to their owners. Only a disc's table of contents is sent.

## Audio CDs

WittyCD is an original implementation with no third-party code. It contains no code from libcdio, cdparanoia,
cdrdao or any other GPL library, and its FLAC and LZMA code is its own, written from the published formats.

## DVD-Video

The DVD navigator, IFO parser, command VM and sub-picture decoder are original
implementations written from the publicly documented DVD-Video structure. No
code from libdvdnav/libdvdread or any proprietary player is included, and no
CSS decryption is performed.

## WittyMediaPlayer (`apps/wittymediaplayer`, a separate desktop app)

Its release is its own folder, with the full texts in its `licenses/` folder. It's not part of the server's
archives or Docker image.

| Component | Licence | Notes |
|---|---|---|
| **Qt 6.10** (Core, Gui, Widgets, Multimedia) | LGPL-3.0 | Shared libraries beside the program, so they can be replaced. Source: <https://download.qt.io/official_releases/qt/6.10/>. |
| **FFmpeg 8.1** (avcodec, avformat, avfilter, avutil, swresample, swscale) | LGPL-3.0-or-later | Shared libraries, built by `apps/wittymediaplayer/scripts/setup-toolchain.sh` with no GPL or non-free parts, no network code, and no `libbluray`, `libdvdread` or `libdvdnav`. Source: <https://ffmpeg.org/releases/ffmpeg-8.1.tar.xz>, with a copy to go beside each release. |
| zlib 1.3.1 (inside FFmpeg) | Zlib | <https://zlib.net> |
| LAME 3.100 (inside FFmpeg) | LGPL-2.0-or-later | Writes MP3 when copying CDs. Source goes with each release. <https://lame.sourceforge.io> |
| Opus 1.5.2, libvorbis 1.3.7, libogg 1.3.5 (inside FFmpeg) | BSD-3-Clause | Write Opus and Ogg Vorbis when copying CDs. <https://xiph.org> |
| GCC runtime libraries | GPL-3.0 with the GCC Runtime Library Exception | Allowed to ship with programs under any licence. |
| mingw-w64 winpthreads | MIT and BSD | |
| Go standard library (in the helper) | BSD-3-Clause | |

**VLC and libVLC** (VideoLAN and the VLC contributors, LGPL-2.1-or-later, <https://www.videolan.org>) are an option in its
Settings. It loads them at run time from a VLC already on the computer, and no part of VLC ships with it.

**MusicBrainz and the Cover Art Archive** supply CD names and covers, only when someone presses **Look up names**. Their data
isn't shipped.

Not shipped, used only for building or testing it: MinGW GCC, CMake, Ninja and NASM, and a general LGPL FFmpeg
that makes the tests' short clips.
