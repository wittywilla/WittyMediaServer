# The .wittyinfo format

A `.wittyinfo` file holds what people add by hand to a film, an episode, a season or a
whole show: where the intro, recap, credits and other skippable parts are, scenes with
titles, content filters, corrected details, pictures, subtitles and how it plays best. It
sits beside the media, so it moves with the files and works on any server that reads it.

WittyMediaServer reads and writes it, and so does the `wittyinfo` program (see
[The wittyinfo program](#the-wittyinfo-program)). The reference reader and writer is the Go
package `pkg/wittyinfo`; the manifest's JSON Schema is
[wittyinfo.schema.json](wittyinfo.schema.json).

- [Where the files go](#where-the-files-go)
- [The archive](#the-archive)
- [The manifest](#the-manifest)
- [Languages and translations](#languages-and-translations)
- [Which cut it was made for](#which-cut-it-was-made-for)
- [What wins](#what-wins)
- [Safety](#safety)
- [In WittyMediaServer](#in-wittymediaserver)
- [Converting a library](#converting-a-library)
- [The wittyinfo program](#the-wittyinfo-program)
- [Changing the format](#changing-the-format)

## Where the files go

| For | Name | Example |
| --- | --- | --- |
| A film | the video's name with `.wittyinfo` in place of its extension | `Movie (2023).mkv` → `Movie (2023).wittyinfo` |
| An episode | the same | `Show S01E02.mkv` → `Show S01E02.wittyinfo` |
| A disc folder | the folder's name with `.wittyinfo` added, beside it | `Movie (2023)/` → `Movie (2023).wittyinfo` |
| Every episode of a season | `seasonNN.wittyinfo` in the show's folder, or `season.wittyinfo` in the season's own folder | `Show/season01.wittyinfo` |
| Every episode of a show | `show.wittyinfo` in the show's folder | `Show/show.wittyinfo` |

`Movie (2023).mkv.wittyinfo` (the whole file name with `.wittyinfo` added) is read too.

A season or show file holds defaults: "the intro is about 0:45–1:30 in every episode". An
episode's own file beats them, and can say an episode has no intro at all (see `absent`).

## The archive

A `.wittyinfo` is a ZIP archive:

```text
Movie (2023).wittyinfo
├── manifest.json          required: everything below, as JSON
├── images/
│   ├── poster.jpg         pictures the manifest names (JPEG, PNG or WebP)
│   ├── backdrop.jpg
│   └── scenes/
│       └── 01.jpg
└── subtitles/
    └── en.forced.srt      subtitle files the manifest names (SRT, WebVTT, ASS, SSA)
```

- `manifest.json` is UTF-8 JSON. Writers put it first and store it uncompressed, so a reader
  can get it without unpacking anything else.
- Pictures are stored as they are (they are compressed already); other members are deflated.
- Any other member is allowed (for example a tool's own state, under a folder named for the
  tool). Readers keep it when they save the file again and never use it.
- A bare manifest (the JSON alone, not in a ZIP) is read too, so a file can be written by
  hand in a text editor. Writers always write the archive.

To edit by hand, unpack it (`wittyinfo unpack`, or any ZIP tool after renaming it to
`.zip`), change `manifest.json`, and pack it again (`wittyinfo pack`).

## The manifest

```json
{
  "format": "wittyinfo",
  "version": 1,
  "scope": "item",
  "identity": {
    "durationSec": 6312.48,
    "fileName": "Movie (2023).mkv",
    "fileSize": 4823145672,
    "edition": "Theatrical",
    "fingerprint": { "kind": "sha256-headtail-v1", "value": "9f2c…" }
  },
  "authorship": {
    "author": "Willow",
    "created": "2026-11-18T20:15:00Z",
    "updated": "2026-11-18T20:40:12Z",
    "tool": { "name": "WittyMediaServer", "version": "0.8.1-alpha" },
    "license": "CC0-1.0",
    "source": "Checked by hand against the Blu-ray."
  },
  "segments": [
    { "id": "seg-1", "type": "recap", "start": 0, "end": 42.5 },
    { "id": "seg-2", "type": "intro", "start": "0:45", "end": "1:30", "action": "auto" },
    { "id": "seg-3", "type": "custom", "start": 2710, "end": 2745, "label": "Dream sequence" },
    { "id": "seg-4", "type": "credits", "start": "1:42:10", "end": "1:47:30" },
    { "id": "seg-5", "type": "postcredits", "start": "1:47:30", "end": "1:48:20" }
  ],
  "absent": ["preview"],
  "scenes": [
    { "id": "scene-1", "title": "Opening", "start": 0 },
    { "id": "scene-2", "title": "The heist", "start": "12:00", "thumbnailAt": "12:05" },
    { "id": "scene-3", "title": "Finale", "start": "1:30:00", "image": "images/scenes/03.jpg" }
  ],
  "filters": [
    { "category": "language", "severity": 2, "start": 600, "end": 601.4, "action": "mute" },
    { "category": "violence", "severity": 3, "start": 3020, "end": 3044, "action": "skip" }
  ],
  "metadata": {
    "title": "Movie",
    "sortTitle": "Movie",
    "edition": "Theatrical",
    "year": 2023,
    "overview": "…",
    "genres": ["Drama"],
    "externalIds": { "tmdb": "12345", "imdb": "tt0000000" }
  },
  "artwork": { "poster": "images/poster.jpg", "backdrop": "images/backdrop.jpg" },
  "playback": { "startAt": 8, "audioLanguage": "jpn", "subtitleLanguage": "eng", "subtitleMode": "always" },
  "subtitles": [{ "file": "subtitles/en.forced.srt", "language": "en", "forced": true }],
  "notes": "Anything people want to say about it.",
  "extensions": { "mytool.state": { "lastTab": 2 } }
}
```

Every field but `format` and `version` is optional. Times are seconds (`85.5`), or a clock
string (`"1:25"`, `"1:02:03.5"`) for files written by hand; writers write seconds to the
millisecond.

### Top level

| Field | Meaning |
| --- | --- |
| `format` | Always `"wittyinfo"`. |
| `version` | `1`. A reader uses what it knows of a newer version and keeps the rest. |
| `scope` | `item` (one film or episode; the default), `season` or `show`. |
| `season` | The season number, for a season file. |
| `identity` | Which video it was made for: see [Which cut](#which-cut-it-was-made-for). Item files only. |
| `authorship` | Who made it, when (RFC 3339), with what tool, under what licence (an SPDX id), and where the times came from. |
| `segments` | Parts of the video to skip or know about. |
| `absent` | Segment types this video has none of: defaults and automatic detection aren't used for them. |
| `scenes` | Named points, shown in the player's Scenes. Item files only. |
| `filters` | Content a viewer may want skipped or muted. Item files only. |
| `metadata` | Details that replace what the server found. |
| `artwork` | A picture in the archive for each role. |
| `playback` | How it plays best. |
| `subtitles` | Subtitle files in the archive. |
| `notes` | Free text. |
| `language` | The language the file's own text is in (labels, titles, details, notes). |
| `translations` | The same text in other languages: see [Languages and translations](#languages-and-translations). |
| `extensions` | A tool's own data, under a name of its own (`tool.key`). Kept, never used. |

Fields this version doesn't know, at any level, are kept when the file is saved again and
otherwise ignored.

### Segments

| Field | Meaning |
| --- | --- |
| `id` | Optional; writers fill one in. Up to 64 letters, digits and `_ . : -`, unique in the file. |
| `type` | `intro`, `recap` (earlier episodes), `credits`, `preview` (of the next episode), `postcredits` (a scene after the credits), `commercial` (an advert break left in a recording) or `custom` (anything else). |
| `start`, `end` | Where it is. Leave `end` out for a part that runs to the end of the video. |
| `label` | What to call it ("Dream sequence"): the skip button says "Skip dream sequence". |
| `action` | `button` (offer a skip button; the default), `auto` (skip it by itself, if the viewer allows that), `mark` (only show where it is; the default for `postcredits`). |
| `note` | Free text. |

A video has one intro, recap, credits, preview and post-credits scene (the first of each
counts, except credits, where the last does) and any number of advert breaks and custom
parts.

A post-credits scene is something not to miss: when there is one, skipping the credits
lands on it instead of going on to the next episode.

### Scenes

| Field | Meaning |
| --- | --- |
| `title` | Its name. |
| `start` | Where it starts. |
| `thumbnailAt` | The moment to picture it by (default: its start). |
| `image` | A picture in the archive to use instead (`images/scenes/03.jpg`). |
| `description` | Free text. |

A file with scenes replaces the video's own chapters in the player. To keep the chapters,
copy them in (the editor has a button for that).

### Filters

| Field | Meaning |
| --- | --- |
| `category` | `violence`, `language`, `nudity`, `sex`, `drugs`, `frightening`, `flashing` (flashing lights), `discrimination` or `other`. |
| `severity` | `1` mild, `2` moderate, `3` strong (`0` or left out: not said). |
| `start`, `end` | Where it is. |
| `action` | What the file suggests: `skip`, `mute` or `warn`. |
| `label` | What it is, briefly. |

A filter only says what is there. Each person chooses in their own settings what to skip or
mute, so one file serves a whole household: a child's profile can skip what an adult's
plays. (Profiles aren't named in the file: names differ from home to home.)

### Metadata

`title`, `originalTitle`, `sortTitle`, `edition`, `year`, `releaseDate` (YYYY-MM-DD),
`overview`, `tagline`, `genres`, `tags`, `contentRating`, `studios`, `countries`,
`languages`, `collection`, `episodeKind` (`canon`, `filler`, `mixed` or `recap`: mostly for
anime), `externalIds` (`{"tmdb": "…", "imdb": "…", "tvdb": "…"}`) and `trivia`. Each one
given replaces what the server found; those left out don't.

### Artwork

`{"role": "images/name.ext"}` for `poster`, `backdrop`, `logo`, `thumb` (an episode's
picture), `banner`, `clearart`, `disc` and `square`. Each names a picture in the archive.

### Playback

| Field | Meaning |
| --- | --- |
| `startAt` | Where playing starts the first time (past a studio logo, say). |
| `audioLanguage`, `subtitleLanguage` | The tracks to choose (ISO 639 codes). |
| `subtitleMode` | `off`, `forced` (only forced subtitles) or `always`. |
| `volumeDb` | A loudness change, −30 to 30 dB, for a quiet or loud file. |
| `aspectRatio` | How to show the picture (`16:9`, `2.39:1`), for a file that says it wrong. |

Players follow what they can; the rest is a preference, not a promise. WittyMediaServer
follows all of them for every player: it starts the first play at `startAt` (carrying on, or
choosing a time, wins), picks the sound and subtitle tracks by language when the viewer hasn't
(the viewer's own choice wins, then the file's, then the server's), and follows `subtitleMode`
(`forced` picks a forced track in the sound's language). `volumeDb` and `aspectRatio` are passed
to the player, which applies them.

### Subtitles

`{"file": "subtitles/en.srt", "language": "en", "title": "…", "forced": false, "sdh": false}`.
UTF-8 text in SRT, WebVTT, ASS or SSA. WittyMediaServer offers each as a track after the
video's own and the files beside it, converted to WebVTT like any other.

## Languages and translations

Every language a file mentions is a language tag in any of the forms media carries: an ISO
639-1 code (`de`), an ISO 639-2 code in either form (`ger`, `deu`), an ISO 639-3 code (`yue`,
`fil`) or a BCP 47 tag with a script or region (`pt-BR`, `zh-Hant`, `sr-Latn`, `es-419`). These
are the languages WittyMediaPlayer, the apps and the browsers name: every ISO 639-1 language
and any well formed code beyond them. Writers put each in its usual form (`ger` is written
`de`, `pt_br` is `pt-BR`). This holds for `language`, `playback.audioLanguage` and
`subtitleLanguage`, `subtitles[].language`, `metadata.languages` and the keys of
`translations`.

`translations` holds the file's text in other languages, by language tag. Each names the parts
it translates by their `id` (writers give every part one), so a translation stays with its
part however the parts move:

```json
"language": "en",
"segments": [{ "id": "seg-3", "type": "custom", "start": 2710, "end": 2745, "label": "Dream sequence" }],
"scenes": [{ "id": "scene-2", "title": "The heist", "start": "12:00" }],
"translations": {
  "de": {
    "segments": { "seg-3": { "label": "Traumsequenz" } },
    "scenes": { "scene-2": { "title": "Der Raub" } },
    "metadata": { "title": "Der Film", "overview": "…" }
  },
  "pt-BR": { "segments": { "seg-3": { "label": "Sequência do sonho" } } },
  "ja": { "scenes": { "scene-2": { "title": "強盗" } }, "notes": "…" }
}
```

| In a translation | Translates |
| --- | --- |
| `segments.<id>` | `label`, `note` |
| `scenes.<id>` | `title`, `description` |
| `filters.<id>` | `label` |
| `metadata` | `title`, `originalTitle`, `sortTitle`, `edition`, `overview`, `tagline`, `collection`, `genres`, `tags`, `trivia` |
| `notes` | The notes |

Anything a translation leaves out stays in the file's own language. A reader picks the
translation closest to the viewer: the same tag, else the same language whatever its region or
script (`pt-PT` gets `pt-BR` when that is all there is, and `pt` before either), else the file's
own text. A translation that names a part the file doesn't have, or two keys that are the same
language (`de` and `ger`), make the file unusable, like any other mistake. Up to 200 languages.

WittyMediaServer gives each person the text in their language: `?lang=` on an address, else the
languages their browser or app asks for (`Accept-Language`), else the language the server is
set to. The library's titles and overviews follow the server's language.

## Which cut it was made for

Times made for the theatrical cut are wrong for the extended one. `identity` says which
video a file was made for:

| Field | Meaning |
| --- | --- |
| `durationSec` | The video's length. Required in `identity`. |
| `toleranceSec` | How far the length may differ and still be the same cut. Default: 2 seconds, or 0.5 % of the length when that is more. |
| `fileName`, `fileSize`, `edition` | For people: what it was made from. |
| `fingerprint` | `{"kind": "sha256-headtail-v1", "value": "…"}`: SHA-256 of the file's size in decimal, a newline, its first MiB and its last MiB. It names one exact file cheaply. |

A reader compares a file with the video it is about to be used on:

| Result | When | Used? |
| --- | --- | --- |
| `exact` | The fingerprint is the same. | Yes |
| `same-cut` | The length is within the tolerance (another encode or a remux). | Yes |
| `unchecked` | The file has no identity, or the video's length isn't known. | Yes |
| `other-cut` | The length is further off. | **No**: flagged, so someone can look. |
| `forced` | `other-cut`, but someone said to use it anyway. | Yes |
| `defaults` | A season or show file. | Yes |

## What wins

For each kind of segment, the most trusted source that has one gives it:

1. The video's own `.wittyinfo` (the server's copy, else the file beside the media), when it
   fits the cut. Its `absent` list settles those kinds too.
2. Times set by hand in the older "Skip times" editor.
3. A `<file>.skip.json` beside the video.
4. A `<file>.edl` beside the video.
5. The season's `.wittyinfo`.
6. The show's `.wittyinfo`.
7. The video's chapter names.
8. What comparing the season's episodes by ear found.
9. The intro set for the whole show.

Season and show files sit above what is guessed (7–9) and below what was set for the one
video (1–4). Advert breaks and custom parts come whole from one source, never mixed.

The editor shows each part with where it came from, and the item page shows a "Custom
info" badge when a `.wittyinfo` is in use.

For metadata and pictures, the video's own file beats everything the server found, when it
fits the cut. A show's `show.wittyinfo` does the same for the show: its title, overview, year,
genres, rating, poster and backdrop.

## Safety

A `.wittyinfo` is a file anyone could have left beside the media, so readers trust none of it:

- **Sizes.** 64 MB for the whole file, 512 members, 1 MB for the manifest, 16 MB for a
  picture (8192 pixels across at most), 2 MB for a subtitle file, 4 MB for any other
  member, 128 MB unpacked in all, and no member over 1 MB packed more than 100 to 1.
  1000 segments, 1000 scenes, 5000 filters, 50 entries in a list, 200 characters for a
  title or label, 20 000 for an overview or notes, JSON nested 16 deep.
- **Names.** Members are plain relative names (letters, digits, space and `. _ -`, three
  folders deep at most). No `..`, no absolute paths, no backslashes, no name twice. No
  links, no encryption, only stored or deflated members. A manifest field that names a
  member names one inside the archive, in the right folder: never a path or a URL.
- **Content.** Pictures are checked by decoding their header and must be what their
  extension says. Subtitles must be UTF-8 text. Text fields can't hold control characters.
- **Nothing runs.** Nothing in a `.wittyinfo` is fetched, followed, opened or executed. Only
  pictures are ever served from one, with their own image type.
- **All or nothing.** A file with any problem isn't used at all, and every problem is listed
  with where it is (`segments[2].end: has to come after the start`).

## In WittyMediaServer

- **Reading.** The server reads a video's `.wittyinfo` when it scans and whenever the
  markers or the player ask (a changed file is read again), and the season's and show's
  with it.
- **Editing.** On an item's page, ⋯ → **Edit scenes and skips**, or the same in the player.
  Show and season defaults are edited from the show's page. Editing is on desktop only: in the
  web app on a computer and in the desktop app. TV mode and TV apps don't offer it: a remote
  can't place a time to the frame, and a TV's hardware isn't made for scrubbing through a film
  while editing, so they say where to do it instead. Files made anywhere play everywhere.
- **Saving.** An edit is written beside the media when **Settings → Metadata → Save
  metadata beside media** and **Write .wittyinfo files** are both on (`saveMetadata` and
  `saveWittyinfo`). Otherwise the server keeps it, and **Export** downloads it to put
  there by hand. A copy kept by the server wins over the file beside the media; **Remove
  the server's copy** goes back to the file.
- **Import and export** for any film, episode, season or show.
- **The API** is in [CLIENT-API.md](../CLIENT-API.md#media-extras) (`/api/wittyinfo/...`),
  and markers and playback answers carry `segments`, with each one's `action` and `source`.

## Converting a library

A library that already has NFO files and pictures beside its titles can be turned into
`.wittyinfo` files in one go, and back:

- **Gather into .wittyinfo files** takes, for each film and episode, its NFO's details, its
  pictures (poster, backdrop, logo, clear art, banner, disc art; an episode's thumbnail), the
  skips in a `.skip.json` or `.edl` and, if asked, its text subtitle files (SRT, WebVTT, ASS,
  SSA). For each show it takes `tvshow.nfo` and the show folder's pictures into `show.wittyinfo`,
  and each season's poster into `seasonNN.wittyinfo`.
- **Write NFO files and pictures from .wittyinfo files** does the reverse: NFO files and
  pictures in the usual layout, a `.skip.json` for the skips, and the subtitles named as
  players read them (`Film.de.forced.srt`), in the server's language.

What a `.wittyinfo` already says is never replaced by what lies beside it. The old files stay,
unless **Rename the old files to .bak** is ticked: each one converted from is then renamed with
`.bak` on the end, which no scan reads and anyone can rename back. Read-only libraries are left
alone. It runs as a task (Tasks shows how far it has got, and can stop it), after any scan of
those libraries has finished.

It is offered in first-run setup (**Posters and details**), when the libraries just added have
something to convert, and at any time in **Settings → Metadata → Convert a library**. The API
is `GET /api/wittyinfo/convert/counts` and `POST /api/wittyinfo/convert`.

## The wittyinfo program

`wittyinfo` (in `cmd/wittyinfo`) works on files without a server:

```bash
wittyinfo new "Movie (2023).mkv"          # start one, with its identity worked out
wittyinfo show "Movie (2023).wittyinfo"   # what it holds, in words
wittyinfo check "Movie (2023).wittyinfo"  # check it; lists every problem
wittyinfo unpack "Movie (2023).wittyinfo" folder/
wittyinfo pack folder/ "Movie (2023).wittyinfo"
wittyinfo image "Movie (2023).wittyinfo" poster poster.jpg
wittyinfo match "Movie (2023).wittyinfo" "Movie (2023).mkv"
wittyinfo edit "Movie (2023).mkv"         # the editor, in a browser, on this computer only
```

`wittyinfo edit` serves the same editor the server has, on `127.0.0.1` with a one-time key
in the address, and plays the video for the live preview when the browser can.

## Changing the format

- New optional fields can be added in version 1: older readers keep and ignore them.
- A change that older readers would get wrong raises `version`. Readers still read what they
  know of a newer file, but never drop what they don't understand when saving.
- New segment types, filter categories and artwork roles need a new version, because older
  readers refuse values they don't know.
