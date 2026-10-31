# Music, audiobooks and podcasts

Add a library of type **Music** or **Audiobooks** in *Settings → Libraries* (or in the setup
wizard) and point it at your folders.

- [Folder layout](#folder-layout)
- [What is read from your files](#what-is-read-from-your-files)
- [Playing](#playing)
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

Nothing is written into your music folders.

**Audiobooks** use the *album* tag (or the folder) as the book and the *artist* (or the parent
folder) as the author. Each file is a chapter.

## Playing

The player bar stays at the bottom while you browse; tap the cover for the full *Now playing*
view with the queue.

- **Music**: Play, Shuffle, Add to queue, repeat one/all.
- **Audiobooks**: books continue where you stopped (on every device), **speed** from 0.75× to
  3× with the voice pitch kept, **back 15 s / forward 30 s**, and a **sleep timer** (15, 30,
  45, 60 minutes or the end of the chapter).
- Lock screens, headphone buttons and keyboard media keys control playback.

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
