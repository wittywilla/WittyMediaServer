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
| github.com/google/uuid, github.com/dustin/go-humanize, github.com/mattn/go-isatty, github.com/ncruces/go-strftime, github.com/remyoudompheng/bigfft, golang.org/x/sys, golang.org/x/exp, golang.org/x/text (transitive) | BSD / MIT |

Run `go version -m wms` on a binary for the exact module list and versions.

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
