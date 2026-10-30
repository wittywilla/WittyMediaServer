# Roadmap

What is still to do, from the project's own requirements, in the planned order. Done work is
listed at the end so the whole picture is in one place. Names follow [MODULES.md](MODULES.md).

## 1. Disc engines and the player family

- [x] **WittyDVD as its own Go module** (`modules/wittydvd`, mirrored to its repository), with the
  `wittydvd` command-line tool.
- [x] **WittyBluray engine** (`modules/wittybluray`, mirrored to its repository, with the
  `wittybluray` tool and [BLURAY.md](BLURAY.md)): images and folders, menus, titles, chapters,
  audio, subtitles, resume, AACS/BD-J/3D detection, 4K and HDR flags with tone mapping.
  Pop-up menus during the film, animated highlights and 3D viewing modes (side-by-side /
  over-under to 2D or anaglyph) are in, and so are BD-J menus with a Java the server's owner
  installs. [BLURAY-QUIRKS.md](BLURAY-QUIRKS.md) lists every format feature and its state. Still to
  do: test on retail discs (BD-J, MVC 3D, UHD HDR); secondary audio and picture-in-picture; MVC 3D
  decoding.
- [x] **WittyDisc** (`modules/wittydisc`): the disc image reader both engines share.
- [x] **Versioned module releases:** WittyDisc, WittyDVD and WittyBluray are tagged on their
  repositories, so `go get` and `go install` work outside a WittyMediaServer checkout
  (MODULES.md, *Releasing the Go modules*).
- [ ] **DVD playback sometimes stuck on the first warning screen** (Shrek 2): confirm fixed, or
  fix.
- [ ] **Server plugin interface** (`Module` in MODULES.md): books, discs and audio first.
- [ ] **WittyMediaPlayer:** a desktop player for local files, folders, images and discs, built
  on the server's engines, with WittyDVD and WittyBluray as plugins. Own repository.
- [ ] **Diagnostics:** the DVD and Blu-ray tools collect everything useful for diagnosing a disc,
  with no copyrighted material in what they generate. A small GUI for Windows and macOS that
  makes the diagnostic file. Clear program names on every platform.

## 2. Libraries still to build

- [x] **WittyPhotos** ([PHOTOS.md](PHOTOS.md)): timeline, albums, favourites, folders, map, sharing and
  uploads, with EXIF and RAW previews. Still to do: people and places, when the local AI exists (the
  placeholder is in); batch rotate and edit.
- [x] **WittyShorts** ([SHORTS.md](SHORTS.md)): vertical videos with likes, comments, sharing, creators,
  custom metadata and a For You page from local data alone.
- [x] **WittyChannels** ([CHANNELS.md](CHANNELS.md)): creator videos with a web-video-style interface, for
  collections already on disk.
