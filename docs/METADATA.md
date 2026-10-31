# Metadata, NFO files and artwork

WittyMediaServer reads and writes metadata in the standard **NFO and artwork** formats that most other media software
already use, so you can move an existing library in — or out — without re-tagging anything.

- [Where metadata comes from](#where-metadata-comes-from)
- [What is saved next to your media](#what-is-saved-next-to-your-media)
- [Choosing artwork](#choosing-artwork)
- [Editing by hand](#editing-by-hand)
- [Coming from another media server](#coming-from-another-media-server)
- [Video games](#video-games)

## Where metadata comes from

Sources are applied in this order; later ones only fill gaps:

| # | Source | Example |
|---|---|---|
| 1 | `<file>.metadata.json` next to the file | `{"title":"Shrek","year":2001,"overview":"…","genres":["Animation"],"poster":"https://…"}` |
| 2 | NFO file | `Shrek (2001).nfo`, `movie.nfo`, `tvshow.nfo`, episode `<name>.nfo` |
| 3 | Local artwork | `poster.jpg`, `<name>-poster.jpg`, `folder.jpg`, `fanart.jpg`, `season01-poster.jpg`, `<episode>-thumb.jpg` |
| 4 | File and folder names | `Shrek (2001).mkv`, `Show - S01E02 - Title.mkv`, `SHREK.ISO` (disc label) |
| 5 | **Custom agents** you added, in their order (movies, home videos, books) | anything your own service knows: see [Custom agents](#custom-agents) |
| 6 | **TMDB** (movies) / **TheTVDB** (TV), if you added a key | overview, genres, rating, network, posters, backdrops, episode titles |
| 7 | **TVmaze** (TV) until you add a TheTVDB API key — built in, no API key needed | overview, genres, network, status, posters, backdrops, season posters, episode titles and stills |
| 8 | **Wikidata** + Wikipedia (movies) until you add a TMDB API key — built in, no API key needed | year, genres, US rating, runtime, plot summary (no posters) |
| 9 | Generated thumbnail | a frame 20 % into the video |

Audio CDs have an order of their own: see [Audio CDs](#audio-cds).

**Works out of the box.** TV shows and movies get information without any account or key.
TVmaze data is shared under CC BY-SA 4.0, Wikidata is public domain, Wikipedia text is CC BY-SA
4.0; each title page links to its source. Turn either off in *Settings → Metadata agents*. Adding a TMDB or
TheTVDB key later upgrades titles the next time they are scanned or refreshed.

TVmaze and Wikidata do not always have a **content rating**, which Kids and Guest profiles rely
on. Set one under **Edit → Content rating** on any movie or show (a show's rating covers its
episodes).

## Audio CDs

CD images have their own order: **the file beside the image** (`<name>.wittycd.json`, saved earlier), then **CD-TEXT and the
cue sheet**, then **folder and file names**, then **MusicBrainz**, which fills in album and track names and a cover from the
**Cover Art Archive** when it has one clear answer. Pictures beside the image always win.

**What is sent.** To look a disc up, the server sends MusicBrainz the disc's ID and its table of contents, which is the
track count and each track's start in sectors. It says nothing about what the music is, what your files are called or who
you are. No audio, names or folders leave the server. Covers are fetched by the server, so nothing else learns your
address. The lookup happens once for each new disc, at most one request a second as their rules ask, and it can be turned off
in *Settings → Metadata agents*. With it off, nothing is sent.

**Several matches.** Pressings of one album share an answer, so any will do. If the matches disagree, or only the track lengths
are close, nothing is guessed: the disc keeps its own names, and an admin sees **Choose from N matches** on the CD player page.
Pick the right one, change it later, or go back to the disc's own names.

**Saved beside your files?** Only with *Save metadata beside media* on: a `<name>.wittycd.json` with the names and release, and
a `<name>-poster.jpg`. With it off nothing is written there, and the names stay in the server's database.

Album data comes from [MusicBrainz](https://musicbrainz.org) (CC0), and covers from the
[Cover Art Archive](https://coverartarchive.org); cover pictures belong to their owners. Both are credited on the player page.

## Custom agents

A custom agent is a web service of your own that the server asks about titles the built-in agents
don't know: a family archive, a club's catalogue, a script you wrote. Add one in
**Settings → Metadata agents → Custom agents**.

| Field | What it is |
|---|---|
| **Address** | A URL template. `{title}`, `{year}` and `{type}` (`movie`, `video` or `book`) are filled in, e.g. `https://archive.local/lookup?title={title}&year={year}` |
| **API key header** | Optional, sent with every request, e.g. `Authorization: Bearer abc123` (stored on the server, never shown again) |
| **Use it for** | Movies, Home videos and/or Books libraries |

Agents run in the order listed, before TMDB, TheTVDB, TVmaze and Wikidata, and only fill in what is
still empty: information inside your files, NFO files and your own edits always win. When an
agent answers, **Refresh** on a title uses it instead of the built-in agents. **Try it** in the
agent's dialog looks a title up without changing anything.

The agent answers with JSON, either one object, a list, or `{"results": [...]}` (the first entry
is used). Every field is optional:

```json
{
  "id": "lx-2012",
  "title": "The Lorax",
  "year": 2012,
  "overview": "A boy searches for the one thing that will win the girl of his dreams…",
  "tagline": "Meet the original force of nature.",
  "genres": ["Animation", "Family"],
  "rating": 6.4,
  "contentRating": "PG",
  "studio": "Illumination",
  "runtimeMinutes": 86,
  "poster": "https://archive.local/art/lorax.jpg",
  "backdrop": "https://archive.local/art/lorax-wide.jpg"
}
```

Answer `404` when you don't know a title. Pictures are downloaded when the title is scanned: a
picture on the agent's own server is fetched like the agent (it may be on your network); any other
picture is fetched from the internet only. Only real pictures (JPEG, PNG, WebP, GIF) are kept.

## What is saved next to your media

With *Settings → Metadata agents → Save information and artwork next to your media* on (the
default), the server writes these files in the standard layout other media servers read too:

| Media | Written |
|---|---|
| Movie / disc image in its own folder | `<name>.nfo`, `poster.jpg`, `fanart.jpg` |
| Movies sharing a folder (e.g. a folder of ISOs) | `<name>.nfo`, `<name>-poster.jpg`, `<name>-fanart.jpg` |
| `VIDEO_TS` folder | `movie.nfo`, `poster.jpg`, `fanart.jpg` inside the movie folder |
| TV show | `tvshow.nfo`, `poster.jpg`, `fanart.jpg`, `season01-poster.jpg`… in the show folder |
| Episode | `<name>.nfo` (`<episodedetails>`), `<name>-thumb.jpg` |

Artwork saved this way is always a real picture from the internet, the image cache or your own
disk: a sidecar file or a feed can't make the server copy another kind of file here, or fetch
something from your local network.

Scans only write metadata for items that have no NFO yet; your own NFOs are never touched by a
scan, and if you edit an item whose NFO you wrote by hand, the original is kept as `<name>.nfo.bak`.
Mount media read-only (Docker `:ro`) and turn the option off if you prefer the server not to write.


## Choosing artwork

**Artwork** on a movie, disc, episode or show page opens a picker with every candidate: images
already in the folder, TMDB posters/backdrops (movies) and TheTVDB artwork (shows). The chosen
image is copied to the standard file name above, so other servers use it too. You can also paste
any image URL or file path.

## Editing by hand

**Edit** on any page changes title, year, overview, genres, content rating, poster and backdrop. With metadata
saving on, the NFO is rewritten at once (a hand-written NFO is kept as `.nfo.bak` the first time).

## Coming from another media server

- **Software that uses NFO files**: point a library at the same folders. Existing NFOs
  and artwork are read as-is, including TMDB/TheTVDB ids, so matches are exact.
- **Servers that keep metadata in their own database**: local artwork (`poster.jpg`,
  `fanart.jpg`) is still read, and WittyMediaServer matches by the usual folder/file naming
  (`Movie (Year)`, `Show/Season 01/Show S01E01`). After a scan with metadata saving on, your
  folders gain NFOs that other NFO-based tools can read too.

## Video games

Games get their details from their own set of sources, in the background, so a scan of thousands of games
never waits on the internet. It works out of the box; a free key adds more.

### Sources

Choose which are on, and which is believed first, in **Settings → Metadata agents → Video games**. Keys can be
added there or in the setup wizard, which can be skipped. The first source with a value for something wins,
except for scores, age ratings and pictures, which are collected from all of them.

| Source | Needs | Good for | Sends | Licence |
|---|---|---|---|---|
| **libretro database** | Nothing | Recognising your exact files by checksum; dates, makers, genres, players, some age ratings and scores | Nothing about your games: a list for each console you have is downloaded once, checked monthly, and matched here | CC BY-SA 4.0 |
| **libretro thumbnails** | Nothing | Box art, title screens, in-game pictures | Game names, as the database lists them | None stated; the pictures belong to their publishers. Never bundled, only fetched to your server |
| **Wikidata and Wikipedia** | Nothing | Summaries, makers, and some review scores and age ratings | Which console, and the names of games it matches | CC0; Wikipedia text is CC BY-SA 4.0 |
| **IGDB** | A free Twitch application's id and secret | The fullest source: covers, art, critic and user scores, every age rating board | Game and console names | IGDB's terms: free for non-commercial use |
| **ScreenScraper** | A free account. Official releases carry a developer id; a copy you built yourself needs its own | Recognising files by checksum, with art, scores and age ratings | Names, console, checksums and sizes | CC BY-NC-SA 4.0, for free software only |
| **RAWG** | A free key | Metacritic and user scores, wide backdrops | Game and console names | RAWG's terms: free with a link back, which the game page gives |
| **TheGamesDB** | A free key | Box art and fan art, some checksums | Names, console and checksums | No written terms; credited and kept on your server |

**Files never leave your server.** Only names, checksums and sizes are looked up, as the table says. Turning a
source off stops it being asked. Pictures are fetched by the server and kept in its image cache, so a browser
or an app never talks to a source.

### Left out, and why

| Source | Why |
|---|---|
| MobyGames | No free access since 2025, and little it would add to the others |
| Giant Bomb | Its limits and terms couldn't be confirmed from its own pages. Nothing here depends on it |
| PCGamingWiki | Its data query needs a login since August 2026, and its licence is non-commercial with share-alike |
| OpenVGDB | No licence at all, and no release since 2021 |
| Other published data files, including TheGamesDB's own dump | Downloadable, but no terms could be found that allow this use |
| GameTDB | Its files say not to use them on a website without asking. It's worth asking, and it could be added |
| ArcadeDB | For arcade games, which game libraries don't have yet. Its author asks developers to get in touch first |
| Hasheous | Useful as a way between sources' ids, but it would send a checksum of every file to one more place, and it adds no data itself |

### How a game is matched

1. **By checksum.** Archives record their files' checksums (`.zip`, `.7z` and `.gz`), so only the headers are read and
   nothing is unpacked. A cartridge file up to 128 MB is read once, with and without a copier's header. Nothing is
   read twice unless the file changes.
2. **By disc code.** A disc's product code is read from a few sectors near its start, or from its file name (PlayStation,
   PlayStation 2, PSP, GameCube, Wii, Saturn, Dreamcast, Mega-CD). A compressed disc such as a `.chd` goes by name.
3. **By name and console.** Region and dump tags are taken off and kept as details. Punctuation, articles and II or 2 don't
   matter, but different numbers always do. A name must be at least 85% alike to be used without asking, and only an exact name or a checksum renames a game: a close match
   gets its details, but keeps the name on its file, as it may be a variant.

A checksum found by one source gives the exact name to ask the others about, so they match better too. Every game's page
shows administrators how it was matched and how sure that is, with **Fix match** (search every source and pick), **Lock**
(leave it alone from now on) and **Look up again**. **It has no match** clears what sources filled in.

A game from a gaming PC has no file to read, so it goes by name alone, on sources that can search PC games.

### Keeping requests down

The queue is in the database, so a restart carries on where it stopped. Games are worked on in batches of 50, one console
at a time. Each source's answer is kept, so changing the order of sources asks nobody anything, and adding one only
asks that one, for what it can add.

| Source | How it's asked |
|---|---|
| libretro database | One download a console, then no requests at all |
| Wikidata | One query for a console's whole list; then facts 50 games a request and summaries 20 games a request, one request a second at most. Backs off when it says it's busy |
| libretro thumbnails | Four pictures a second at most. Only covers and one in-game picture are fetched up front; others when someone looks |
| IGDB | Three requests a second |
| ScreenScraper | One every 1.2 seconds, up to 18,000 a day |
| RAWG | Two a game, up to 18,000 a month. Only for games still missing something |
| TheGamesDB | One a second, for games still missing something, stopping when its monthly allowance runs low |

### Age ratings and Kids profiles

ESRB, PEGI, CERO, USK, ACB, GRAC, ClassInd and ELSPA ratings all become a minimum age. The strictest one a game has is its
content rating, written like `ESRB T`, and that's what Kids and Guest profiles go by, as for a film. A game above a
profile's limit doesn't appear at all, and can't be opened or played. A game with no rating counts as unrated, so it's
hidden from profiles that don't allow unrated titles: set one with **Edit** on its page. Ratings a source marks as still
pending count as no rating.

### What's kept, and what's saved beside your games

Answers go in the server's database and pictures in its image cache. With *Save information and artwork next to your media*
on, a game gets `<name>.nfo`, `<name>-poster.jpg` and `<name>-fanart.jpg` beside it, and a scan reads them back. With it
off, nothing is ever written next to your games. Your own files always win: a picture or NFO beside a game, and anything
typed under **Edit**, stays as it is.

### For anyone building their own copy

ScreenScraper's terms identify each program by a developer id, which isn't in this repository. Release builds set it with
`-ldflags "-X github.com/wittywilla/wittymediaserver/internal/gamemeta.DevID=... -X github.com/wittywilla/wittymediaserver/internal/gamemeta.DevPassword=..."`.
Without one the source says so, and asks for one in Settings; other sources aren't affected.

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
