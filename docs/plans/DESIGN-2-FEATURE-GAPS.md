# Design 2: features with no drawn UI element

Date: 2026-11-12. Branch: wip/design.

Owner requirement: every feature the WittyMediaServer server and its modules offer must have a UI element in the Design 2 concept canvas ("Witty Design 2 Concepts"). This list names the features that do not, so a designer can draw them.

## How this was made

1. The canvas was read from its downloaded files: `canvas.json` (11 pages, about 100 boards), `Exp.dc.html` (the shared renderer, whose `screen` enum is the list of drawn screens: home, library, detail, player, search, settings, menu, dialog, signin, nowplaying, games, you, guide, tvinfo, tvwatch, reader, pdf, comics, photos, stats, viz, files, disc, cd, couch, songs, karaoke, vizfull, setup, setupblock, settingsLook, policy, chooselook, tvrec, tvsets, tvlook, remote, remoteDvd, remoteVideo), the WittyPlay Online boards (`Play.dc.html`, `P_*`), the music, player, WittyTV, settings and first-run boards, the layout editor and the server name editor. The text in every board was extracted and searched.
2. The server's features were taken from `docs/*.md` (PLAYING, SYNCPLAY, REMOTE-PROTOCOL, PROFILES, ACCOUNTS, COLLECTIONS, MUSIC, LYRICS-KARAOKE, PHOTOS, BOOKS, GAMES, GAME-RIGS, GAMING-STREAM, CHANNELS, TV, SHORTS, CREATOR-VIDEOS, STATS, TASKS, BACKUP, REPAIR, DIAGNOSER, LOGGING, FIXING-METADATA, METADATA, MODULE-STORE, LIBRARIES, TDARR, LIBRARY-TRANSCODING, APPS, CLIENT-API and others), the page and route list in `web/src/App.tsx` and `web/src/pages/*`, and the module READMEs and `sdui.json` files beside the server (WittyPlay, WittyCouch, WittyCouchHost, WittyRigs, WittyDVD, WittyBluray, WittyDisc, WittyCD, WittyTuner, WittyInk, WittyEbook, WittyPDF, WittyComics, WittyReader, WittyPhotos, WittyStats, WittyVisualizer, WittyIntegratedFileBrowser, WittyMediaPlayer). The module `sdui.json` files hold only a theme scope and, for WittyPlay, one consoles page; the functions come from the READMEs.
3. Only features that exist in a doc, route or module file are listed, each with its source. Two things were looked for and are not in the server, so they are not listed: subtitle search or download from an online service, and a language picker for the apps' own text (only "Preferred languages" for discs exists, under Settings, Playback and discs).

## How "drawn" was judged

A feature counts as drawn only if a screen or board visibly contains its control, state or result. These do not count as drawn:

- A name in the `ModuleMatrix` table or in a board caption (the matrix is a text table of intentions, not a drawing; several of its claims have no matching screen, for example "Why this title", wake a PC, CD+G, EXIF facts, typography and night modes).
- A button label with nothing behind it (the player's "⋯", the reader's "Aa", the Photos tab "Map", the You menu rows "Your devices" and "Profiles and parental limits").
- A step of first-run setup that only toggles a module or an option on.

Settings is hand-built by decision and only Appearance is drawn. Settings pages are not listed as gaps unless the function is something a person uses outside Settings. A few admin functions the owner named (backup, repair, diagnoser, logs, module store, metadata fixing, Tdarr) are listed anyway, marked "Settings page", because the owner asked for every function; the designer may decide to draw them as one generic settings pattern.

If a feature is drawn in one look or on one platform only, or only in part, it is under "Drawn but incomplete" at the end, not in the table.

## Gaps

