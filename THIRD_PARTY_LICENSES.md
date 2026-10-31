# Third-party notices

WittyMediaServer (Apache-2.0, see LICENSE and NOTICE) bundles or depends on the following open-source components.

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
| golang.org/x/crypto (password hashing; the Let's Encrypt client, used only if you choose it), golang.org/x/net | BSD-3-Clause |
| github.com/pion/webrtc and its family (ice, dtls, srtp, sctp, rtp, rtcp, interceptor and others) | MIT |
| github.com/coder/websocket | ISC |
| github.com/google/uuid, github.com/dustin/go-humanize, github.com/mattn/go-isatty, github.com/ncruces/go-strftime, github.com/remyoudompheng/bigfft, golang.org/x/sys, golang.org/x/exp, golang.org/x/text (transitive) | BSD / MIT |

Run `go version -m wms` on a binary for the exact module list and versions.

## The game runner (`wittyplay-host`, a separate program)

| Module | Licence |
|---|---|
| github.com/ebitengine/purego | Apache-2.0 |
| golang.org/x/sys | BSD-3-Clause |

## Games

Libretro cores are **not bundled**. An administrator installs each one on request from the libretro
project's build server, and it stays a separate file, loaded only by the separate runner. Each core's
licence is shown before it is installed; they include GPL-2.0, GPL-3.0, AGPL-3.0, MPL-2.0, MIT and Zlib.
Cores with non-commercial licences are never offered. RetroArch (GPL-3.0) is not bundled, and its
cores folder is read if present. See `docs/GAMING.md` and `docs/WITTYRETROARCH.md`.

Game metadata comes from Wikidata (CC0) and Wikipedia (CC BY-SA 4.0). Libretro's thumbnail collections
state no licence, so they are not used. No game or BIOS files are bundled, downloaded or linked.

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
| GCC runtime libraries | GPL-3.0 with the GCC Runtime Library Exception | Allowed to ship with programs under any licence. |
| mingw-w64 winpthreads | MIT and BSD | |
| Go standard library (in the helper) | BSD-3-Clause | |

**VLC and libVLC** (VideoLAN and the VLC contributors, LGPL-2.1-or-later, <https://www.videolan.org>) are an option in its
Settings. It loads them at run time from a VLC already on the computer, and no part of VLC ships with it.

Not shipped, used only for building or testing it: MinGW GCC, CMake, Ninja and NASM, and a general LGPL FFmpeg
that makes the tests' short clips.
