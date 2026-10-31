# Music, audiobooks and podcasts

Add a library of type **Music** or **Audiobooks** in *Settings → Libraries* (or in the setup
wizard) and point it at your folders.

- [Folder layout](#folder-layout)
- [What is read from your files](#what-is-read-from-your-files)
- [Playing](#playing)
- [Formats](#formats)
- [Podcasts](#podcasts)

## Folder layout

Any layout works as long as the files are tagged. Without tags, folders are used:

```
Music/
  Adele/21/01 Rolling In The Deep.mp3          artist / album / "track title"
  Adele - 21/01 Rolling In The Deep.mp3        album folder only (artist from the tags)
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
