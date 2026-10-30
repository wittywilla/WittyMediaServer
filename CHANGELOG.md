# Changelog

All notable changes to WittyMediaServer. Versions follow [semantic versioning](https://semver.org);
`-alpha` releases may change settings and APIs between versions.

## 0.3.0-alpha

### Home

- **A home page that makes sense at a glance**: the featured carousel, then *Your libraries* as
  tiles (with *Add a library* for admins), one **Jump back in** row for everything you are
  watching, reading or listening to, *Next up*, what's new in each library, and *Can't decide?*
  Saved layouts start fresh once; sections you hid stay hidden.
- **First run**: with no libraries yet, home welcomes you and adds one right there, instead of an
  empty page.
- **Offline**: when the server can't be reached, home says so plainly and reconnects by itself,
  instead of showing "Failed to fetch".

### DVDs and Blu-rays

- **A DVDs & Blu-rays library type**, for a shelf of disc images, films and TV series together.
  Discs still work in Movies and TV shows libraries; the descriptions explain the choice.
- **Film or TV series**: each disc is sorted from its name (season, series, box set…) or its
  library, and you can set it yourself in *Disc settings*. Libraries holding discs get
  *Films* / *TV series* (or *DVDs & Blu-rays*) filters; TV libraries list their discs below the
  shows. Discs of a series are no longer looked up as films.
- Blu-rays are offered by *Can't decide?* again.

### Podcasts

- Adding a podcasts library explains what it holds (shows you follow, downloads, your own files)
  and goes straight on to **following shows**: search the free Apple Podcasts directory, or paste
  any RSS feed. The folder is optional: without one, episodes stream.
- *Keep downloaded* now reads "newest 5 episodes"; an empty podcasts library shows the same
  search instead of a bare message.

### Settings

- **Version and modules**, in *About & legal* and at the foot of the settings menu: this server's
  version and those of WittyDVD, WittyBluray, WittyDisc and the readers inside it.


### Disc engines as Go modules

- **WittyDisc, WittyDVD and WittyBluray v0.1.0**: tagged on their own repositories, so
  `go install github.com/wittywilla/wittydvd/cmd/wittydvd@latest` (or `…/wittybluray/cmd/wittybluray`)
  works without a WittyMediaServer checkout. In this repository a `go.work` builds them from
  local source; `scripts/mirror.sh <Repository> <vX.Y.Z>` publishes and tags a release.

### WittyDVD

- **Angles work**: interleaved multi-angle blocks are read per angle, so intros no longer flicker
  between language versions (Shrek). Pick a *Video version* while watching; playback continues
  from the same moment.
- **Disc memory**: the audio language, subtitles and angle you pick are remembered per disc and
  put back next time, like a DVD player's last-memory feature.
- Fixed features that were treated as menus (no seek bar, no audio/subtitle choice).
- Fixed playback stalling a few seconds into some titles: streams are now matched by their DVD
  stream id, so cells that list their streams in a different order keep their sound.
- Damaged audio frames no longer stop a transcode (which used to leave the player stuck on a
  warning screen); a watchdog also moves on when a clip stalls right before its end.
- No black flash after pressing a menu button.

### Disc page

- **Feature Presentation** first, with a thumbnail for every scene, then **Bonus Features** and
  shortcuts straight into the **Disc Menus**; the full title list is folded away underneath.
- Admins can name the feature, scenes and bonus features; the names show in the player too.
- Thumbnails that cannot be made show "Preview not available" after a few attempts.

### Player

- Big TV-style tile pickers for **Audio**, **Subtitles** and **Video version** (angles).
- **Viewing settings** (gear): seek bar, chapter marks, remote, remote style, hide delay,
  fit/fill picture, subtitle size — for this disc/video or for everything.
- **Playback speed** from 0.5× to 4× on every kind of media with the pitch of voices kept;
  admins can allow up to 16× in *Settings → Server* (with a hardware warning).
- **Mini player**: *Back* keeps the video playing in a corner window (a bottom bar on phones)
  with play/pause, chapter skip, disc Menu/Title buttons, expand and stop.
- The remote: labelled **Remote** button in the control bar, resizable from every edge and
  corner, see-through tooltips (*Eject the DVD*, *Close the Remote*), and an *In control bar*
  style that puts its buttons in a strip above the seek bar.

### First run and sign-in

- **Setup wizard**: welcome → administrator account → libraries → posters & info → done, with a
  step indicator. Libraries are added by kind (Movies, TV Shows, Home videos).
- **System folder picker**: on the server's own screen, *Choose folder…* opens the native
  dialog (the modern Explorer window on Windows, Finder on macOS, the desktop's dialog on
  Linux); other devices browse the server's folders in the page.
- **Metadata keys in setup**: step-by-step instructions for TMDB and TheTVDB with a *Test*
  button. TMDB accepts either the API key or the API Read Access Token.
- The browser opens the setup page by itself on first start (not on servers, containers or
  with `WMS_NO_BROWSER=1`).
- **"Who's watching?"** profile screen; *Settings → Users → Sign-in screen* switches back to
  the classic form. Profiles are only listed on the home network unless allowed for remote
  devices.
- Sign-in is rate-limited: after 5 wrong passwords for a name, each try waits longer (up to
  15 minutes). First-run account creation cannot be raced.

### Books, comics and manga

- New **Books** library type for EPUB, PDF and CBZ (CBR/CB7/MOBI/AZW3 are listed and can be
  downloaded). Files sharing a name are one book read from the best format; digital-library
  extras (OCR text, scans, DRM-locked copies, metadata databases) are ignored, while their
  `_meta.xml` and thumbnail are used.
- Metadata from the EPUB package (title, author, series, description, cover), `ComicInfo.xml`
  (series, number, writer, summary, manga reading direction, age rating) or the file name
  (`Series 145 (2023) (Digital)` → series, number, year). Folders named "Manga" read right to left.
- **Reader**: comics page by page, as two-page spreads or a vertical strip, left-to-right or
  right-to-left, with tap zones, swipes, keys and preloading; EPUBs paged or scrolling with
  font size and light/sepia/dark pages and the contents; PDFs in the browser's viewer.
  Progress is saved per account.
- Security: book HTML is served with a policy that blocks scripts, forms and outside requests,
  and is shown in a sandboxed frame.

### Podcasts

- New **Podcasts** library type: follow shows by searching the Apple Podcasts directory or by
  pasting a feed address; feeds are checked every 2 hours.
- Episodes play through the server (private; old http:// feeds work; seeking supported) and can
  be **downloaded** into the library folder, by hand or automatically (keep newest N).
- Episodes resume where you stopped; explicit episodes are hidden from Kids and Guest profiles.
- Security: feeds and episodes may only be fetched from the internet — never from the server
  itself or the local network.

### Music and audiobooks

- New **Music** and **Audiobooks** library types: tags (artist, album, album artist, track,
  disc, year, genre), folder or embedded covers (the largest image wins), folder-name fallbacks.
  Nothing is written into music folders.
- Albums / Artists / Songs views (Books / Authors for audiobooks), album and book pages with
  Play, Shuffle, Add to queue and Continue.
- **Audio player bar** that keeps playing while browsing, full *Now playing* view with the
  queue, shuffle and repeat; audiobooks resume on every device, speed up to 3× with pitch kept,
  ±15/30 s, sleep timer (incl. end of chapter); lock-screen and media-key controls.
- MP3/AAC/FLAC/Opus/Vorbis/WAV play directly; other formats (ALAC, WMA, APE…) are converted to
  AAC while playing, with seeking.
- Audit: every FFmpeg/ffprobe process now starts without a console window on Windows (one
  shared helper replaces two copies).

### Anime

- New **Anime** library type with **AniList** information (no key): titles in English, rōmaji
  or Japanese, synopsis, studio, score, cover, banner, episode names; adult titles are rated 18.
- Anime in TV libraries (genre *Anime*) also shows in the Anime library; a setting can show it
  **only** there. An Anime library may have no folders and just collect anime from TV.
- **Absolute episode numbers** (`[Group] Show - 012 [1080p].mkv`, `Show - 05v2`) are parsed;
  release tags ignored, years never mistaken for episodes.

### Layout and home page

- **Navigation where you want it**: side menu, top bar, or both (per device, *Settings →
  Appearance → Layout*). Phones always get a bottom tab bar (Home, Search, Libraries, You) with
  room for the mini player above it.
- **Home page widgets**: *Customize* to drag sections into any order (or use the arrow buttons
  on touch screens, TV remotes and keyboards), hide and show them, or reset. The layout is saved
  to your profile and follows you to every device.
- New widgets: **Your libraries** (big shortcut tiles) and **Shuffle** ("Can't decide?" — a
  random movie or disc with Play and Another).
- Account menu in the corner: Appearance, Settings, **Switch profile**, Sign out.
- Larger touch targets on touch screens; the interface scales up on 4K screens and TVs.

### Metadata without keys

- **TV shows work out of the box** with **TVmaze** (no key): titles, summaries, networks,
  status, posters, backdrops, season posters and episode names. TheTVDB is used instead when a
  key is set; shows switch over on their next scan.
- **Movies get facts without a key** from **Wikidata** and Wikipedia: year, genres, US rating
  and a plot summary (posters still need TMDB). *Refresh* re-reads a title.
- Each title credits its source; both can be switched off in *Settings → Server*.
- **Content rating** can be set by hand on movies and shows (for parental controls).
- The setup wizard shows what is built in before asking for optional keys.

### Profiles and parental controls

- **Kids** and **Guest** profiles next to adult accounts. Kids and Guest profiles may have no
  password: they then sign in with one tap, but only on the home network.
- **Limits per account**: an age level (worked out from US, UK, German and TV ratings), titles
  without a rating on or off, allowed libraries, hidden genres, and *always allow* / *never
  allow* exceptions for single movies and shows. Live TV switches (all Live TV, channels made
  from your libraries, tuner/IPTV channels) are stored for the coming Live TV feature.
- Limits are enforced by the server everywhere (lists, home, search, show pages, playback,
  artwork, recommendations); hidden titles answer "not found". Unrestricted accounts pay no
  extra cost; unreadable limits fail closed.
- Adults may add and manage Kids profiles unless the admin keeps that to themself; Guest and
  adult accounts are admin-only. The setup wizard offers to add a Kids profile.
- The Insert DVD sound can be switched off separately from the animation.

### Config file, portable mode and logs

- **`wms.json` config file** for launch settings (port, folders, FFmpeg, transcode limit, log
  level), found next to the program, in the user config folder, or via `-config`. Typos are
  reported. `wms config path` / `wms config init [--portable]`. See [docs/CONFIG.md](docs/CONFIG.md).
- **Portable mode**: a `wms.json` next to the program keeps all data in that folder.
- **Custom folders** for metadata artwork, cache, transcodes and logs.
- **Logs**: daily files with opt-in categories (playback, discs, FFmpeg, library, web
  requests, app errors), secrets always removed, kept 14 days. *Settings → Logs & privacy*
  explains that logs never leave the server, and has the folder, an *Open folder* button and
  downloads. The web app reports its errors to the server only when allowed there.

### TV, controllers and accessibility

- Arrow keys / D-pad move between controls on every page; Back returns. Game controllers
  (Xbox, PlayStation, Nintendo, generic) work through the Gamepad API.
- Screen readers: labelled landmarks, live announcements, DVD menu options as real buttons,
  visible focus, reduced-motion and high-contrast support.

### Interface

- Fixed missing borders in the default look (a circular colour variable).
- Theme cards: the whole card is clickable and switching themes is reliable (the new theme
  replaces the old one only once it has loaded).

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

- Metadata and artwork are **saved next to media** in the standard NFO + artwork layout other media servers and Kodi read
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