| # | Feature | Where the server or module defines it | Screen it belongs on | Why it is not drawn |
|---|---|---|---|---|
| | **Playback** | | | |
| 1 | How this is playing panel: Direct play, Rewrapped or Transcoding, why, work for the server, Show it in 2D, list of video streams | PLAYING.md "How this is playing"; CLIENT-API.md "Playing something" | Player (info button, key I) | Nothing found. The player draws no title-line badge or info button. |
| 2 | Next episode card with the 8 second countdown and Dismiss; "something else to watch" after a film | PLAYING.md "What's next"; TV.md "Show pages" | Player, last 30 seconds | Nothing found. Only "Skip intro" is drawn. |
| 3 | Skip recap, Skip credits and Skip adverts buttons | PLAYING.md "Recaps, credits and adverts"; `GET /media/{id}/markers` | Player | Only "Skip intro" is drawn. |
| 4 | Automatic skipping note ("Skipped intro" with Undo, credits countdown, Watch the credits) | PLAYING.md "Skipping without pressing anything" | Player, toast | Nothing found. The Undo toast drawn is for Watch later, not skipping. |
| 5 | Viewing settings (gear): Apply to this video or everything, hide controls after, Fit or Fill, seek bar and chapter marks, 3D as side by side or over-under, Streaming (Server's choice, Standard, Robust) | PLAYING.md "Viewing settings" | Player | Nothing found. The player has a "⋯" with no menu drawn. |
| 6 | Robust streaming notices ("Picture lowered to 480p to keep playing", "Reconnected after 12 s") and its choice | PLAYING.md "Robust streaming"; CLIENT-API.md "Robust streaming" | Player overlay | Nothing found. |
| 7 | Mini player in the corner (back 10 seconds, play, next, Remote, Full player, Stop) | PLAYING.md "The mini player" | Any page while playing | Nothing found. Only the music bar on the Now playing screen exists. |
| 8 | Volume, mute, full screen and the key list (?) in the video player | PLAYING.md "The player's controls" | Player | Nothing found. The bottom bar draws Prev, -10, play, +30, Next, Audio, Subtitles, 4K, speed, Chapters and "⋯" only. |
| 9 | Audio & language and Subtitles pickers (tiles with language, format and channels, tick, Off, "With its styling" and "Drawn into the picture") | PLAYING.md "Sound tracks and subtitles"; CLIENT-API.md "Subtitles" | Player | Buttons are drawn; the pickers are not. |
| 10 | Play on another device button in the player's top bar, hand over at the same place | PLAYING.md "Playing on another device" | Player (video, disc, music) | The Play on screen is drawn, the hand-over control in the player is not. |
| 11 | Playback error and recovery states: "Playback failed" with reason, "The stream has ended on the server", subtitles could not load, buffering | PLAYING.md "If it won't play" | Player | Nothing found. |
| 12 | Disc settings in the player: Forced captions, Disc restrictions, Subtitle size, remembered sound and subtitle language and angle | PLAYING.md "Viewing settings" (discs); DVD.md "Disc settings"; BLURAY.md | Player (DVD and Blu-ray looks) | Only the dimmed remote keys and an angle key are drawn. |
| 13 | "Why this title": which title or playlist is the film, how sure, other cuts, admin picks another, ranking again | DVD.md "Which title is the film"; BLURAY.md; web/src/pages/DiscWhy.tsx; WittyMediaPlayer README (Disc, Why this title) | Disc item page; WittyMediaPlayer | Only in the ModuleMatrix text. |
| 14 | Disc item page sections: Feature Presentation with scenes, bonus material, shortcuts into the disc's menus, admin naming of titles and scenes | web/src/pages/DiscSections.tsx; DVD.md "Playing a disc" | Item page (disc) | Nothing found. The disc screen draws only the menu on the picture. |
| 15 | Encrypted or copy-protected disc refusal message; BD-J (Java) menus and their states | BLURAY.md "BD-J (Java) menus", "Limits"; WittyMediaPlayer README ("Says no plainly") | Disc player | Only in the ModuleMatrix text ("plain refusal message"). |
| 16 | Disc diagnostics result for one disc | DIAGNOSER.md "Checking a disc"; DVD.md "Diagnosing a disc" | Item page (disc) | Nothing found. |
| 17 | WittyMediaPlayer app: open a file, folder, drive or image, recent items, File Settings (VLC libraries, module updates, Look for updates), Disc menu (cuts, Play main feature) | WittyMediaPlayer README "What it does" | WittyMediaPlayer window | The matrix says it shares the player and remote; its own menus and settings are not drawn. |
| 18 | WittyMediaPlayer: copy tracks from an audio CD (FLAC, ALAC, WAV, MP3, AAC, Opus, Ogg), make a disc image, Look up names | WittyMediaPlayer README (CD screen, Makes disc images) | WittyMediaPlayer CD screen | Nothing found. |
| | **Library and item pages** | | | |
| 19 | Library filters panel (category, studio, network, type, decade, year, age rating, score, picture size, audio language, subtitles, sound, edition, tag, collection, watched) and the chosen-filter chips | web/src/pages/library/Filters.tsx; METADATA.md "Categories, studios and browsing"; PLAYING.md "Resuming" (Watched filter) | Library | Only the sort chip "Sort: Added" is drawn. |
| 20 | Saved views (save the chosen filters and sort, come back to them) | web/src/pages/library/Filters.tsx (`SavedView`) | Library | Nothing found. The word "saved" appears only in the build list. |
| 21 | Studio, network and category pages (everything from one studio, across libraries) | web/src/pages/library/BrandPage.tsx, route `/brand/:kind/:name`; METADATA.md | Library | Only the view tab names "Genres" and "Collections" are drawn. |
| 22 | Show page: seasons, episode list, Cards or Playlist layout, Play the next episode with Resume, specials | TV.md "Show pages", "Seasons and specials"; route `/show/:id` | Item page (show) | Nothing found. Only a film's item page is drawn. |
| 23 | Versions and editions of a title (pick which file plays) | FIXING-METADATA.md "Bonus features, versions and moving"; `local.versions` in CLIENT-API.md | Item page | Nothing found. |
| 24 | Where to watch on the library item page (here and on streaming services, region) | METADATA.md "Streaming listings"; web/src/components/WhereToWatch.tsx; `GET /media/{id}/where-to-watch` | Item page | Drawn only inside the WittyTV More info pop-up. |
| 25 | Bonus features on a film's page: trailers, featurettes, deleted scenes, interviews | FIXING-METADATA.md "Bonus features, versions and moving"; LIBRARIES.md "Movies" | Item page | Nothing found. Trailers are drawn only inside the WittyTV More info pop-up; "Extras" appears only as a disc menu entry and a folder row. |
| 26 | Person page (filmography of an actor or director) | Route `/person/:id`; CLIENT-API.md "People" | Item page, Search | Only a row of cast names is drawn. |
| 27 | Download a copy (and the state when the profile may not) | web/src/components/DownloadButton.tsx; PROFILES.md "Downloads", `features.downloads` | Item page, Photos viewer, Shorts | Nothing found. |
| 28 | Save to a list dialog (tick lists, make a new list, Watch later) | COLLECTIONS.md "Collections and smart playlists"; web/src/components/SaveToCollection.tsx | Item page, context menu | Only a toast "Added to Watch later" is drawn. |
| 29 | Collections page and a collection's page: lists you fill and smart playlists, Play all, Edit, Delete, Share with the other profiles | COLLECTIONS.md; routes `/collections`, `/collections/:id` | Collections | Only a library view tab named "Collections" is drawn. |
| 30 | Smart playlist builder with live match preview (libraries, genres, words, years, rating, added in, watched, liked, sort, at most) | COLLECTIONS.md "Making a smart playlist" | Collections (dialog) | Nothing found. |
| 31 | Recommended for you row on Home | COLLECTIONS.md "Recommended for you" | Home | Nothing found. Rows drawn: Continue watching, Recently added, On TV now. |
| 32 | Search: scope and filters, schedule search, empty results | web/src/pages/SearchPage.tsx; CHANNELS.md "The guide" (Search); CLIENT-API.md | Search | Only a result list for the word "harb" is drawn. |
| 33 | Empty and nothing-allowed states: empty library, no results, "this library has nothing you may see", Continue watching with nothing in it | PROFILES.md "What a Kids or Guest profile is shown"; web/src/components/common.tsx (`EmptyState`); CHANNELS.md "Recordings" | Library, Home, Search, Recordings | Nothing found in any look. |
| 34 | Edit dialog for a title: Match, Details, Artwork, Skipping, Organise, Logos, Delete | FIXING-METADATA.md "The Edit dialog"; web/src/components/edit | Dialog over Item page | The context menu has "Edit metadata"; the dialog is not drawn. |
| 35 | Choose the right match: search by name or id, scores with Why?, field-by-field changes, Use this match, Remove this match | FIXING-METADATA.md "Choosing the right match" | Edit dialog | Nothing found. |
| 36 | Fix matches page: Review tabs, scores, Accept, no match, Find suggestions for all, keys | FIXING-METADATA.md "The Fix matches page"; route `/fix-matches` | Library, Settings | Nothing found. |
| 37 | Edit several titles together; Renumber episodes; Naming rules; History and undo | FIXING-METADATA.md; web/src/pages/library/BulkEdit.tsx | Library (select mode) | Nothing found. |
| 38 | Skipping markers editor ("Pick in player") and Find intros and credits with progress | PLAYING.md "Skip intro", "Finding intros nothing marks"; web/src/components/FindIntros.tsx; `POST /shows/{id}/detect-intros` | Show page, Edit dialog | Nothing found. |
| 39 | Libraries management: add a library (kinds), scan now with progress, library settings, read-only, network shares, hide from profiles | LIBRARIES.md "Add a library", "Scanning", "Library settings" | Libraries page | First run draws a folder browser and a list of libraries; the ongoing Libraries page is a Settings page and is not drawn. |
| 40 | Canvas (looping silent video behind a page): where it plays, admin upload or cut from the film with task progress | MUSIC.md "Canvas"; `GET /media/{id}/canvas`, `POST /media/{id}/canvas/generate` | Item page, Album page, full player | Only a caption on the visualizer screen ("Canvas on") mentions it. |
| | **Music, audiobooks, podcasts, radio** | | | |
| 41 | Music library home with cover flip, and Artists, Albums and Playlists tabs, with sort and genre filter | MUSIC.md "Finding your music" | Music | Only the Songs list is drawn. |
| 42 | Album page (Play, Shuffle, Station, Play next, Add to queue, Like all, discs, Other versions) and Artist page (Discography, Singles and EPs) | MUSIC.md "Finding your music"; web/src/pages/music/ArtistPage.tsx | Album and Artist | Nothing found. |
| 43 | Song menu: Play next, Add to queue, Like, Start a station, Add to a playlist, Go to artist, Go to album, Watch the video | MUSIC.md "Finding your music" | Context menu (song) | The context menu drawn is for films only. |
| 44 | Queue editing: drag, jump, Save the queue as a playlist, Clear what's to come | MUSIC.md "Playing and the queue" | Now playing | A "Queue" tab label exists on the karaoke screen; its contents are not drawn. |
| 45 | Sleep timer (15 to 90 minutes, After this song, end of chapter) | MUSIC.md "The full-screen player", "Audiobooks" | Now playing | Nothing found. |
| 46 | Sound panel: No gaps, Crossfade, ReplayGain, ten-band equaliser | MUSIC.md "Sound" | Now playing (More) | Nothing found. |
| 47 | Station from a song, Like, add to playlist, shuffle and repeat states | MUSIC.md "Playing and the queue" | Now playing | Only Play, Prev, Next, Lyrics, Karaoke are drawn. |
| 48 | Read (booklet and text files that came with the album) | MUSIC.md "The full-screen player" | Now playing | Nothing found. |
| 49 | Music videos tab and shelf; Watch the video from a song; Play the song from a video; Tie or untie a song and a video (admin) | MUSIC.md "Music videos"; web/src/pages/music/MusicVideos.tsx, SongVideo.tsx | Music, Now playing, video page | Nothing found. |
| 50 | Lyrics: not found, Add your own, fix wrong lyrics, online lyrics agreement | LYRICS-KARAOKE.md "Adding your own lyrics", "If lyrics are missing or wrong"; web/src/pages/MusicOnline.tsx | Now playing, Settings | Only found, synced lyrics are drawn. |
| 51 | Karaoke preparing and quality states: "Preparing karaoke...", "Preparing a better version... 40%", Quick version or Separated, unavailable message | LYRICS-KARAOKE.md "Karaoke versions, made for you"; CLIENT-API.md `GET /media/{id}/karaoke` | Karaoke | Only the ready state with Key and Mic is drawn. |
| 52 | Visualizer flashing notice with calm mode, effect auto-change timer, More effects, Effect settings and your own effects | MUSIC.md "Visualizer"; WittyVisualizer README | Visualizer | Only Preset and Full screen are drawn. |
| 53 | Audiobooks: Home, Books, Authors, book page with chapters, Continue, back 15 and forward 30, speed 0.75 to 3 | MUSIC.md "Audiobooks" | Audiobooks library | Nothing found. |
| 54 | Podcasts: follow a show (search the directory or paste an RSS address), episodes, Download, Keep downloaded, Check for new episodes | MUSIC.md "Podcasts" | Podcasts library | Nothing found. |
| 55 | Internet radio: stations, Add stations (search, stream address, playlist), Off the air, Try it now, sleep timer, what's on now | COLLECTIONS.md "Internet radio"; route `/radio/:id` | Radio library, Now playing | Nothing found. |
| 56 | Listening party button in the full music player | SYNCPLAY.md "Starting one" (Music) | Now playing | Nothing found. |
| 57 | Audio CD player details: Program order, Hidden track, Index, CD+G pictures, time display modes, About this disc with Sound quality, Find this album | MUSIC.md "Audio CDs"; WittyCD README | Audio CD | Only Play disc, Import to library, Eject and the track list are drawn. CD+G and Hidden track appear only in the matrix. |
| | **Live TV** | | | |
| 58 | Recording choices dialog: Start early, Keep going after it ends, New episodes only, On any channel, Keep newest | CHANNELS.md "Recordings"; web/src/pages/tv/RecordOptions.tsx | WittyTV guide, More info | Recordings list is drawn; the choices are not. |
| 59 | Recording problems: no tuner free when it starts, a live viewer stopped for a recording, disk almost full | CHANNELS.md "Recordings" | Recordings, Player | Nothing found. |
| 60 | Reminder pop-up one minute before a programme, on any page, with Watch | CHANNELS.md "The guide" (Favourites and reminders); web/src/pages/tv/reminders.tsx | Any page | "Remind me" button is drawn; the pop-up is not. |
| 61 | Which channels menu (All, Favourites, Your sets, Shared sets, search) and Add to set (key T) with New set | CHANNELS.md "The guide", "Sets of channels"; web/src/pages/tv/SetPicker.tsx, AddToSet.tsx | WittyTV guide | Sets editor is drawn; the picker and add dialog are not. |
| 62 | Guide search across channels up to two weeks ahead; Find a channel by number or name; dial a channel with 0 to 9 | CHANNELS.md "The guide", "Watching a channel"; web/src/pages/tv/SearchPanel.tsx | WittyTV guide, Player | Day, Filter and Channels 50 are drawn; search and number entry are not. |
| 63 | We'll be right back card when nothing is scheduled | CHANNELS.md "Watching a channel"; web/src/pages/tv/Slate.tsx | Guide preview, Player | Nothing found. |
| 64 | Encrypted channels help page | CHANNELS.md "Encrypted channels"; route `/tv/encrypted` | Live TV | Nothing found. |
| 65 | Keep playing in the background choice | CHANNELS.md "Watching a channel"; web/src/pages/tv/BackgroundPlayChoice.tsx | Live TV player | Nothing found. |
| 66 | Channel editor, own channels, bumpers, commercials, discs on a channel, Who sees it | CHANNELS.md "Making your own channel", "Commercials", "Bumpers", "Who sees a channel"; web/src/pages/tv/ChannelEditor.tsx, BumperEditor.tsx | Channels (Settings page) | Hand-built by decision (matrix); not drawn. |
| 67 | Tuner and IPTV setup, guide data, tuner tools | CHANNELS.md "Tuners and IPTV", "Guide data"; web/src/pages/tv/TunerSetup.tsx | Channels and tuners (Settings page) | First run draws a "What do you have?" picker only. |
| | **Games** | | | |
| 68 | Game PCs: list of PCs, add and pair with a PIN, Wake it up, start or quit a game, Running now: pick up where it is | GAME-RIGS.md "Add and pair a PC", "Play"; routes `/couch`, `/couch/:id`; WittyCouch README | Game PCs page | The Play on screen lists media devices, not PCs. First run draws a module toggle only. The matrix claims a Play on screen for it. |
| 69 | Game PC stream options (Picture, Frames a second, Quality) and in-stream menu (Back to the game, on-screen controller, Leave it running, Close the game, Show the PC's desktop too) | GAME-RIGS.md "Play"; GAMING-STREAM.md | Game PC stream | Nothing found. |
| 70 | Share a PC with everyone; host games from this server (WittyRigs) | GAME-RIGS.md "Share a PC with everyone", "Host games on this server: WittyRigs"; WittyRigs README | Game PCs (Settings page) | Only the matrix text. |
| 71 | Game settings menu: Picture, Sound, Speed and rewind, Saves, Cheats, Shortcuts, On-screen numbers, Core options, Patches, Settings file, Start with default settings | GAMES.md "The game menu and game settings" | In-game menu | The drawn menu has pause, save state, rewind and remap only. |
| 72 | Resume or start over choice, quick save and load, Undo load, "Before starting over" | GAMES.md "Saves, memory cards and save states" | Game page, in-game menu | Resume and slots are drawn; Start over, Undo load and the Saves option are not. |
| 73 | Controller setup page: buttons, profiles (copy, reset, export), players and ports, dead zone, Calibrate sticks, rumble test, turbo, background | GAMES.md "Controllers, touch and keyboard"; route `/controllers` | Controllers | Only a "Remap buttons" button and the phone as controller are drawn. |
| 74 | Arrange touch controls (move, resize, fade; portrait and landscape) | GAMES.md "Touch controls" | Phone controller | The controller is drawn, the arrange mode is not. |
| 75 | Fix this game: Fix match, Files and advanced, download or import a battery save, Settings for this game | GAMES.md "Saves..."; web/src/pages/games/Details.tsx, GameMeta.tsx | Game page | Nothing found. |
| 76 | Games that need attention: missing BIOS, encrypted game, "If a game won't start", plain http hides controllers | GAMES.md "BIOS files", "Encrypted games", "If a game won't start", "Controllers" | Game page | Nothing found. |
| 77 | Standalone emulator games (Switch, 3DS): launch on the machine, running state, ended by screen time | GAMES.md "Standalone emulators"; web/src/pages/games/EmulatorPlay.tsx | Game page | Nothing found. |
| 78 | Stream stats and delay readout (frame rate, encoding time, network) | GAMING-STREAM.md "Stats"; GAMES.md "Streaming and delay" | In-game overlay | Nothing found. |
| 79 | Consoles and cores page: what this server plays, ready or not, core picker, BIOS | WittyPlay `sdui.json` (page "What this server plays"); GAMES.md "Cores" | Games, Consoles | First run draws "Consoles and cores 14 ready" as one row only. |
| 80 | Games library needs a look: review of uncertain matches (admin) | web/src/pages/GameReview.tsx; GAMES.md "Needs a look" | Games (admin) | Nothing found. |
| | **Books and readers** | | | |
| 81 | Books library: covers with progress, Continue reading, filters | BOOKS.md "Reading"; route `/books` | Books | Matrix names "library shelves"; no books screen is drawn. |
| 82 | Book page: Read, Continue reading, Read again, formats, download | BOOKS.md "Reading" | Item page (book) | Nothing found. |
| 83 | Reader Text and display panel: layout (Auto, One page, Book view, Scroll), page colour and warmth, font, text size, line spacing, margins | BOOKS.md "Books (EPUB and converted formats)" | Reader | An "Aa" button is drawn; the panel is not. Typography and night modes appear only in the matrix. |
| 84 | Contents list and slider, footnote pop-up | BOOKS.md "Getting around" | Reader | "Chapter 4" and "Page 84 of 312" are drawn; the contents list is not. |
| 85 | Select-text toolbar: highlight colours, Note, Define (with online notice), Copy, Cite, Search, Read aloud | BOOKS.md "Bookmarks, highlights and notes", "What goes online" | Reader | A Highlight button is drawn; the toolbar is not. |
| 86 | Handwriting: pen, pencil, highlighter, eraser, pressure, undo and redo | BOOKS.md "Handwriting"; WittyInk README | Reader (pen mode) | Only the matrix text. |
| 87 | Read aloud control | BOOKS.md "Read aloud" | Reader | Nothing found. |
| 88 | PDF reader controls: zoom (Fit width, Fit page), pages across, page colour, outline and thumbnails, go to page, search | BOOKS.md "PDFs"; WittyPDF README | PDF | Thumbnails and typography appear only in the matrix. |
| 89 | Comics settings: Page, Spread, Scroll, left to right or manga, Fit height or width | BOOKS.md "Comics and manga"; WittyComics README | Comics | One caption line ("right to left off") is drawn; the settings are not. |
| 90 | Book refused: copy protected, password protected, too large | BOOKS.md "Copy-protected and password-protected books" | Reader, Book page | Nothing found. |
| 91 | WittyReader app: own library, add books, Continue reading, filters, Define toggle, import and export notes | BOOKS.md "WittyReader"; WittyReader README | WittyReader app | Matrix says "same reader screens"; the app's own screens are not drawn. |
| | **Photos** | | | |
| 92 | Photo viewer: zoom, Details (EXIF, camera, place), Download the original, Favourite, Slideshow with timing | PHOTOS.md "The picture viewer"; web/src/pages/photos/PhotoViewer.tsx | Photos viewer | EXIF and viewer appear only in the matrix. |
| 93 | Map view (clusters, list of places, home button, optional online map notice) | PHOTOS.md "The map", "The map and the internet"; web/src/pages/photos/PhotoMap.tsx | Photos, Map | The "Map" tab is a label; the map is not drawn. |
| 94 | Albums (create, Select many, Share with other profiles), Favorites and Folders tabs, People and places (marked Planned) | PHOTOS.md "The screens" | Photos | Only an "Albums" tab label is drawn. |
| 95 | Upload to a library (drop files, quota, refused file) | PHOTOS.md "Uploads"; web/src/components/UploadButton.tsx | Photos | Nothing found. |
| | **Accounts and profiles** | | | |
| 96 | Password or PIN entry on a profile tile, wrong tries wait message, Sign in with another account form (Classic sign-in) | ACCOUNTS.md "Signing in", "Passwords"; PROFILES.md "Switching profile" | Sign-in, Who's watching | Tiles and a Sign in button are drawn; no entry, error or wait state. |
| 97 | Two-factor code entry and recovery code | ACCOUNTS.md "Two-factor sign-in" | Sign-in | Nothing found. (Setting it up is a Settings page.) |
| 98 | Sign in with your phone: TV or web shows a code, or a QR code with a PIN that changes every 30 seconds; approval page (who is asking, from where, Sign it in as, Refuse, admin password again) | ACCOUNTS.md "Signing in with a code"; CLIENT-API.md "With a code", "With a QR code and a PIN"; routes `/pair`, web/src/components/PairDevice.tsx | TV sign-in, Sign in a TV | Only a "Sign in a TV" menu row and a first-run option "Pair an app with a code" exist. |
| 99 | Connect to a server: servers found on the network, type an address, certificate or fingerprint warning, Choose another server, join by code (peer to peer) | APPS.md "How apps find your server", "Typing the address", "If the app warns about the server"; P2P.md "Joining by a code"; web/src/components/ConnectServer.tsx | App first launch | Only the "server isn't set up" screen with "Choose another server" is drawn. |
| 100 | Your devices: list, last used, Sign out, Sign out the others | ACCOUNTS.md "Your devices" | You | A menu row only. |
| 101 | Screen time notices: time left at the foot of the page, time is up with Ask a parent, outside allowed hours or bedtime | PROFILES.md "Screen time" | Any page, Player | Nothing found. |
| 102 | Ask a parent: choose 15, 30 or 60 minutes, parents' PIN or parent's username and password, too many tries | PROFILES.md "Asking a parent for more time" | Dialog | Nothing found. |
| 103 | Parent's report for one profile: Today with Give time, What they watched, What was refused and extra time, Clear | PROFILES.md "What a parent can see" | Profiles (Settings page), You | Only a "Profiles and parental limits" menu row. |
| 104 | Kids and Guest experience: reduced menus, no Settings pages they cannot use, no Sign in a TV, features switched off (games, Live TV, downloads, links, comments, watch parties, away from home) | PROFILES.md "What a Kids or Guest profile is shown", "What a profile can do" | Home and menus for Kids | A Kids label and a "Kids Cove" channel are drawn; no kids-mode home or limited menu. |
| 105 | Signed out because the profile may not be used away from home | PROFILES.md "What a profile can do" (Use away from home) | Sign-in | Nothing found. |
| 106 | Your year: totals, streak, biggest day, months, kinds, top titles; Clear my history | COLLECTIONS.md "Your year"; route `/review` | Your year | Nothing found. |
| 107 | Motion and big screens switches (Moving posters, Moving backgrounds, TV mode) and Skipping switches (Skip intros, recaps, credits, adverts) | PLAYING.md "Moving posters", "Skipping without pressing anything"; SETTINGS.md "Appearance" | Appearance (Settings page) | Look and theme are drawn; these switches are not. |
| 108 | Share dialog: with other profiles, or Make a link with an end date and Allow downloading originals; link shown once | PHOTOS.md "Sharing"; web/src/components/ShareDialog.tsx; SHORTS.md | Photos, Shorts, Item page | Only the word "share" in a caption and a "Sharing settings" row. |
| 109 | Public shared page for a link (no navigation) and Your links with End link | PHOTOS.md "Sharing"; web/src/pages/SharedPage.tsx, route `/s/:token` | Shared page, Sharing (Settings page) | Nothing found. |
| | **Sharing and remote** | | | |
| 110 | Start or join a watch party: Start a watch party, six-letter code, Join, Everyone can play, pause and seek | SYNCPLAY.md "Starting one", "Joining one" | Item page, Player, You menu | Only the status line "Watch party · code HARB-2210 · 3 watching · in step". |
| 111 | Watch party panel: people with states (Ready, Loading, Buffering, Catching up, Away, Can't watch this one), queue, Invite, Wait for everyone, make host, remove, Leave | SYNCPLAY.md "While you watch" | Player (side panel or sheet) | Nothing found. |
| 112 | Watch party invitation card (Accept, Decline) and errors (No such watch party, Too many tries, Only the host controls) | SYNCPLAY.md "Joining one", "When something isn't right" | Any page | Nothing found. |
| 113 | Let other devices play here switch; control devices signed in as others | PLAYING.md "What a browser answers to"; REMOTE-PROTOCOL.md | Play on | The device list is drawn; the switch and the cannot-do messages are not. |
| 114 | Remote access state: reachable or not, test from outside, peer to peer devices and taking one off | REMOTE.md "Testing it from outside", "Peer-to-peer connections"; P2P.md "The server's side"; web/src/pages/settings/RemoteAccess.tsx, PeerToPeer.tsx | Remote access (Settings page) | First run draws the choice (Only at home, Private network, Peer to peer, Port forwarding) only. |
| | **Modules** | | | |
| 115 | Module store: browse, labels, install with what the module asks for, turn off, remove | MODULE-STORE.md "Finding and adding a module", "What a module can ask for"; routes `/settings/modules/store` | Modules (Settings page) | First run draws a module toggle list only. |
| 116 | Updates: update available, automatic updates, choose a version, pin, go back, a version taken back out | MODULE-STORE.md "Updating", "Choosing a version, pinning and going back" | Modules | Nothing found. |
| 117 | Other stores and installing from a file (including keys and fork warnings) | MODULE-STORE.md "Other stores", "Installing from a file" | Modules | Nothing found. |
| 118 | Module missing message where a feature needs a module | web/src/components/ModuleMissing.tsx; MUSIC.md "Visualizer" | Any page | Nothing found. |
| 119 | Module pages drawn from a module's own definition | SDUI-MODULES.md; route `/m/:module/:page` | Module page | Settings and module pages are hand-built by decision; the one WittyPlay consoles page is not drawn. |
| | **Admin and status** | | | |
| 120 | Activity button with progress ring and count, panel with Now and History, Pause, Resume, Cancel, exclamation on a problem | TASKS.md "The activity button", "Pausing and cancelling"; web/src/components/ActivityPanel.tsx | Top bar | Nothing found. |
| 121 | Update notice banner and release notes | web/src/components/UpdateBanner.tsx; SETTINGS.md "About" | Home, Settings | Nothing found. |
| 122 | Backup and restore: make, password, schedule, restore, undo restore, restore during first-time setup | BACKUP.md | Backup (Settings page), first run | First run has no "restore a backup" step. |
| 123 | Repair, Reset, and diagnosing from Settings | REPAIR.md "Repairing from Settings", "Starting again: the Reset page" | Repair, Reset (Settings pages) | Nothing found. |
| 124 | Diagnoser reports for discs and storage | DIAGNOSER.md "Reports" | Diagnoser (Settings page) | Nothing found. |
| 125 | Logs: read, choose how much is recorded, Download logs to share | LOGGING.md "Reading them", "Downloading logs to share"; web/src/pages/settings/logs | Logs (Settings page) | Nothing found. |
| 126 | Tdarr connection, nodes and what it shows in the activity panel | TDARR.md | Transcoding (Settings page) | Nothing found. |
| 127 | Live transcoding and library transcoding: quality modes, hardware encoder choice, presets, progress | TRANSCODING.md; LIBRARY-TRANSCODING.md | Transcoding (Settings pages) | Nothing found. |
| 128 | Legal, Licences and About screens, the notices next to web results and logos, and the acknowledgement | CLIENT-API.md "Legal notices"; web/src/pages/settings/Legal.tsx, Licenses.tsx; METADATA.md "The logo service" | Legal | Nothing found. |
| 129 | Help guides built into the app | web/src/pages/HelpPage.tsx, route `/help` | Help | Nothing found. |
| 130 | Folder browser extras: Places, filter, multi-select, disc folders marked, container permission help | WittyIntegratedFileBrowser README | Folder browser | A path bar and a file list are drawn; the rest is only in the matrix. |
| | **Apps and devices** | | | |
| 131 | Server unreachable, reconnecting, signed out by the server (stream token and sign-in expiry) | TROUBLESHOOTING.md "Connecting"; CLIENT-API.md "Rules every client shares" | Every app | Nothing found. The only offline wording drawn is a friend being offline in WittyPlay. |
| 132 | Keep for offline (downloads) on phone and TV apps, following the Downloads switch | PROFILES.md "What limits can't do" (Downloads); `features.downloads` in `GET /me/limits` | Item page, Downloads list | Nothing found. |
| 133 | Notifications in one place: reminders, watch party invitations, task results, update | CHANNELS.md "The guide"; SYNCPLAY.md "Joining one"; TASKS.md | Notification area | Each is mentioned in its own feature; no notification surface is drawn. |
| | **Other: vertical videos, creator videos and comments** | | | |
| 134 | Vertical videos feed: For You, Following, Liked, Creators; Like, Comments, Share, Speed, About, Edit, why this video is here | SHORTS.md "Watching", "How For You works" | Shorts | Nothing found in any look. |
| 135 | Creator videos: Home, Latest, Subscriptions, Channels, History, Liked; channel page (Videos, Vertical, Playlists, About) | CREATOR-VIDEOS.md "The screens" | Creator videos | Nothing found. |
| 136 | Creator video page: Wide view, Like and see less, Share, Save, chapters, Up next with Autoplay | CREATOR-VIDEOS.md "The screens" | Video page | Nothing found. |
| 137 | Comments with replies (2,000 characters), read-only for Kids and Guest | CREATOR-VIDEOS.md "The screens", "Profiles"; PROFILES.md "Comments" | Video page, Shorts | Nothing found. |
| 138 | Music videos and concerts: Concerts tab, Setlist from chapters, artist as channel | CREATOR-VIDEOS.md "Music videos and concerts" | Music videos | Nothing found. |

## Drawn but incomplete

These have a drawn element, but only in some looks or platforms, or only part of the function.

1. Statistics: widgets are drawn as charts only. Pause, Time range, the Charts or Tables switch, widget details with Save as CSV and JSON, arranging and resetting the layout, and "Not available here" are not drawn (STATS.md "What it shows", "Customising the layout").
2. Notes panel: the list and Export to Markdown are drawn. Filters (Highlights, Notes, Bookmarks, Handwriting), naming colours and the Remove action are not (BOOKS.md "Bookmarks, highlights and notes").
3. WittyTV: the screens board (guide, More info, watching with mini guide) exists in five looks (Lumen, Aero, Classic Cable, Retro, Rail), and the Recordings, Sets and Look boards exist in two (Lumen, Aero).
4. Songs, karaoke and visualizer have boards in five looks (Lumen, Aero, Classic Cable, Retro, Rail); the other looks only reach them through the shared renderer.
5. The player and WittyRemote boards exist in four looks (Lumen, Aero, Classic Cable, Retro).
6. First run is drawn in four looks (Lumen, Aero, Rail, Classic Cable) with 14 steps; the settings, policy and first-use boards exist in two (Lumen, Aero).
7. WittyPlay Online is drawn in its own style plus four themes; in the 17 looks the shared Games screen shows only a "Pick up where you left off" row.
8. Remote on screen: the DVD remote dims keys a disc does not use and the Blu-ray remote dims angle; a plain video's remote with disc keys dimmed is drawn on TV only (PlayerBoard).

## Count

- Gaps in the table: 138 numbered rows. By area: Playback 18, Library and item pages 22, Music and audio 17, Live TV 10, Games 13, Books and readers 11, Photos 4, Accounts and profiles 14 (rows 96 to 109, including sharing links), Sharing and remote 5 (rows 110 to 114), Modules 5, Admin and status 11, Apps and devices 3, Other (vertical and creator videos, comments) 5.
- Drawn but incomplete: 8 items.
- Features checked and found drawn (not listed): home rows and layout editor, library sort and views, item page with Resume and Watchlist, context menu for films, search results, You menu, who's watching, Appearance with look, theme, import and required looks, server name editor, first run in 14 steps, WittyTV guide, More info, watching banner, mini guide, recordings list, sets, WittyTV look, WittyRemote for video and DVD, DVD menu, audio CD, folder browser, songs list, now playing, karaoke, visualizer, WittyPlay Online home, game page, in-game quick menu, play together, controllers, memories, discover, phone controller.
