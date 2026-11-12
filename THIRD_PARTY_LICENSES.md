# Third-party notices

WittyMediaServer (AGPL-3.0-or-later, see LICENSE and NOTICE) bundles or depends on the following open-source components.

**Settings, About** lists every library in the program you're running, with its version, license and full license
text. That list is made from what's really compiled in (`internal/licenses`), and this file is the summary of it.

## Bundled in release archives / the Docker image

| Component | Licence | Notes |
|---|---|---|
| **FFmpeg** (`bin/ffmpeg`, `bin/ffprobe`) | GPL-2.0-or-later (builds include libx264) | Run as a separate process for probing, transcoding and stills. Windows/Linux builds from [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds); macOS builds from [ffmpeg.martin-riedl.de](https://ffmpeg.martin-riedl.de); FreeBSD built from source by `scripts/build-ffmpeg-static.sh`. The Docker image uses Debian's `ffmpeg` package (GPL-2.0-or-later, with x264 and x265). Source: <https://ffmpeg.org>. If you need an LGPL-only deployment, point `WMS_FFMPEG` at an LGPL build with `libopenh264` or a hardware encoder; nothing in WittyMediaServer requires GPL components. |
| libx264 (inside FFmpeg) | GPL-2.0-or-later | <https://www.videolan.org/developers/x264.html> |
| LAME, Opus (inside FFmpeg) | LGPL-2.1 / BSD-3-Clause | |
| Mesa VA-API drivers (Docker image, amd64) | MIT | AMD hardware transcoding. From Debian's `mesa-va-drivers`. <https://mesa3d.org> |
| Intel media driver, with its encoding kernels (Docker image, amd64) | MIT; the kernels are redistributable binaries without source | Intel hardware transcoding, Broadwell and newer. From Debian's `intel-media-va-driver-non-free`, in its non-free section. <https://github.com/intel/media-driver> |
| i965 VA-API driver and its shaders (Docker image, amd64) | MIT | Older Intel graphics. From Debian's `i965-va-driver-shaders`. <https://github.com/intel/intel-vaapi-driver> |
| Intel oneVPL GPU runtime (Docker image, amd64) | MIT | Quick Sync. From Debian's `libmfx-gen1.2`. <https://github.com/intel/vpl-gpu-rt> |
| vainfo (Docker image, amd64) | MIT | For checking the graphics by hand. <https://github.com/intel/libva-utils> |

## Connected to, never included

| Component | Licence | Notes |
|---|---|---|
| **Tdarr** (server and nodes) | Proprietary: [Tdarr's EULA](https://github.com/HaveAGitGat/Tdarr/blob/master/LICENSE.md) | Not bundled, downloaded or run by this project. Library transcoding can connect to a copy someone installs themselves, over its HTTP API, if they turn that on. Whoever installs it agrees to its terms. No Tdarr code or plugin (its plugins are GPL-3.0) is included. See [docs/LIBRARY-TRANSCODING.md](docs/LIBRARY-TRANSCODING.md). |

## Fonts

| Component | Licence | Notes |
|---|---|---|
| **Inter** 4.1 (`web/public/fonts`, `internal/brandart/fonts`) | SIL Open Font License 1.1 (`web/public/fonts/Inter-OFL.txt`) | The web client's font, served by the server, and the outlines `scripts/genicon` draws the brand name and other words in pictures with. Copyright (c) 2016 The Inter Project Authors. <https://github.com/rsms/inter> |

## Go modules (statically linked)

| Module | Licence |
|---|---|
| modernc.org/sqlite (and its modernc.org/libc family) | BSD-3-Clause |
| golang.org/x/crypto (password hashing; the Let's Encrypt client, used only if you choose it), golang.org/x/net, golang.org/x/image (reading WebP pictures, and resizing) | BSD-3-Clause |
| golang.org/x/crypto (password hashing; the Let's Encrypt client, used only if you choose it), golang.org/x/net | BSD-3-Clause |
| golang.org/x/image (decoding WebP images people upload for around a game's picture) | BSD-3-Clause |
| github.com/pion/webrtc and its family (ice, dtls, srtp, sctp, rtp, rtcp, stun, turn, interceptor and others). Compiled in because go-libp2p names its WebRTC transport, which this server doesn't turn on | MIT |
| github.com/coder/websocket | ISC |
| github.com/bodgit/sevenzip, github.com/bodgit/plumbing, github.com/bodgit/windows (reading .7z game archives) | BSD-3-Clause |
| github.com/nwaples/rardecode (reading .rar game archives) | BSD-2-Clause |
| github.com/klauspost/compress, github.com/ulikunitz/xz, github.com/pierrec/lz4 (decompression, for the above) | BSD-3-Clause |
| github.com/andybalholm/brotli, github.com/stangelandcl/ppmd (decompression, for the above) | MIT |
| github.com/spf13/afero, go4.org/readerutil (used by github.com/bodgit/sevenzip) | Apache-2.0 |
| github.com/hashicorp/golang-lru (used by github.com/bodgit/sevenzip) | MPL-2.0. Unmodified; its source is at <https://github.com/hashicorp/golang-lru> |
| github.com/google/uuid, github.com/dustin/go-humanize, github.com/mattn/go-isatty, github.com/ncruces/go-strftime, github.com/remyoudompheng/bigfft, golang.org/x/sys, golang.org/x/exp, golang.org/x/text (reading the text encodings of books and subtitles) | BSD / MIT |
| **github.com/libp2p/go-libp2p**, go-libp2p-kad-dht, go-libp2p-kbucket, go-libp2p-record, go-libp2p-routing-helpers, go-libp2p-asn-util, go-msgio, go-cidranger, go-flow-metrics (peer-to-peer connections: the node itself, hole punching, relays, the public address book) | MIT. Copyright (c) 2014 Juan Batiz-Benet and the libp2p contributors |
| github.com/libp2p/go-yamux (several streams over one connection) | MPL-2.0 and BSD-3-Clause. Unmodified; its source is at <https://github.com/libp2p/go-yamux> |
| github.com/libp2p/go-buffer-pool | MIT and BSD-3-Clause |
| github.com/libp2p/go-netroute, github.com/google/gopacket (finding which network card leads where) | BSD-3-Clause |
| github.com/libp2p/go-reuseport | ISC |
| github.com/quic-go/quic-go, qpack, webtransport-go (QUIC, the transport peer-to-peer connections prefer) | MIT |
| github.com/flynn/noise (the Noise handshake, for peer-to-peer connections over TCP) | BSD-3-Clause |
| github.com/multiformats/go-multiaddr, go-multiaddr-dns, go-multiaddr-fmt, go-multibase, go-multihash, go-multistream, go-varint, github.com/mr-tron/base58, github.com/ipfs/go-cid, go-datastore, go-log, github.com/ipld/go-ipld-prime, github.com/polydawn/refmt, github.com/jbenet/go-temp-err-catcher, github.com/whyrusleeping/go-keyspace, github.com/libp2p/go-libp2p's other small parts (addresses, names and records as the IPFS network writes them) | MIT |
| github.com/ipfs/boxo, github.com/multiformats/go-multicodec, github.com/multiformats/go-base36 | Apache-2.0 or MIT, at your choice |
| github.com/multiformats/go-base32, github.com/spaolacci/murmur3, github.com/pbnjay/memory, github.com/munnerz/goautoneg, github.com/dunglas/httpsfv, filippo.io/bigmod, gonum.org/v1/gonum, google.golang.org/protobuf, golang.org/x/sync, golang.org/x/time | BSD-3-Clause |
| github.com/gorilla/websocket, github.com/huin/goupnp, github.com/marten-seemann/tcp, github.com/mikioh/tcpinfo, github.com/mikioh/tcpopt | BSD-2-Clause |
| github.com/decred/dcrd/dcrec/secp256k1, filippo.io/keygen (key types other nodes on the network use) | ISC |
| github.com/minio/sha256-simd, github.com/jackpal/go-nat-pmp, github.com/prometheus/client_golang, client_model, common and procfs, github.com/go-logr/logr and stdr, go.opentelemetry.io/auto/sdk (pulled in by go-libp2p; no metrics are collected or sent) | Apache-2.0. The Prometheus libraries' NOTICE files are kept with their license texts |
| go.opentelemetry.io/otel, with its metric and trace packages (pulled in by go-libp2p; nothing is traced or sent) | Apache-2.0 and BSD-3-Clause |
| github.com/hashicorp/golang-lru v1 (used by go-libp2p's dependencies) | MPL-2.0. Unmodified; its source is at <https://github.com/hashicorp/golang-lru> |
| go.uber.org/fx, dig, multierr and zap, github.com/benbjohnson/clock, github.com/filecoin-project/go-clock, github.com/beorn7/perks, github.com/cespare/xxhash, github.com/davidlazar/go-crypto, github.com/klauspost/cpuid, github.com/koron/go-ssdp, lukechampine.com/blake3 (used by go-libp2p) | MIT |
| github.com/wlynxg/anet (used by pion) | BSD-3-Clause |

Run `go version -m wms` on a binary for the exact module list and versions.

## Peer-to-peer connections

Peer-to-peer connections ([docs/P2P.md](docs/P2P.md)) are built on **go-libp2p** and its family, listed above. All of it
is under permissive licenses (MIT, BSD, ISC, Apache-2.0) or the MPL-2.0, which the AGPL-3.0-or-later and the client apps'
GPL-3.0-or-later can both be combined with. Nothing in it was changed. One test helper, `simulated` in
`internal/p2p/holepunch_test.go`, is adapted from go-libp2p's own tests and says so in its header, with go-libp2p's
copyright line; the tests also use github.com/marcopolo/simnet (BSD-3-Clause), which no release contains.

`wittyconnect`, the helper program the desktop app starts, and the library the phone and TV apps link, are built from
this repository (`cmd/wittyconnect`, `pkg/wittyconnect`) and ship with WittyMediaClient, not with the server.
`go run ./internal/licenses/gen -notices ./cmd/wittyconnect <file>` writes the notices file that goes with them:
every library in it, with each license's full text.

When turned on, the feature talks to other people's computers: the public IPFS network's bootstrap nodes
(`bootstrap.libp2p.io`), its address book, and relays. They are not part of this software and nothing of theirs is
bundled. [docs/REMOTE.md](docs/REMOTE.md#peer-to-peer-connections) says what they see.

## Modules

No module's code is in the server. Each one is passed on by itself, and carries the licenses of the libraries in its
programs in `THIRD-PARTY-NOTICES.txt`, which the server shows at `GET /api/modules/{id}/notices`.

## Game PCs

None of these is in the server's release, and no code from them is in WittyMediaServer. See `docs/GAME-RIGS.md`.

| Program | License | How it's used |
|---|---|---|
| **WittyCouch Host** | **AGPL-3.0-or-later**, with Moonlight-derived parts under **GPL-3.0** | A separate program, installed separately, that the server starts and talks to over pipes. Built from the WittyCouchHost repository, with its complete source offered beside each release |
| [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c) | GPL-3.0 | Moonlight's streaming core, inside WittyCouch Host |
| [ENet](https://github.com/cgutman/enet), [nanors](https://github.com/sleepybishop/nanors) | MIT | Inside moonlight-common-c |
| [Mbed TLS](https://github.com/Mbed-TLS/mbedtls) | Apache-2.0 (it's dual-licensed; this is the license chosen) | Inside WittyCouch Host |
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
| Some interface icons in `web/src/icons.tsx` follow the shapes of Material Icons | Apache-2.0 |
| The map's outline of the world's land (`web/public/world-land.txt`, loaded only when the photo map is opened), made from [Natural Earth](https://www.naturalearthdata.com) 1:110m land via world-atlas (ISC) | Public domain |
| pdf.js (pdfjs-dist; loaded only when a PDF is opened) | Apache-2.0 |
| pdf.js's image decoders and colour profile, fetched only when a PDF needs them: OpenJPEG (BSD-2-Clause), PDFium's JBIG2 (BSD-3-Clause), qcms (MIT), CGATS001Compat ICC profile (CC0) | As listed |
| Vite, Rolldown, TypeScript (build only) | MIT / Apache-2.0 |

## Metadata services

**The Movie Database (TMDB)** — optional. "This product uses the TMDB API but
is not endorsed or certified by TMDB." <https://www.themoviedb.org>

Audio CDs are identified with [MusicBrainz](https://musicbrainz.org), whose core data is CC0, and their covers come from the
[Cover Art Archive](https://coverartarchive.org); the pictures belong to their owners. Only a disc's table of contents is sent.

## Audio CDs

WittyCD is an original implementation with no third-party code. It contains no code from libcdio, cdparanoia,
cdrdao or any other GPL library, and its FLAC and LZMA code is its own, written from the published formats.
**TheTVDB** — optional. Metadata provided by TheTVDB. <https://thetvdb.com>

**TVmaze** — on by default. CC BY-SA 4.0. <https://www.tvmaze.com>

**AniList** — for Anime libraries. <https://anilist.co>

**Free Dictionary API** — when someone presses Define in a reader. It publishes no terms.
<https://dictionaryapi.dev>

**Apple Podcasts directory** — when an administrator searches for a podcast.

None of these is associated with, or endorses, WittyMediaServer. Their terms apply to their data.
*Settings → Legal → Privacy* in the app says what each one receives.

## DVD-Video

The DVD navigator, IFO parser, command VM and sub-picture decoder are original
implementations written from the publicly documented DVD-Video structure. No
code from libdvdnav/libdvdread or any proprietary player is included, and no
CSS decryption is performed.

## WittyMediaPlayer (a separate desktop app, in its own repository)

Its release is its own folder, with the full texts in its `licenses/` folder. It's not part of the server's
archives or Docker image.

| Component | Licence | Notes |
|---|---|---|
| **Qt 6.10** (Core, Gui, Widgets, Multimedia) | LGPL-3.0 | Shared libraries beside the program, so they can be replaced. Source: <https://download.qt.io/official_releases/qt/6.10/>. |
| **FFmpeg 8.1** (avcodec, avformat, avfilter, avutil, swresample, swscale) | LGPL-3.0-or-later | Shared libraries, built by its `scripts/setup-toolchain.sh` with no GPL or non-free parts, no network code, and no `libbluray`, `libdvdread` or `libdvdnav`. Source: <https://ffmpeg.org/releases/ffmpeg-8.1.tar.xz>, with a copy to go beside each release. |
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

**LRCLIB** (<https://lrclib.net>, its server is MIT licensed) supplies timed lyrics, and **Musixmatch** does with your own key, only if the owner
turns online lyrics on. **Wikipedia** and **Wikimedia Commons** supply artist biographies (CC BY-SA 4.0, credited on the artist's page) and photos
(each with its own licence, linked from its article), only if the owner turns online music information on. None of it is shipped, and lyrics belong to their
writers.

Not shipped, used only for building or testing it: MinGW GCC, CMake, Ninja and NASM, and a general LGPL FFmpeg
that makes the tests' short clips.

## Optional programs, not included

These aren't part of this server or its packages. If one is installed on the same machine, the server uses it.

| Program | License | What for |
|---|---|---|
| DjVuLibre (`ddjvu`, `djvused`) | GPL-2.0 | Drawing the pages of DjVu books. Without it they're listed and can be downloaded. |
