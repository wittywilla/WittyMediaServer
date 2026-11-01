# What each media type supports

Use this guide to choose what to add to your server: for each kind of media, it says which library to make, which files it takes, what works, what needs a module, and what's refused.

You add a library under **Settings → Libraries → Add library**. A kind of library that needs a module you don't have is shown with its name and a link to the store: see [Modules and the store](MODULE-STORE.md). For every file extension, see [Supported formats](SUPPORTED-FORMATS.md). The same table is in the app under **Settings → About → Supported formats**, where it also says what your own FFmpeg can do.

## At a glance

| Media | Library to add | Needs a module |
|---|---|---|
| [Films](#films) | Movies | No |
| [TV shows](#tv-shows) | TV shows, or Anime | No |
| [Home videos](#home-videos) | Home videos | No |
| [Discs](#dvds-blu-rays-and-audio-cds) | DVDs, Blu-rays & CDs | WittyDVD, WittyBluray or WittyCD |
| [Music](#music) | Music | No. CD images need WittyCD |
| [Audiobooks](#audiobooks) | Audiobooks | No |
| [Podcasts](#podcasts) | Podcasts | No |
| [Internet radio](#internet-radio) | Internet radio | No |
| [Photos](#photos) | Photos | WittyPhotos |
| [Books](#books) | Books & comics | WittyEbook or WittyPDF |
| [Comics](#comics) | Books & comics | WittyComics |
| [Games](#games) | Games | WittyPlay |
| [Creator videos](#creator-videos-music-videos-and-concerts) | Creator videos, or Music videos & concerts | No |
| [Shorts](#shorts) | Vertical videos | No |
| [Live TV](#live-tv) | None: **Settings → Channels & tuners** | WittyTuner for tuners and IPTV. Channels from your library need nothing |

## How far this has been checked

- The format table has only been run on Linux.
- Safari, phones, hardware decoding of HEVC and HDR screens have not been checked.
- The web app is the most complete way to use all of this, in any browser. The native apps cover less, and [Apps and devices](APPS.md) says what each one does. Android has only run on emulators, and the Apple apps have never been built.

## Films

- **Library:** Movies. Disc images of films can go here too, and then need the disc modules below.
- **Files:** nearly any video file, such as MP4, MKV, AVI, MOV, WebM, MPEG and transport streams. A film split across files (`Film CD1.avi`, `Film CD2.avi`) is one title.
- **Resume:** yes, per profile.
- **Subtitles:** tracks inside the file, and files beside it such as `Film.en.srt` or `Film.en.forced.srt`, or in a `Subs` folder. Subtitles stored as pictures (from DVDs and Blu-rays) are drawn into the picture.
- **Details:** TMDB with your own key, otherwise Wikidata and Wikipedia, with no key but no posters. You can also use NFO files and artwork beside the file. See [Where details come from](METADATA.md).
- **Playing:** a file plays as it is if your device can. Otherwise only what has to change is converted while you watch: see [Live transcoding](TRANSCODING.md).
- **Refused:** nothing here gets around copy protection. A copy-protected file is refused by [Library transcoding](LIBRARY-TRANSCODING.md), and a protected TiVo recording is refused outright.

## TV shows

- **Library:** TV shows. Anime can have its own Anime library, with details from AniList.
- **Files:** the same as films. Season folders and names such as `S01E02` tell it what each episode is: see [TV shows](TV.md).
- **Resume:** yes, and **Play** on a show carries on from the next episode.
- **Also:** Skip intro, the next episode offered near the credits, and specials as a season of their own.
- **Details:** TVmaze with no key, or TheTVDB with your own key. Anime libraries use AniList with no key.

## Home videos

- **Library:** Home videos.
- **Files:** any video file the server plays.
- **Resume and subtitles:** as for films.
- **Details:** nothing is looked up online. The file's name and anything beside it are all it uses.

## DVDs, Blu-rays and audio CDs

- **Library:** DVDs, Blu-rays & CDs. Disc images of films can also go in a Movies library, and of a series in a TV shows library.
- **Files:** images (`.iso`, `.img` and others), and folders holding `VIDEO_TS` or `BDMV`. Audio CD images use `.cue`, `.toc`, `.ccd`, `.nrg` and `.chd`.
- **Modules:** WittyDVD for DVDs, WittyBluray for Blu-rays, WittyCD for audio CDs. Add only the ones you want. A disc in a library with no module for it stays listed, with nothing known about what's in it, until you add one.
- **Menus:** real disc menus in the browser, with an on-screen remote. A Blu-ray with Java menus needs a Java you install yourself, and it's off until you turn it on: see [Blu-rays](BLURAY.md#bd-j-java-menus).
- **Resume:** yes, in the title you were watching.
- **Subtitles and sound:** the disc's own tracks and languages, chapters and camera angles. Choose your preferred languages under **Settings → Playback & discs**.
- **Details:** films and series are matched as usual. Audio CDs use CD-TEXT, then MusicBrainz and the Cover Art Archive, which are asked only about the track lengths.
- **Refused:** encrypted discs (CSS on DVDs, AACS on Blu-rays) are listed with a clear message and can't be played. BD+ is never run.

See [DVDs](DVD.md) and [Blu-rays](BLURAY.md).

## Music

- **Library:** Music.
- **Files:** MP3, AAC, FLAC, Ogg, Opus and WAV play as they are in a browser. Everything else, including Apple Lossless, WMA, Monkey's Audio, DSD, MIDI, tracker and game music, is converted as it plays, and your files are left alone. Tracker and game music need an FFmpeg with those libraries. A `.cue` sheet splits one long sound file into tracks.
- **Lyrics:** from `.lrc` or `.txt` files beside a song, or from its tags, and online only if you turn that on. There's also a karaoke display: see [Lyrics and karaoke](LYRICS-KARAOKE.md).
- **Details:** tags and covers are read from your files. See [Music, audiobooks and podcasts](MUSIC.md).
- **Modules:** nothing for ordinary music. WittyCD shows CD images among your music, and WittyVisualizer adds light shows that follow it.
- **Playlists:** M3U, PLS, XSPF and Windows Media playlists in the library become playlists of the songs they name. Web addresses in them are ignored.

## Audiobooks

- **Library:** Audiobooks.
- **Files:** the same as music. `.m4b` is the usual one.
- **Resume:** yes. **Continue** picks up at the chapter and place you left.
- **Details:** read from your files.

## Podcasts

- **Library:** Podcasts. The folder is optional, and only holds downloads.
- **Files:** you follow feeds by searching or pasting an address. Episode files can sit in the folder too.
- **Resume:** your place in an episode is saved.
- **Details:** from the feed. Episodes marked explicit are hidden from Kids and Guest profiles.
- **Privacy:** episodes play through your server, so your devices don't contact the podcast's host.

## Internet radio

- **Library:** Internet radio. It needs no folder.
- **Stations:** search a free directory, type a stream address, or give an M3U or PLS playlist.
- **Resume:** it's live, so there's nothing to resume. Favourites and what you played lately stay at the top.
- **Details:** logos, genres and countries from the directory, and what's playing now if the station says. See [Collections, radio and Your year](COLLECTIONS.md).

## Photos

- **Library:** Photos. Needs **WittyPhotos**.
- **Files:** JPEG, PNG, GIF, WebP and AVIF as they are. HEIC, TIFF, JPEG XL, Photoshop files and others are shown as a JPEG the server makes. Camera RAW shows the preview the camera stored, and nothing is developed. Short clips and live photos play.
- **Details:** dates and places come from the photo itself, and nothing is looked up online unless you use the map.
- **Your files:** never changed. See [Photos](PHOTOS.md).

## Books

- **Library:** Books & comics. Needs **WittyEbook** for EPUB and the formats below that become EPUB, and **WittyPDF** for PDFs.
- **Files:** EPUB and PDF open as they are. Kindle (MOBI, AZW), FictionBook, Word, OpenDocument, Rich Text, Markdown, web pages and plain text are converted to EPUB once, on the server, and the original is left alone. DjVu needs the DjVuLibre tools on the server, and without them it can only be downloaded.
- **Resume:** your place and your notes are saved per profile, across devices.
- **Refused:** copy-protected Kindle books and password-protected files, with a clear message.

See [Books and comics](BOOKS.md).

## Comics

- **Library:** Books & comics. Needs **WittyComics**.
- **Files:** `.cbz`, `.cbr`, `.cb7` and `.cbt`. The archive is recognised by what's inside it, so a RAR file named `.cbz` still opens.
- **Reading:** pages, spreads, vertical strips and either reading direction.
- **Resume:** your place is saved per profile.

## Games

- **Library:** Games, for one console or one folder holding many. Needs **WittyPlay**, plus a core for each console and any BIOS it needs.
- **Files:** your own game files, as they are or in archives. **Settings → Consoles & cores** lists what this server can play.
- **Playing:** games run on the server and stream to a browser, with your controller. Playing on the device itself isn't built. Firefox, Safari, phones, TVs and real networks haven't been tried for game streaming.
- **Resume:** saves and memory cards follow your account.
- **Details:** box art and details from game databases, as you choose.
- **Modules:** to play on a PC at home instead, **WittyCouch** connects to one, and **WittyRigs** is an advanced option for this server's own hardware. See [Game PCs](GAME-RIGS.md).
- **Refused:** encrypted game files and password-protected archives are listed with a clear message. A 3DS game file that's really encrypted is refused, for example.

See [Games](GAMES.md).

## Creator videos, music videos and concerts

- **Library:** Creator videos, with one folder per channel. Or Music videos & concerts, with `Artist - Title` names or a folder per artist.
- **Files:** any video the server plays.
- **Resume and subtitles:** as for films.
- **Details:** from sidecar files beside the video. Nothing downloads, and no video site is contacted. Recordings of 20 minutes or more are listed under Concerts, with their chapters as a setlist.

See [Creator videos](CREATOR-VIDEOS.md).

## Shorts

- **Library:** Vertical videos, with one folder per creator.
- **Files:** any video the server plays.
- **Watching:** a feed you swipe, or move through with arrow keys or a remote. Likes, comments and a For You page are worked out on your server alone.
- **Details:** from sidecar files, or the file's name.

See [Shorts](SHORTS.md).

## Live TV

- **Where:** **Settings → Channels & tuners**, and **TV guide** in the menu. You don't add a library.
- **Channels from your library:** no module, and no extra hardware. DVD and Blu-ray main features and scenes can be on a channel if you have the disc modules.
- **Tuners and IPTV:** need **WittyTuner**. No channel lists come with the server.
- **Recording:** tuner and IPTV channels only, on a schedule that belongs to each person.
- **Refused:** encrypted channels are labelled and can't be tuned or recorded. Nothing decrypts them.

See [Channels and live TV](CHANNELS.md).

---

<sub>WittyMediaServer documentation</sub>
