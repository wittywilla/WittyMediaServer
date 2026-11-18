# The .wittyaudio format

A `.wittyaudio` file holds what people add by hand to music in WittyAudio (the server's
music, audiobooks and radio): who made a song, its words in time (word by word, for karaoke,
and in other languages), how loud it is, where to trim or fade it, an audiobook's chapters,
and pictures. It is the music sibling of [`.wittyinfo`](wittyinfo.md): the same ZIP archive,
`manifest.json`, safety rules, languages, translations and cut check, with `"format":
"wittyaudio"` and music's own scopes and fields. The reader and writer are the same Go package
(`pkg/wittyinfo`), and the same JSON Schema describes both
([wittyinfo.schema.json](wittyinfo.schema.json)).

## Where the files go

| For | Name | Scope |
| --- | --- | --- |
| A song (or an audiobook's file) | the song's name with `.wittyaudio`: `01 Song.flac` → `01 Song.wittyaudio` | `song` (the default) |
| Every song of an album | `album.wittyaudio` in the album's folder | `album` |
| An artist | `artist.wittyaudio` in the artist's folder | `artist` |

A song's own file beats its album's, which beats what the song's tags say.

## The manifest

```json
{
  "format": "wittyaudio",
  "version": 1,
  "scope": "song",
  "language": "en",
  "identity": { "durationSec": 211.4, "fingerprint": { "kind": "sha256-headtail-v1", "value": "…" } },
  "metadata": { "title": "A Song", "year": 2024, "genres": ["Pop"] },
  "audio": {
    "artists": ["Someone", "Someone Else"],
    "album": "An Album", "albumArtist": "Someone",
    "track": 3, "trackTotal": 12, "disc": 1, "discTotal": 1,
    "composers": ["…"], "lyricists": ["…"], "label": "…",
    "isrc": "GB-AAA-26-00001", "barcode": "…",
    "bpm": 120, "key": "A minor", "moods": ["Upbeat"], "explicit": false,
    "loudness": { "trackGain": -6.5, "trackPeak": 0.98, "albumGain": -7.1, "albumPeak": 1.0 },
    "trimStart": 0, "trimEnd": "3:20", "fadeIn": 0, "fadeOut": 4,
    "lyrics": {
      "language": "en",
      "lines": [
        { "start": 12.5, "text": "First line", "words": [{ "start": 12.5, "text": "First" }, { "start": 13.1, "text": "line" }] },
        { "start": "0:16", "text": "Second line", "singer": "Someone Else" }
      ]
    }
  },
  "scenes": [{ "id": "ch-1", "title": "Chapter One", "start": 0 }],
  "segments": [{ "type": "custom", "start": "3:20", "end": "5:00", "label": "Silence before the hidden track", "action": "auto" }],
  "artwork": { "poster": "images/poster.jpg", "back": "images/back.jpg", "artist": "images/artist.jpg" },
  "translations": {
    "es": { "lyrics": { "lines": [{ "start": 12.5, "text": "Primera línea" }, { "start": 16, "text": "Segunda línea" }] } }
  }
}
```

Everything `.wittyinfo` has is here too: `metadata`, `artwork` (with `back` for the back of the
sleeve and `artist` for a picture of them), `identity`, `authorship`, `segments` (parts to
skip), `scenes` (an audiobook's chapters), `notes`, `language`, `translations` and
`extensions`. Content filters and subtitles are for videos only.

### audio

| Field | Meaning |
| --- | --- |
| `artists`, `album`, `albumArtist` | Who and on what. |
| `track`, `trackTotal`, `disc`, `discTotal` | Where on the album (0 to 9999). |
| `composers`, `lyricists`, `label`, `isrc`, `barcode` | Credits and codes. An ISRC is written `GB-AAA-26-00001` (dashes optional). |
| `bpm`, `key`, `moods`, `explicit`, `compilation` | For mixes and filters. |
| `biography` | An artist's, in an `artist` file. |
| `lyrics` | The words: `plain` text, or `lines` in time, each with optional `end`, `singer` and `words` (each word's `start`, for karaoke). `instrumental: true` says there are none. Lines are in time order; 5000 lines and 200 words a line at most. |
| `loudness` | ReplayGain: `trackGain` and `albumGain` in dB (−60 to 60), `trackPeak` and `albumPeak` (0 to 10). |
| `trimStart`, `trimEnd` | Play only this part (a long silence, a hidden track). |
| `fadeIn`, `fadeOut` | Seconds of fade. |

A translation can hold `lyrics` in its language, timed like the song's own (or plain). A
viewer whose language has them gets them; everyone else gets the song's own.

## In WittyMediaServer

- Scans read a song's `.wittyaudio` and its album's: their details replace the tags', and the
  album's cover is the songs' picture.
- **Lyrics** (`GET /api/music/lyrics/{id}`) come from the song's `.wittyaudio` first, in the
  listener's language when it has them, then an `.lrc` beside it, the tags, and online.
- **Loudness** (`GET /api/music/loudness/{id}`) comes from the song's or album's `.wittyaudio`
  when either says.
- **Editing** through the same API as a film's: `/api/wittyinfo/item/{song id}` for a song, and
  `/api/wittyinfo/album/{a song's id}` for its album. Saved beside the music when writing
  `.wittyinfo` files is allowed, else kept by the server. Desktop only, as for films.

## Converting

**Gather** takes a song's `.lrc` (or `.txt`) lyrics into its `.wittyaudio`, and an album
folder's cover (`cover.jpg`, `folder.jpg`, `front.jpg` and the like) and `album.nfo` into
`album.wittyaudio`. **Spread** writes them back out as `.lrc`, `cover.jpg` and `album.nfo`. The
audio files themselves are never changed: tags inside them stay as they are. See
[Converting a library](wittyinfo.md#converting-a-library).