- [x] **Music videos and concerts** ([CHANNELS.md](CHANNELS.md#music-videos-and-concerts)), **internet radio**,
  **collections and smart playlists**, **Your year** and **recommendations** ([COLLECTIONS.md](COLLECTIONS.md)).
- [ ] **Watch together:** a shared, synchronised player across profiles and devices. Best built with
  remote access in place, so it can be tested across real networks.
- [ ] **WittyTV (Live TV and DVR):**
  - channels made from the libraries: by network, by show type, custom; DVDs can play on a
    channel (without remote features); server-managed and the same for every user;
  - an early-2000s cable/satellite guide interface;
  - IPTV import, HDHomeRun and other tuners;
  - DVR, disabled for library channels (with a disclaimer); recordings in their own folder by
    default, with an admin toggle to put them in the show's folder;
  - guide metadata, with any key requirements in the setup wizard;
  - per-user access toggles for Live TV, library channels and tuner channels.
- [x] **Playback speed on every media type:** up to 4×, higher by a server setting with a
  warning about hardware and preferred codecs; pitch preserved. Video, discs, music, audiobooks,
  podcasts and vertical videos; each kind of listening remembers its own speed.

## 3. Remote access and clients

- [x] **Remote access:** [REMOTE.md](REMOTE.md). *Settings → Remote Access* with a clear status and
  guided steps for Tailscale, NetBird, port forwarding and a VPS; a test with your own phone; HTTPS
  with Let's Encrypt or your own certificate; optional port mapping; limits on streams away from
  home; a server address in the clients; a greyed-out hosted relay marked as a future paid service.
- [ ] **Hosted relay:** the future paid service shown greyed out in Settings. Not built.
- [ ] **WittyMediaRemoteMedia:** the basics of a NAS agent (UGREEN, Synology, Docker) that
  shares folders with a server. Own repository.
- [x] **The server's side of the clients:** [the client API](CLIENT-API.md) with versioning and
  capabilities, playback profiles for Apple TV and Android TV (direct, remux or transcode), stream
  tokens, TV sign-in with a code, themes as data, and D-pad navigation on every page. Not yet tried
  on real TVs.
- [ ] **WittyMediaClient** (own repository): a themed, frameless desktop client (no File/View
  menus; fully themeable; Windows 7 and up), Android TV (and a server build for Android TV /
  NVIDIA Shield), Apple TV with the current themes; TV remotes, every kind of game controller,
  screen readers and accessible navigation everywhere.
- [ ] **WittyReader** desktop builds in their own window.

## 4. Packaging and setup

- [ ] **Windows installers:** server, client and combined; silent/headless install; the portable
  version as an option.
- [ ] **Linux:** native amd64 and arm64 packages for Fedora, Ubuntu and Arch, plus Flatpak and
  Snap and other popular distributions; portable builds.
- [ ] **Minimum hardware specification** document (realistic; Windows 10 for the server,
  Windows 7 for the desktop client).
- [ ] **Local AI:** a future-plan document, with image recognition for photos as the first use. The clearly
  marked placeholders are in: People & places in WittyPhotos, and Settings, Sharing & photos (never in the
  setup wizard), and `GET /api/photos/ai` answers that it isn't available.

## 5. Documentation and quality

- [ ] **New screenshots** at 1920×1080 for the README (the layout, separate guides and download
  link are done; the pictures still show the earlier interface).
- [ ] **Publish the GitHub wikis** of WittyInk, WittyEbook, WittyPDF, WittyComics and WittyReader
  (the pages are written in each `wiki/` folder; each wiki needs its first page saved on
  github.com before `scripts/mirror.sh` can push).
- [ ] **Continuous audit:** performance and security first, built for a wide range of hardware;
  trim unneeded code. Known items: the main web bundle is 1.1 MB and should be split by page;
  the next-up query runs one query per show.

## Done

- Video libraries, TV shows, anime (AniList, absolute numbering), metadata without keys (TVmaze,
  Wikidata), metadata keys in the setup wizard.
- WittyDVD: menus, the resizable remote with tooltips and the seek-bar option, the insert
  animation with its own sound toggle, angles, subtitles, speed with pitch preserved.
- Music and audiobooks, podcasts, books, comics and manga; WittyEbook, WittyPDF, WittyComics and
  WittyInk as plugins; WittyReader; the five repositories with READMEs, licences and attribution.
- Profiles: Kids and Guest with content limits and admin-only settings; per-user library access.
- Mobile-first shell (side, top or both navigation; home widgets), TV and controller navigation,
  accessibility.
- Config file (portable mode), custom metadata location, opt-in logging with categories, a
  custom log folder and an open-folder link.
- First run: default browser opens, native folder picker, a seven-step wizard (media, metadata
  agents, discs, look) that says plainly the server is for the home network until remote access is set up.
- Settings redesign (grouped pages, section cards, a save bar); custom metadata agents.
- Security audit: cross-site protection, artwork and feed file-read and SSRF fixes, sandboxed book
  files, fuzz-tested disc parsers, a hardened desktop client.
- READMEs for every repository in one layout, with installing, reference and troubleshooting guides.
