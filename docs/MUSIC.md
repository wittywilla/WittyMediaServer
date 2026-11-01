# Music, audiobooks and podcasts

Add a library of type **Music** or **Audiobooks** in *Settings → Libraries* (or in the setup
wizard) and point it at your folders.

- [Folder layout](#folder-layout)
- [What is read from your files](#what-is-read-from-your-files)
- [The music pages](#the-music-pages)
- [Playing](#playing)
- [The full-screen player](#the-full-screen-player)
- [Lyrics](#lyrics)
- [Karaoke](#karaoke)
- [Visualizer](#visualizer)
- [Sound](#sound)
- [Formats](#formats)
- [CD images](#cd-images)
- [Podcasts](#podcasts)

## Folder layout

Any layout works as long as the files are tagged. Without tags, folders are used:

```
Music/
  Some Artist/An Album/01 First Song.mp3       artist / album / "track title"
  Some Artist - An Album/01 First Song.mp3     album folder only (artist from the tags)
Audiobooks/
  Jane Author/A Short Book/01 Chapter 1.m4b   author / book / chapter
  Jane Author/Another Book.m4b                one file per book
```

## What is read from your files

| From | Used for |
|---|---|
| Tags (ID3, Vorbis comments, MP4 atoms…) | title, artist, album, album artist, track and disc number, year, genre, comment |
| `cover.jpg`, `folder.jpg`, `front.jpg`, `AlbumArt*.jpg` in the folder | the album cover (the largest one wins) |
| Picture embedded in the file | the cover when the folder has none (extracted once per album) |
| File and folder names | title, track number, album and artist when tags are missing |
| `<song>.lrc` or `<song>.txt` beside a song | its lyrics |
| Lyrics in the tags (ID3 `USLT` and `SYLT`, Vorbis, MP4) | its lyrics, when there's no file beside it |
| `replaygain_*` or `R128_*` tags | volume leveling, if you turn it on |
| `artist.jpg` (or `.png`, `.webp`) in the artist's or the album's folder | the artist's photo |
| PDFs and text files in the album's folder | **Read** in the full-screen player |

Nothing is written into your music folders.

**Audiobooks** use the *album* tag (or the folder) as the book and the *artist* (or the parent
folder) as the author. Each file is a chapter.

## The music pages

A music library opens on its front page: a row of covers to flip through, with your most played at the front, then
**Artists**, **Albums** and **Songs**, each with **See all**. Colours behind them come from the artwork.

- **Play my music** shuffles the whole library.
- **Albums** sort by title, artist, year, date added or most played, either way round, and filter by genre.
- **Songs** have views for **Liked**, **Most played**, **Played lately** and **Newest**, with search, genre and sorting.
- **Artists** have a page of their own, with their songs and discography, and singles and EPs apart.
- An album with more than one disc is divided by disc, and other versions of it are offered below (Deluxe, Remaster).
- **Playlists** are [collections](COLLECTIONS.md), so a smart playlist made there is here too.
- Any song's **⋯** menu has Play next, Add to the queue, Like, Start a station and Add to a playlist.
- **Station** makes a queue from one song, using only your library: the same artist, genre and era, leaving out what you've disliked.

Audiobooks and podcasts have the same front page, with **Continue listening**.

## Playing

The bar floats along the bottom while you browse, and the page leaves room for it so it never hides anything. Its arrow
collapses it to a small button in the corner. On a phone it's art, title, play and next, above the tab bar.

**What's playing comes back after a reload,** stopped where it was, with the queue, shuffle and repeat as they were. It's kept on
that device for each person, for 30 days, and nothing is fetched until you press play. Stop and close forgets it.

- **Music**: Play, Shuffle, Add to queue, repeat one/all. Shuffle really reorders the queue, so what's shown is what will play.
- **Audiobooks and podcasts**: they continue where you stopped (on every device), **speed** from 0.75× to 3× with the voice
  pitch kept, **back 15 s / forward 30 s**, and a **sleep timer**.
- Lock screens, headphone buttons and keyboard media keys control playback, with artwork and a progress bar on the lock screen.

## The full-screen player

Press the artwork or **Full screen** in the bar. **F** then fills the whole screen. It's the same for music, audiobooks,
podcasts and radio, with what doesn't apply left out.

**Keys:** Esc close, Space play or pause, ← → previous or next, L lyrics, K karaoke, Q queue, V visualizer, S shuffle,
R repeat, M mute, F fill the screen, `,` and `.` back and forward 10 seconds. Up or Down moves onto the controls, and arrows then move between them,
so a remote or a controller reaches everything.

**Top row:** volume, add to a playlist, sleep timer, queue, like, fill the screen, and **⋯** for sound settings, a station, and the
album and artist.

**The queue** can be edited as it plays: drag a row, or use its buttons from a keyboard or remote. Save it as a playlist, or clear
what's to come.

**Read** shows the words at your own pace, and any PDF or text file in the album's folder.

## Lyrics

**Lyrics** (or L) follow the music: the line being sung stays in the middle, and pressing any line goes to it. Words without
times read as plain text.

They're looked for in this order:

1. A correction made on this server.
2. `<song>.lrc` or `<song>.txt` beside the song.
3. The song's own tags.
4. Online, **only if the owner has turned it on** in first-time setup or *Settings → Metadata agents*.

**Online sources:**

| Source | Cost | What it's told |
|---|---|---|
| [LRCLIB](https://lrclib.net) | Free, with no key or account | The artist, title, album and length of a song, when someone opens its lyrics |
| Your own LRCLIB server | Free | The same, and it stays on your network |
| [Musixmatch](https://developer.musixmatch.com) | Your own key; lyrics need a paid plan | The artist, title and length |

Never a file name, or who's listening. Answers are kept in the server's cache folder, so they don't go into backups of its data,
and aren't passed on to anyone. *Forget them* in Settings clears them. The words belong to their writers.

**If the timing is off:**

- **Sooner** and **Later** under the lyrics move them for you alone, and the setting stays with that song.
- An admin can press **Fix** to correct them for everyone: choose another version, paste or edit them, or use **Tap along**, which plays
  the song from the start while you press as each line begins. None of this writes anything beside your music.

A `.lrc` file is plain text, with word timing if you want it:

```
[00:12.50]A line, with the time it starts
[00:16.00]<00:16.00> Each <00:16.40> word <00:16.90> timed
```

## Karaoke

**Karaoke** (or K) is a big display for singing: words that fill in as they're sung, a three-second count before a line after a break,
and the next line waiting below. It needs timed lyrics, and without timing it says so.

- **Voice lower** and **Voice lowest** turn the singer down. **This has limits.** It works by removing what's in the middle of the stereo mix,
  which is where most singers are. Echoes of the voice remain, songs that spread the voice wide keep more of it, and anything else in the
  middle (often a snare or a solo) goes down with it. Bass is kept. A mono recording has nothing to separate, and the screen says so.
- **Key** moves the music up to six semitones either way, for the singer, without changing the speed. Long held notes can warble a little at large
  shifts. Both go back to normal when you leave.
- **Words** moves the timing if it's slightly off.
- The fill across a line is exact for lyrics with word timing, and otherwise an estimate from the line's length. It says which.

**Keys:** Esc leave, Space play or pause, ← → previous or next line, V voice, − + key, `[` `]` timing, F fill the screen.
Up or Down moves onto the controls for a remote.

## Visualizer

**In the full-screen player the visualizer is on by default**, behind the artwork, with the big **Visualizer** button (or V) to turn it off.

- **Bars, Waves and Drift** are soft shapes drawn from the music in the artwork's colour. They don't flash, so they need no warning.
- **More** has every effect from [WittyVisualizer](https://github.com/wittywilla/WittyVisualizer). The notice about flashing comes first, the first time.
- **Slow** is a slower, gentler pace for whichever one is showing. Devices set to reduce motion start that way.
- **It never changes by itself unless you ask.** To have it change, choose an interval under the shuffle button: every 15 seconds up to every 10 minutes,
  among Bars, Waves and Drift, or among every effect.
- It's one canvas, and it stops when the page is hidden.

If the sound can't be read (a server on a different address, or an iPhone, which offers no way to do it that survives a locked screen), the shapes
move slowly on their own, not in time with anything, and that's all they are pretending to do.

## Sound

Under **⋯ → Sound**, kept on each device, because speakers differ:

- **No gaps** starts the next song right as this one ends. How close it gets depends on the file: formats like MP3 have a little silence built in at each end.
- **Crossfade** from 2 to 12 seconds. Songs of the same album aren't crossed unless you say, as they're often meant to run together.
- **ReplayGain** levels volume using tags in your files. Songs without tags play as they are.
- **Equaliser**: ten bands, presets, and an overall level.

On iPhones and iPads, the equaliser and leveling stop when the screen locks, because those devices stop this kind of audio then. Playing without them keeps going.
With a server on a different address from the page, the browser doesn't allow them at all, and the panel says so.

## Formats

MP3, AAC/M4A, FLAC, Opus, Ogg Vorbis and WAV are sent to the browser as they are (instant
seeking, no server work). ALAC, WMA, APE, WavPack, AIFF, DSD and others are converted to AAC
while playing — a light job even for small servers.

## CD images

Keep images of your audio CDs in a **Music** library and they become albums, with their tracks in the Songs list and
search like any others. Keep them in a **Discs** library (**DVDs, Blu-rays & CDs**) and they sit on the shelf with your
other discs. Either way they play in the CD player, a window in the spirit of the desktop players of the early 2000s: the disc
goes in, spins behind a window on a dark stage, and a lit display shows the track, index and time beside the list of tracks.
Press the time to change what it shows. **Visualizer** (or V) puts the light shows on the stage instead of the disc.

**Which should I use?** Music, unless you want your CDs on the same shelf as your films. An album from a CD works like
every other album, and pressing **Insert disc** brings up the player.

**Formats:** `.cue` with `.bin`, `.wav`, `.aiff` or one `.flac`; `.toc`; `.ccd` with `.img`; `.nrg`; `.chd`; and a bare `.iso`,
`.img` or `.bin` that is really audio, which plays as one long track as nothing says where the others begin. A sheet beside
already tagged per-track files, or files in formats such as MP3, is left alone, and those stay ordinary songs.

**Better than the real thing:**

- **No gaps, ever.** The whole disc is sent as one continuous stream, so a live album or a segued record plays exactly as
  it was laid out, pregaps included. The display counts down through a pregap, as a good player does.
- **Lossless,** as FLAC, or as compressed AAC if a connection can't keep up: that switch is automatic, and can be set by hand
  under *About this disc*. Limits for people away from home apply as usual.
- **Your place on the disc** is kept for each person, on every device.
- **Repeat one or all, shuffle, and your own program,** all without a join between repeats.
- **Hidden tracks:** press Previous at the start of track 1 to rewind into audio hidden before it, or pick it from the list.
- **Pre-emphasis,** a treble boost on some early CDs, is corrected as a good player would.
- **Index points** and **CD+G** karaoke pictures, from a `.cdg` file beside the image.
- The music keeps playing while you browse, in the bar along the bottom, with lock screen and media key controls.

**Names and covers.** A disc's own text comes first (CD-TEXT or the sheet), then its folder and file names. Each new disc is
also looked up on MusicBrainz, using only its table of contents: see [Metadata](METADATA.md#audio-cds). A cover beside the
image always wins. If several albums match, the disc keeps its own names and offers a **Choose from N matches** button.

**Keys:** Space play or pause, N and P next and previous, J and L back and forward 10 seconds, S stop, R repeat, H shuffle,
V visualizer, E eject. Remotes and controllers move around the deck as they do everywhere else.

**A damaged image** plays as far as it goes, and says so. To find out what is wrong with one:

```bash
wms cd check "Album.cue"
```

**Copy protection:** plain CD audio doesn't have any. An image whose audio is scrambled sounds like noise and is reported as
such, and nothing in this project unscrambles it. DTS surround discs are reported too.

## Podcasts

Add a library of type **Podcasts** (a folder is optional: it is only used for downloads), open
it and press **Add podcast**: search by name (results from the Apple Podcasts directory) or
paste a feed address. New episodes are picked up every 2 hours, or with **Check for new
episodes** on the podcast's page.

- Episodes play **through your server**: your devices never contact the podcast's host, and
  old `http://` feeds work on secure pages. Seeking works as usual.
- **Download** keeps an episode in the library folder (`<folder>/<podcast>/<date> <title>.mp3`);
  **Keep downloaded: newest N** does it automatically.
- **Unfollow** removes the episodes that were not downloaded; downloaded files stay.
- Episodes marked explicit are rated TV-MA, so Kids and Guest profiles don't see them.

For safety, feeds and episodes are only ever fetched from the internet: the server refuses to
connect to itself or to devices on your local network on a feed's behalf.

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
