# Changelog

All notable changes to WittyMediaServer. Versions follow [semantic versioning](https://semver.org);
`-alpha` releases may change settings and APIs between versions.

## 0.2.0-alpha

### WittyDVD (the DVD engine, formerly `internal/disc/dvd`)

- Renamed the DVD engine to **WittyDVD** (`internal/wittydvd`).
- **Interactive titles**: buttons inside titles (games, quizzes, voting such as Shrek 2's
  *Far Far Away Idol*) now work; such titles play cell by cell with their buttons and timers.
- **Insert DVD** button with an optional disc-tray animation and synthesized drive sound
  (per disc and server-wide), and a per-disc start mode (menu or main feature).
- **On-screen DVD remote** (top menu, menu, d-pad, return, audio, subtitle, chapter, transport
  and number keys); movable, resizable and translucent; also available for regular videos with
  disc-only keys greyed out.
- **Audio** tracks map through the disc's audio control table (the right language plays), are
  labelled with their purpose (*Commentary*, …), and switching keeps your position.
- **Subtitles** are rendered from the disc's own sub-pictures in the player: instant switching,
  forced subtitles, purpose labels.
- **Menu** and **Resume** remember the exact frame; *Return* key; *Play movie* works on discs
  that guard their feature.
- Fixes: menus that are a single still frame no longer stall; a still first-play no longer
  hangs; highlights parsed for "same as previous" NAV packs; forced-select/forced-action buttons;
  navigation timer; no black flashes or highlight flicker between menu clips.
- Faster on small machines: cached menu scans and decoded commands, chunked disc reads, shared
  file handles (fixes a handle leak), next-clip prefetch, low-power mode.
- New `wms dvd crawl` command presses every button of every menu.

### TV shows

- New **TV shows** library type: show → season → episode, multi-episode files, specials.
- **TheTVDB** metadata: overview, network, genres, status, content rating, posters, season
  posters, episode titles/overviews/air dates/stills; manual matching by TheTVDB id.
- Show pages with **Cards** or **Playlist** layout (per show + server default), **Next Up**,
  **Skip intro** (chapters or per-show range), **Watch next**, recommendations.
- Networks/genres API (`/api/shows/networks`, `?network=`, `?genre=`) for future TV channels.

### Library and metadata

- Metadata and artwork are **saved next to media** in Kodi/Jellyfin/Emby/Plex layouts
  (`.nfo`, `poster.jpg`, `fanart.jpg`, …), with backups of hand-written NFOs.
- **Artwork picker** (local files, TMDB, TheTVDB) for movies, discs, episodes and shows.
- Movie recommendations at the end of a film (sequels first).

### Transcoding

- **Settings → Transcoding**: direct-play toggle; one checkbox per hardware encoder (NVENC,
  Quick Sync, VideoToolbox, AMF, VAAPI), each test-encoded at start-up and greyed out when
  missing; **Easy / Advanced / Expert** quality modes with tooltips;
  [transcoding guide](docs/TRANSCODING.md) with GPU/CPU capability tables.
- VAAPI and Quick Sync now upload frames correctly (nv12 + hwupload).

### Interface

- **Themes**: 20 built-in themes (dark, light, OLED and colour variants), custom themes as ZIPs
  in the `themes` folder, per-device or server-wide; [theme guide](docs/THEMES.md).
- Custom server name in the top-left corner.
- Player controls auto-hide outside fullscreen too.
- Version shown at the bottom of the sidebar and the sign-in page.

### Builds and releases

- Release archives for **10 platforms**: Windows x64/ARM64, macOS Intel/Apple Silicon, Linux
  x64/ARM64/ARMv7/x86, FreeBSD x64/ARM64, plus a SHA256 checksum file; Docker images for
  amd64, arm64 and armv7.
- Fixed the FreeBSD FFmpeg build (x264's configure needs bash) and moved it to FreeBSD 14.4;
  a failed FreeBSD FFmpeg build no longer blocks the release.
- macOS FFmpeg downloads fall back to snapshot builds when release links are missing.
- Archives built on Windows now keep the executable permission for Linux/macOS/FreeBSD files.
- CI now runs on `master`; GitHub Actions updated to their Node 24 versions.

### Documentation

- New guides: [Getting started](docs/GETTING_STARTED.md), [DVDs](docs/DVD.md),
  [TV shows](docs/TV.md), [Metadata](docs/METADATA.md), [Transcoding](docs/TRANSCODING.md),
  [Themes](docs/THEMES.md), [Contributing](CONTRIBUTING.md), [Publishing](docs/PUBLISHING.md).

## 0.1.0-alpha

- First public build: Go media server with native DVD-Video menu navigation, libraries,
  metadata, HLS transcoding with instant seeking, users, tray launcher, installers, Docker and
  Kubernetes manifests.
