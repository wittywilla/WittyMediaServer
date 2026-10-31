# Changelog

All notable changes to WittyMediaServer. Versions follow [semantic versioning](https://semver.org);
`-alpha` releases may change settings and APIs between versions.

## Unreleased

### Smooth DVD playback

- **Fixed:** a DVD could freeze or skip for a moment around a chapter change, with nothing pressed, and the
  sound could slip with it. The disc's sections were read one by one and placed by their earliest sound, which
  lies up to a quarter of a second ahead of the picture and differs each time. Sections that follow each other on
  the disc are now read as one recording, and where a join is real its first picture lands exactly in place.
- **Fixed:** a smaller stutter every few minutes on DVD and Blu-ray, at the moment the server starts converting
  again after resting. Sound and picture now carry on from exactly where they left off.
- **Fixed:** skipping within a DVD or Blu-ray could hold the first picture for up to half a second. It now lands
  on the right frame.
- **New for developers:** a measuring check compares a stream with the disc, frame by frame and sound by sound,
  at every chapter. See [DVD.md](docs/DVD.md#diagnosing-a-disc).

### Interface audit

- **Fixed:** the TV guide went blank with no channels. The home banner could be an empty box when the newest
  titles had no art. The phone's bottom bar wrapped onto two rows. A folder typed without pressing Enter left
  Add greyed out in setup. A remote's Back button did nothing in a dialog. Home videos under 5 MB were skipped.
- **Games:** large libraries load a page at a time (they stopped at 2,000 before), with search and console filters.
  Files a scan can't use, such as .7z archives, are now reported instead of quietly missing.
- **Wording:** movies, shows and ads throughout. Sign in a TV is clearer, and search asks for a word instead of
  listing everything.

### Modules update on their own

- Every module now has its own version and states the oldest and newest server it works with.
  **Settings, Modules** lists them with their status, and a version that doesn't fit is refused with a
  plain reason while the built-in copy carries on.
- **WittyPhotos** is the first that can update without a new server: signed like server updates, installed
  with no restart, and taken back out by itself if it keeps failing. Other modules follow. Looking for
  updates follows the existing update setting, and installing them automatically is off unless turned on.
  See [MODULES.md](docs/MODULES.md).

### New libraries

- **WittyPhotos**: a gallery with a timeline, albums, favourites, folders and a map. JPEG, PNG, GIF, WebP,
  BMP, TIFF, HEIC, AVIF, JPEG XL, camera RAW (shown by the preview the camera stored) and clips, with live
  photos and RAW+JPEG pairs grouped. Scanning reads only file headers; thumbnails are made when first looked
  at. Uploads from any device, slideshows, and a clearly marked placeholder for the local AI. The map needs
  nothing online; an online map is an opt-in setting. See [PHOTOS.md](docs/PHOTOS.md).
- **WittyShorts**: vertical videos, one at a time, with likes, comments, sharing, creators, custom metadata
  and a For You page worked out from what each profile did on this server, and nothing else. See
  [SHORTS.md](docs/SHORTS.md).
- **WittyChannels**: creator videos already on disk, with a home, subscriptions, channel pages, playlists and a
  watch page with comments and up next. Reads the sidecar files video archivers leave, including saved
  comments. See [CHANNELS.md](docs/CHANNELS.md).
- **Music videos and concerts**: by artist, with recordings of 20 minutes or more under Concerts and their
  chapters as a setlist.
- **Internet radio**: stations by stream address or M3U/PLS playlist, relayed by the server, which contacts a
  station only while someone is listening. See [COLLECTIONS.md](docs/COLLECTIONS.md).

### New everywhere

- **Collections and smart playlists** across libraries, with Watch later and Play all.
- **Links for people without an account**, for pictures, clips and albums: with an end date, showing only
  what they name, with EXIF and locations left out unless downloads are allowed. Only a fingerprint of a link
  is stored. Listed and ended in Settings, Sharing & photos.
- **Your year**: time, days, and top titles, shows, artists, creators and genres, from each profile's own record
  on the server. **Recommended for you** on the home page, from the same record.
- **Playback speed on every media type**: music, audiobooks, podcasts and vertical videos join video and discs,
  up to the server's limit. Each kind of listening keeps its own speed, and < and > step through them.
- **Chapters from video files** in the player, for every video library.
- The **WittyPhotos** engine (`modules/wittyphotos`): EXIF, picture sizes and RAW previews in pure Go, fuzz-tested.

Everything new has JSON routes, plays through standard HLS or direct files, and works with a D-pad, for the
coming TV apps.
## Unreleased: security

A full security review and its fixes; the details are in `docs/SECURITY-REVIEW.md`. Things you may
notice:

- First-time setup from another network asks for a code from the server's log.
- New passwords for full accounts need 8 characters (Kids and Guest PINs stay at 4; passwords
  already set keep working).
- `maxTranscodes` is now enforced: beyond it, viewers are told the server is busy.
- Files that are links leading out of their library are no longer indexed or served.
- The expert FFmpeg fields accept only options that tune encoding.
- Accounts other than the administrator no longer see server folders or podcast feed addresses.
- An HLS playlist opened with a token in its address names its segments with a token for that stream
  alone, so the session's own token doesn't end up in playlists.
Everything found in a full audit of the code; [docs/AUDIT.md](docs/AUDIT.md) has the details.

### Security

- Nothing the API answers can run as a page on this site; a podcast host could have answered with
  a script. Podcast episodes are only passed on as sound.
- Saving metadata can't be made to copy another kind of file beside your media, or to fetch from
  your local network.
- Accounts with limits no longer see hidden shows' artwork, genres, networks or podcasts, and only
  administrators see where files are on the server.
- Sign-in slows down one address trying many accounts, and a flood of attempts can't take the
  whole processor.
- Releases are built with the newest Go patch release.

### Faster, on small machines above all

- The first page loads a third of the script it did; the player, readers and Settings come when
  opened, and scripts travel compressed.
- The home page and library pages ask far less of the database (home: 146 ms to 33 ms on a test
  library of 25000 items, with a thirtieth of the memory churn). Libraries load 120 items at a time.
- FFmpeg rests when it is well ahead of the viewer, *maxTranscodes* is really enforced, and the
  player says when it is done with a stream.

### Fixed

- A film paused for over 10 minutes hung when resumed.
- A library folder that is away no longer marks everything in it as missing.
- An interrupted database upgrade could stop the server from ever starting again.
- One DVD in 256 opened as "expired". Library pages flickered while scanning.
- Starting the server deleted the whole transcode folder, whatever else was in it.
### Getting ready for the TV apps

- **Client API**: `/api/v1`, `GET /api/capabilities`, and [a guide](docs/CLIENT-API.md) and
  [OpenAPI file](docs/openapi.json) covering every route. A test fails when a route is left out, or
  when what they say about sign-in isn't what the server does.
- **One way to play anything**: `POST /api/items/{id}/playback` takes what a device can play and
  chooses direct play, remux or transcode. Built-in profiles for Apple TV and Android TV; apps can
  add what their device reports. Remuxing keeps the picture untouched, in fMP4 for HEVC, for almost
  no work on the server.
- **Stream tokens**: short-lived, read-only addresses for one item's media, for players that can't
  send headers. They end with the sign-in they came from.
- **Apps can find the server on the home network** (mDNS, with an off switch), and check it is really
  your server with a key they remember, so a look-alike is noticed. Only its name, a random ID and
  its version are announced.
- **TV sign-in with a code**, either way round: the TV shows a code, and someone signed in approves it at
  *Sign in a TV*, after seeing which device is asking; or that page shows 6 digits to type on the device.
- **Subtitles for native players**: text tracks as WebVTT, by HLS rendition or as a separate file.
- **Themes as data**: `GET /api/themes/{id}/tokens.json` gives a theme's colours, lengths and fonts.
- The resume rule and how a disc starts now come from the server, for every client.

### Fixed

- **Text subtitles (SRT, ASS) that are burned in now show.** Choosing one converted the video but drew
  nothing.
- **TV remotes and controllers**: no more getting stuck in the search box, a drop-down list or the
  seek bar, and lists no longer change value as focus passes over them (OK opens them). OK brings up
  the player's buttons, and reaches Skip intro and Play when those are waiting. Disc menus hand over to
  the player's buttons at their edges. Episode cards and a disc's list of all titles can be reached.
  In books, OK brings up the toolbar and walking along it no longer turns the page.
### Remote access

Reach your server from outside your home ([REMOTE.md](docs/REMOTE.md)). Everything is off until you
turn it on in the new **Settings → Remote Access**, and nothing phones home.

- **A clear status:** whether the server can be reached, from where and how, based on what it has
  really seen.
- **Guided steps** for Tailscale, NetBird, forwarding a port, your own reverse proxy and a rented
  server. A hosted relay is shown greyed out as a possible future paid service; it doesn't exist.
- **A test with your own phone:** a QR code and a secret address; no outside service is involved.
- **HTTPS** without a restart, with Let's Encrypt or your own certificate (renewals are picked up by
  themselves), beside the home port.
- **Automatic port mapping** with UPnP or NAT-PMP, off by default. It only ever opens the HTTPS port,
  and closes it again. It also says when your provider shares addresses, so forwarding can't work.
- **Limits for viewers away from home:** highest resolution and most data per stream. Every encoder is
  told in the way it really obeys, direct play and direct downloads keep to them too, and playback at
  home is never affected.
- **The apps** ask for a server address: the mobile app has a screen for it, and the desktop app warns
  about unencrypted addresses outside a private network.

### Security

- **Connections from outside your home network are now refused until Remote Access is on.** Servers
  with `publicUrl` set count as on. Behind a reverse proxy, list it under the new trusted proxies
  (Settings, `trustedProxies` or `WMS_TRUSTED_PROXIES`). Forwarded headers are believed from nobody else.
- **Unencrypted connections from the internet are refused** or sent to the HTTPS address.
- **Two-factor sign-in:** optional codes from an authenticator app, with recovery codes, under the new
  **Settings → Sign-in & security**. `wms reset-2fa` is the way back for an administrator.
- **Your devices:** see where each account is signed in, and sign any of them out.
- **Sign-in limits** now also cover one address trying many names and one name tried from many
  addresses (away from home only, so nobody can lock you out at home), and how many passwords are
  checked at once. Changing a password counts too. Wrong tries and locks are shown in Settings.
- **Weak administrator passwords** are warned about when remote access is on.
- **Other websites** can no longer read the server's answers without a sign-in token, such as the list
  of profiles of a server on your network.
- Fixed: with *Show profiles outside your home* on, password-free profiles could sign in from outside.
- Players that can only sign in with `?token=` can now play HLS: the playlist carries it on to its
  segments, when that is how the playlist was fetched.
- Over HTTPS, the web app no longer puts its sign-in token in media addresses, where a reverse proxy
  would log it. The version is hidden from visitors on the internet who aren't signed in.
### TV channels and live TV

See [CHANNELS.md](docs/CHANNELS.md).

- **Channels from your library** that run all the time: tune in part-way through a programme, as on
  real TV. One click makes one from a TV network, an era, a genre or a show. Rules, day parts and a
  full weekly timetable are there for whoever wants them, and moving between them loses nothing.
- **As it aired**: shows in the day and time they first aired, where the metadata knows it.
- **Commercials libraries**, as many as you like. Each channel uses a whole library or only the videos
  you tick. Each library can be shown in the menus or kept out of the way.
- **A guide with a look of your choosing**: a bulky classic cable or satellite box, or clean streaming
  rows, with a line at the current time down all channels and a live preview you can put anywhere.
- **Live TV** through the new **WittyTuner** module: IPTV playlists and streams, network tuners, Linux
  DVB, capture cards, SAT>IP, external backends and a custom command. Most drivers have not been tried
  on real hardware yet, and say so.
- **Encrypted and copy-protected channels are detected and refused**, never decrypted. Every channel
  shows whether it is clear or encrypted, with a help page, and the same station from a clear source
  is offered when there is one.
- **Unencrypted ATSC 3.0** tunes, plays and records. The bundled FFmpeg has no AC-4 decoder, so channels
  with AC-4 sound play silently for now, and say so.
- **Guide data** from XMLTV and from the broadcasts themselves, and **recording** of live channels.
- One stream per channel however many are watching, and FFmpeg runs only while someone is.
### Games

- **Games libraries**: add one for a console, or for a folder of many consoles, with several libraries of
  one console allowed. Zipped games and multi-file games are understood. Bring your own game files.
- **Play in the browser**: games run on the server, each in its own sandboxed process, and stream with
  low latency over WebRTC. Where WebRTC can't connect, the player falls back to a WebSocket with WebCodecs,
  and then to JPEG, without restarting the game.
- **Every controller**: gamepads with hot-plug and up to four players, a keyboard, and a screen for
  changing what every button does.
- **Touch controls**: an on-screen controller per console, with an editor to move, resize, fade or hide
  each control, saved per console, device and orientation. Android gets haptics.
- **Saves**: battery saves and nine save-state slots per account with pictures, and a resume state written
  when you stop.
- **Settings, Gaming**: install cores on request with each licence shown first, and set up streaming.
- **Ready for TV apps**: controller layouts live on the server, and `docs/GAMING-API.md` documents everything a client
  needs. The player works with a TV remote's D-pad.
### Interface

- A new look across every page, built on one design system (see `docs/DESIGN.md`): one type scale,
  a spacing grid, shared buttons, fields, dialogs, cards and empty and loading states. All 20 themes
  are updated; a theme's `--radius` now shapes the whole interface.
- Backdrops run behind the top bar, rows of cards have arrows for the mouse, and dialogs are bottom
  sheets on phones.
- Faster on low-end devices: no blur on anything that scrolls or covers video, and long libraries
  skip the cards that are off screen.
- Accessibility: the featured banner can be paused, field outlines and switches reach 3:1, every
  target is at least 24px, and a focused control is never hidden under the bars.
- Rows show an album once instead of once per song; books no longer say "0 min".

## 0.4.0-alpha

0.3.0-alpha was never published as a release, so this is the first release with everything in it
as well (see its section below).

### Player

- **Scenes**: the Scenes button opens every scene as a picture with its time and name, like the
  disc page, with the one playing now marked.
- **Next moves on**: on the last scene, or in a short clip such as a warning or a trailer, Next goes
  on to what the disc plays next instead of starting the clip again.
- **No black flash** when a disc moves from one clip to the next (a menu to the next screen, Far Far
  Away Idol's show to its vote, a vote to its answer): the last picture stays up until the next
  clip has one.
- **Forced captions** can be turned off in the viewing settings, for one disc or everything.
- The disc remote fits its own edges with every engine name (WittyBluray was cut off).

### DVDs

- **Researched against the DVD-Video specification and the quirks real discs have**
  ([DVD-QUIRKS.md](docs/DVD-QUIRKS.md)): random and shuffle program chains, parental blocks, menu
  language fallback, junk cells used as structural protection, cells without NAV packs, every
  sub-picture control sequence with colour regions (CHG_COLCON), buttons that end when the disc ends
  them, and reserved bits in jump targets. Copy protection is still never decrypted.
- Clips that hold their last picture (menus, games, credits) are never restarted part-way, which
  left empty segments: Far Far Away Idol's credits and their music no longer race by.

### Libraries and setup

- **Built-in folder browser** (WittyIntegratedFileBrowser): choose the server's folders from any
  device, with drives and free space, your folders, a path you can type, filtering, several folders
  at once and full keyboard and screen-reader support. Read-only.
- **Folders of the same kind** join that kind's library or become a new, named one: set the default
  in setup and in *Settings → Libraries*, and choose either way when adding. Setup and Settings
  show how many libraries there are.
- **Server name and logo**: a Rename window with a live preview and a logo upload.
- Setup: a centred welcome, and no Continue button while you choose a folder.

### Accessibility

- All 20 themes meet WCAG 2.2 AA contrast for text and buttons; a new `--accent-text` colours
  links and highlighted words (see [THEMES.md](docs/THEMES.md)).
- Dialogs keep the keyboard while open and give focus back; page changes are announced; settings
  controls are named by their rows; the player says Play/Pause, reads its timeline, speaks what
  changes and lists its keys under **?**.

### Fixes

- Pictures load when the browser holds another server's (or an expired) sign-in cookie.
- The docked music player no longer moves the profile button; the profile's name shows beside it.


### Updates

- **One-click updates**: *Settings → About & legal → Updates* shows the installed and the newest
  version with its release notes. **Update now** downloads it, verifies it, installs it and
  restarts; the page reloads by itself. Admins see a dot on *Settings* when an update is out.
- **Only verified releases install**: each release's checksum list is signed with the project's
  Ed25519 key, built into the server. The server checks that signature and the package's
  SHA-256, downloads only from GitHub over HTTPS, and refuses anything that doesn't match.
- **Your data is never touched**: the new files are swapped in next to the running ones (old
  programs are set aside and removed on the next start); the data folder, `wms.json` and the other
  data folders are never written, and a failed install puts every file back.
- The server looks for new versions twice a day (it only asks GitHub for its list of releases);
  *Check for updates automatically* turns that off.
- Copies that can't replace their own files (macOS, Docker, `install.sh` installs owned by root, a
  read-only program folder) say why and link to the download.

### Windows

- **A setup program** (`…-windows-amd64-setup.exe`): installs for your account without
  administrator rights, adds a Start menu entry, starts at sign-in and appears in *Installed apps*
  with an uninstaller that keeps your library. Installed this way, the server updates itself.
  `wms-setup.exe` in the zip installs an unpacked folder the same way.

### For maintainers

- `cmd/wms-sign` makes the release signing key and signs checksum lists;
  `scripts/build-release.sh` signs each release (`WMS_SIGNING_KEY`, default
  `~/.wms-signing/release.key`) and builds the Windows setup. The release workflow signs with the
  `WMS_SIGNING_KEY` repository secret. See *Signing releases* in `docs/PUBLISHING.md`.

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

- Metadata and artwork are **saved next to media** in the standard NFO + artwork layout other media software reads
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
