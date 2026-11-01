# Metadata, NFO files and artwork

WittyMediaServer reads and writes metadata in the standard **NFO and artwork** formats that most other media software
already use, so you can move an existing library in — or out — without re-tagging anything.

- [Where metadata comes from](#where-metadata-comes-from)
- [What is saved next to your media](#what-is-saved-next-to-your-media)
- [Editing and fixing matches](#editing-and-fixing-matches)
- [Fixing metadata by hand](FIXING-METADATA.md): a folder of plain text files, one for each title, to edit in any text editor
- [Fix matches: the review list](#fix-matches-the-review-list)
- [Names that say what a file is](#names-that-say-what-a-file-is)
- [Categories, studios and browsing](#categories-studios-and-browsing)
- [Streaming listings](#streaming-listings)
- [Coming from another media server](#coming-from-another-media-server)
- [The logo service](#the-logo-service)
- [Video games](#video-games)

## Where metadata comes from

Sources are applied in this order; later ones only fill gaps:

| # | Source | Example |
|---|---|---|
| 1 | `<file>.metadata.json` next to the file | `{"title":"My Film","year":2001,"overview":"…","genres":["Animation"],"poster":"https://…"}` |
| 2 | NFO file | `My Film (2001).nfo`, `movie.nfo`, `tvshow.nfo`, episode `<name>.nfo` |
| 3 | Local artwork | `poster.jpg`, `<name>-poster.jpg`, `folder.jpg`, `fanart.jpg`, `season01-poster.jpg`, `<episode>-thumb.jpg` |
| 4 | File and folder names | `My Film (2001).mkv`, `Show - S01E02 - Title.mkv`, `MY_FILM.ISO` (disc label) |
| 5 | **Custom agents** you added, in their order (movies, home videos, books) | anything your own service knows: see [Custom agents](#custom-agents) |
| 6 | **TMDB** (movies) / **TheTVDB** (TV), if you added a key | overview, genres, rating, network, posters, backdrops, episode titles |
| 7 | **TVmaze** (TV) until you add a TheTVDB API key — built in, no API key needed | overview, genres, network, status, posters, backdrops, season posters, episode titles and stills |
| 8 | **Wikidata** + Wikipedia (movies) until you add a TMDB API key — built in, no API key needed | year, genres, US rating, runtime, plot summary (no posters) |
| 9 | **AniList** (Anime libraries) — built in, no API key needed, no switch of its own | titles in your chosen language, plots, posters, episode numbering |
| 10 | Generated thumbnail | a frame 20 % into the video |

NFO and metadata files have to be plain files of at most 4 MB. One that is a link, or larger, is ignored.

Audio CDs have an order of their own: see [Audio CDs](#audio-cds).

**Works out of the box.** TV shows and movies get information without any account or key.
TVmaze data is shared under CC BY-SA 4.0, Wikidata is public domain, Wikipedia text is CC BY-SA
4.0; title pages credit their source. Turn either off in *Settings → Metadata agents*. Adding a TMDB or
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
still empty: information inside your files, NFO files and your own edits always win. They're also
searched in **Search and match**, where an agent's answer can be chosen like any other. **Try it** in the
agent's dialog looks a title up without changing anything.

The agent answers with JSON, either one object, a list, or `{"results": [...]}` (the first entry
is used). Every field is optional:

```json
{
  "id": "lx-2012",
  "title": "My Film",
  "year": 2012,
  "overview": "A boy searches for the one thing that will win the girl of his dreams…",
  "tagline": "Meet the original force of nature.",
  "genres": ["Animation", "Family"],
  "rating": 6.4,
  "contentRating": "PG",
  "studio": "Illumination",
  "runtimeMinutes": 86,
  "poster": "https://archive.local/art/my-film.jpg",
  "backdrop": "https://archive.local/art/my-film-wide.jpg"
}
```

Answer `404` when you don't know a title. Pictures are downloaded when the title is scanned: a
picture on the agent's own server is fetched like the agent (it may be on your network); any other
picture is fetched from the internet only. Only real pictures (JPEG, PNG, WebP, GIF) are kept.

## What is saved next to your media

Your metadata is in the server's database, and comes along in a [backup](BACKUP.md). 
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

Anything read back from a file beside your media, your own or anyone else's, is limited in size and cleaned to plain text: no HTML, no scripts, no hidden
characters, and pictures that are only web addresses or files in that folder. A file that can't be read is left out with a warning in *Settings → WittyLog*
naming the file and the line. [How it's kept safe](FIXING-METADATA.md#how-its-kept-safe).

Scans only write metadata for items that have no NFO yet; your own NFOs are never touched by a
scan, and if you edit an item whose NFO you wrote by hand, the original is kept as `<name>.nfo.bak`.
Mount media read-only (Docker `:ro`) and turn the option off if you prefer the server not to write.


## Editing and fixing matches

**Edit** on a film, episode or show opens one dialog with everything about it, in sections you can jump between. Delete, Cancel
and Save stay in view however long it runs. *Ctrl+S* saves, and *Escape* closes, asking first if something isn't saved. Selecting text
and letting go outside the dialog doesn't close it.

### Search and match

Type a name, or paste an id or address from TMDB, IMDb, TheTVDB, TVmaze, AniList or Wikidata: `tt0000123`, `tmdb:12`,
`https://www.tvmaze.com/shows/56/a-show`. Every source that's on is asked at once, films and shows alike, so a disc of a series can be matched
to the series. Each result says how likely it is out of 100, and **Why?** says why in plain words: the name, the year, whether it's the right kind of
thing for the library.

Choosing a result changes nothing yet. It shows **what would change**, field by field, with what's there now beside what's coming. Untick anything you want to keep.
Text you typed, a hand-written NFO and pictures beside the file are shown but not ticked; locked fields can't be ticked. For a show, it also says how
many of your episodes the source knows.

A match you choose **stays**: no scan or refresh changes it, even if the file is replaced by a better copy. **Remove this match** takes away what the source gave
and keeps what you typed; the title is named after its file again and nothing is looked up for it until you choose one. **Check for changes**, and **Refresh** on
a title's page, show what looking it up again would change. Both keep the match it has.

### Locks, sources and Revert

Each field says where its value came from: *You typed this*, *From TMDB*, *From an NFO file*, *Saved earlier* (an NFO this server wrote), *From the file name*.

| | |
|---|---|
| **Lock** | A locked field stays as it is. No scan, refresh, new match, NFO or sidecar can change it, whether the title is matched or not. Anything you type locks itself. **Lock all** does every field |
| **Revert** | Goes back to what the source said before you typed over it, and unlocks the field |
| **Undo** | Every match, edit and renumbering can be undone: right after it, or later under *Fix matches → History*. The last 5,000 changes are kept |

### Artwork

Poster, backdrop, banner and title logo each have **Browse images**, a grid from everywhere with a tick on the chosen one, and **Upload your own** (JPEG, PNG, WebP or GIF, up to 20 MB; it's checked to
be a real picture, and kept in the image cache). Pictures from sources are fetched by your server, so no browser or app contacts them. Without a TMDB key, films have only what's in their folder:
Wikidata has no posters, and TV shows get theirs from TVmaze.

With *Save information and artwork next to your media* on, the chosen poster and backdrop are also copied to the standard names beside the file, and the NFO is rewritten (a hand-written one is kept as `.nfo.bak`).

### Skip intro

Where the intro ends, as `M:SS`, or **Pick in player**: play to the moment and press the button. Blank means work it out from the file's chapters. On a show it covers every episode with no intro of its own. Discs have their own menus, so this isn't offered for them.

### Bonus features and versions

**It's a bonus feature** takes a trailer, featurette, deleted scene, interview or short out of the lists and puts it on the page of the film or show it belongs to. **This is another version of…** does the same for a second copy of a film, such as a DVD and a 4K
file, which its page then offers as a choice. Either can be undone with one button: *Make it a title of its own*, or *Split it off*.

### In the wrong library?

**Move to** puts a title in another library that holds the same kind of thing. Its files don't move: the library it was found in leaves it alone from then on, and the new one looks after it. Where it can't move, it says why.

**Delete** takes a title out of its library and leaves its files on disk. It won't come back on a scan, and can be brought back from *Fix matches → Removed*. Nothing here ever deletes a file.

## Fixing by hand

Prefer a text editor, or have a lot of small fixes to make? *Settings → Repair → Metadata files* writes a plain text file for each title, shows you what your edits
would change, and applies them, locked and undoable like anything typed into **Edit**. The guide, with examples, is [Fixing metadata by hand](FIXING-METADATA.md).

## Fix matches: the review list

*Fix matches*, on a library's page and in *Settings → Metadata agents*, lists every title in film, disc and TV libraries, worst first. Opening it asks nobody anything: how sure each match is comes from what's stored, comparing the file's name and year with what it's matched to.

| Tab | What's there |
|---|---|
| **Needs a look** | No match, or one under 70% |
| **No match**, **Not sure**, **Looks right** | Those on their own |
| **You chose**, **Left alone** | What you've decided, which nothing changes |
| **Removed** | Titles taken out of a library, to bring back |

Choosing a title finds suggestions, one request per source; **Find suggestions for all** does the whole list in the background, gently. If a file's whole name finds nothing, its first words are dropped and it's tried again, as in `Dr Story The Treeline` for *The Treeline*.

| Key | |
|---|---|
| `J` `K` | Next and last |
| `A` | Accept the first suggestion |
| `1` to `6` | Accept that suggestion |
| `N` | It has no match |
| `E` | Edit details |
| `X` | Choose it, for doing several at once |
| `/` | Search |
| `U` | Undo |

The arrow keys and a remote reach everything on the list and in the edit dialog: moving down the list follows along in the panel beside it, and OK on a title goes to its first suggestion. A year you typed, or one kept from before, counts when scoring, so an exact name and year comes to 100%. **Choose the 4 that are 90% sure** ticks those at once, for you to accept.

With several chosen, **Accept their best suggestions** or **Edit together**, which adds or takes away genres and tags, sets a year or a rating, and locks or unlocks. **Renumber episodes** shows each file beside what the source calls that number, with ways to move a whole season. Numbers you change are locked, so a scan keeps them.

## Names that say what a file is

An id in a file or folder's name wins over any search, and doesn't appear in the title. The usual forms all work: `My Film (2001) [tmdbid-123].mkv`, `{imdb-tt0000123}`, `[tvdb-456]`, `[tvmaze-78]`, `[anilist-90]`.

A folder with a naming of its own can have **rules**, under *Fix matches → Naming rules*. A rule says how a path reads, using `{title}`, `{year}`, `{edition}`, `{tmdb}`, `{imdb}`, `{tvdb}`, `{any}`, and `*` for anything within a folder's name. For example, `Harbour Studio/{year} - {title}`. It can also say where to look (or not to look it up at all), add tags, or mark what it fits as a bonus feature. The first rule that fits decides. **Try them** shows what would happen without changing anything. Rules apply to new and changed files when a library is scanned.

## Categories, studios and browsing

A film or TV library opens on **shelves**: categories, collections, studios, networks, recently added, highest rated, and a row for each category. Each row asks for its titles only as it comes near the screen. Choosing a sort, a filter or a card turns it into a grid, and the address holds the whole state, so any view can be bookmarked.

**Categories** are made from genres, one library at a time, with the art of what's in them. Different spellings of a genre are put together (*Sci-Fi* and *Science Fiction*), and combined ones split (*Action & Adventure*). **Categories** on the page lets an administrator rename, hide, reorder and merge them. Only the changes are kept, so new genres appear by themselves, and *Start again* forgets them.

**Collections from film series** appear by themselves. When a source says a film is part of a series, and two or more films in a library share it, they show as a card under *Collections*. The **Collection** field in *Edit details* changes or clears it. Nothing is created in the *Collections* side menu.

**Studios and networks** come from metadata, every studio a source names. A card opens everything from that studio, across libraries. Kids and Guest profiles see neither the titles they can't watch nor a studio or category only those titles are in.

**Filters** cover type, category, studio, network, decade, year, age rating, score, picture (4K, 1080p, 720p, SD), sound (`AC3 5.1`), audio and subtitle language, edition, tags, whether it's been watched, and words in the title. **Save this view** keeps a view for your account, and can pin it to Home.

## Streaming listings

Optional, and off until an administrator accepts each statement of what it means, in *Settings → Metadata agents*. It shows which of the titles **already on your server** a streaming service currently lists, for someone used to finding things that way.

**Nothing plays from those services, no account with them is used, and there's no connection with any of them.** Every screen that shows a listing says so, and dates it.

| | |
|---|---|
| **Source** | TMDB's watch providers, which come from [JustWatch](https://www.justwatch.com). TMDB's terms require JustWatch to be credited, which every screen does, and don't allow commercial use. Answers are kept for about two weeks; TMDB's terms allow up to six months |
| **Needs** | Your own free TMDB key. Without one it stays off, because no source of this data works without a key and allows this use |
| **Sent** | The TMDB id of each title in your film and TV libraries, or its name and year to find that id, about once every two weeks. A title that can't be found with near certainty isn't listed, because a wrong guess would say something is on a service when it isn't |
| **What counts** | Included with a subscription, free, or free with adverts. Rentals and purchases aren't what's on a service |
| **Region** | Your choice, as listings differ by country. Changing it looks them up for that country |
| **Kept up to date** | A title you match, rematch or unmatch is looked at again straight away, and its old listings go. **Check now** looks at everything again |
| **Names and logos** | Plain text unless you've agreed to logos too |

Turning it off forgets everything it fetched.

## Coming from another media server

- **Software that uses NFO files**: point a library at the same folders. Existing NFOs
  and artwork are read as-is, including TMDB/TheTVDB ids, so matches are exact.
- **Servers that keep metadata in their own database**: local artwork (`poster.jpg`,
  `fanart.jpg`) is still read, and WittyMediaServer matches by the usual folder/file naming
  (`Movie (Year)`, `Show/Season 01/Show S01E01`). After a scan with metadata saving on, your
  folders gain NFOs that other NFO-based tools can read too.

## The logo service

Logos for studios, networks, consoles, game studios and streaming services, for every part of the app. **Nothing is bundled
and nothing is downloaded until an administrator agrees**, in setup or in *Settings → Metadata agents*. Until then, and for
anything with no logo, the app shows the name in plain text, and that's a finished look rather than a gap.

### Where logos come from, and the terms

| | |
|---|---|
| **Source** | [Wikidata](https://www.wikidata.org) says which file is a company's logo (its *logo image* statement). The data is CC0. The file itself is on [Wikimedia Commons](https://commons.wikimedia.org/wiki/Commons:Licensing), where each file's page states its licence |
| **What's taken** | Only public domain files and free licences that allow changes and commercial use (`CC0`, `CC BY`, `CC BY-SA`). Anything marked non-free, non-commercial or no-derivatives is skipped |
| **Credit** | Source, licence and author are kept with each logo and listed under *Settings → Metadata agents → Logos*, which is what `CC BY` and `CC BY-SA` ask for. The white and dark versions are recoloured, and the list says so |
| **Trademarks** | A free picture licence says nothing about trademark. Logos are still their owners' trademarks and appear only to name who made something. The `logos` notice appears beside them |
| **Sent** | The name, and nothing else: nothing about who asked or what's in your files. About one request a second |
| **Why not TMDB's logos** | TMDB's API terms don't allow derivatives, and logos have to be recoloured to read on any background. Its data isn't used for this at all |

A logo is used without asking only if the name on Wikidata (or one of its other names) is the name in your library. Anything
close but not exact is only offered by **Change**, where a person chooses.

### What happens to a picture

It's read, limited to 8 MB and 25 megapixels, and written again from its pixels as a plain PNG, so nothing but pixels gets
through: no SVG, no scripts, no metadata, no data on the end. Then two more versions are made: all white and all dark. If a logo
has two clear tones, such as light letters on a dark shield, the larger tone is painted and the smaller is cut out, so the letters
stay readable. A picture whose background can't be taken off stays as it is and gets a plate of its own (`opaque`).

Pictures are kept in the image cache under `logos/`. They're served only by this server, so no browser or app contacts the source.

### Using it in the web app

```tsx
import BrandLogo from '../components/BrandLogo'

<BrandLogo kind="network" name={show.network} />            // follows the theme
<BrandLogo kind="studio" name={it.studio} on="dark" />       // on a plate that's always dark, such as over artwork
```

It renders the logo, or the name in text, on its own. Lookups in the same moment go as one request. On a screen showing logos, also
show `<LegalNote kind="logos" />` once. `useLogo(kind, name)` and `lookupLogo(kind, name)` give the details, and `forgetLogos()` drops
what's been remembered after a change.

Kinds: `studio`, `network`, `console`, `gamestudio`, `platform`.

### Using it from the server

When you find a new console or game studio, call `Store.EnsureLogo(ctx, kind, name)`. It only adds a row; `Logos.Kick()` starts looking, and does
nothing without the owner's agreement. The studios of films and networks of shows are added by scans already, and so are the consoles of a games library and the studios of its games.

### Consoles and game studios

`GET /api/games/consoles` gives each console a `logoName`, the name it goes by on Wikidata, which is often not the one on its card. On a games screen, use the marks made for this, which are the same size
whichever way a console appears. With no logo they're a short typed label, so a screen looks finished before anyone has agreed to logos, or if a console has none:

```tsx
import { ConsoleMark, StudioMark } from '../components/ConsoleMark'

<ConsoleMark console={c} />                     // sizes: sm, md, lg
<StudioMark name={it.studio} on="dark" />
```

### Routes

| Route | Who | What it does |
|---|---|---|
| `GET /api/logos/status` | Signed in | `{consent: "yes"\|"no"\|"", source, sourceUrl}`, and for an administrator `counts: {pending, found, none, text}` |
| `PUT /api/logos/consent` `{accepted, removeDownloaded}` | Admin | Records the answer. Agreeing starts the downloads. Declining stops them, and removes what was downloaded if asked. Uploaded logos stay |
| `POST /api/logos/lookup` `{logos: [{kind, name}]}` | Signed in | Up to 200 names at once. Answers `{logos: [{id, kind, name, status, has, version, opaque, source, sourceUrl, license, licenseUrl, credit}]}`. An admin's lookup adds names nobody has asked about; anyone else's only reads, because a looked-up name goes to the internet |
| `GET /api/logos/{id}/image?variant=white\|dark\|original&v={version}` | Signed in | The PNG, or 404 |
| `GET /api/logos/credits` | Signed in | Every logo with its source, licence and author. An account with limits gets the studios and networks of what it may see |
| `GET /api/logos/{id}/candidates?q=` | Admin | What the source offers, to choose from. 409 without the owner's agreement |
| `GET /api/logos/preview?src=` | Admin | A candidate's picture, through this server. Only Wikimedia addresses, and 409 until downloads are agreed to |
| `PUT /api/logos/{id}` `{sourceId, q}` or `{status: "text"\|"pending"}` | Admin | Use a candidate; or the name alone; or look again |
| `PUT /api/logos/{id}/upload` (the picture as the body) | Admin | Use your own. PNG, JPEG, WebP or GIF, up to 8 MB. Needs no agreement, since nothing is downloaded |
| `GET /api/brands?library=&kind=studio\|network` | Signed in | The studios or networks of a library, most titles first, counting only what this viewer may see |

`status` is `pending` (not looked for yet), `found`, `none` (no source has one) or `text` (someone chose the name alone).

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
2. **By code.** A disc's product code is read from a few sectors near its start, even inside a `.cso`, or from its file name (PlayStation,
   PlayStation 2, PSP, GameCube, Wii, Saturn, Dreamcast, Mega-CD). A compressed disc such as a `.chd` goes by name. A cartridge's code, and the title in its header, count
   only when the file's name agrees, because a fan's hack of a game keeps both: it isn't that game, and shouldn't be named after it. A file called `rom.z64` has nothing to disagree with.
3. **By name and console.** Region and dump tags are taken off and kept as details. Punctuation, articles and II or 2 don't
   matter, but different numbers always do. A name must be at least 85% alike to be used without asking, and only an exact name or a checksum renames a game: a close match
   gets its details, but keeps the name on its file, as it may be a variant.
4. **By another version in its archive.** In a set, what's asked about first is the file that would be played. If neither that nor the name is known, a checksum of another version
   in the archive still gives the game's details and art. Its name might be another region's, so unless the names agree the game keeps the one on its file.

A file saved here earlier (an NFO this server wrote, and the pictures beside it) is only an old answer, which a better one replaces. An NFO a person wrote is theirs, and stays.

A checksum found by one source gives the exact name to ask the others about, so they match better too. Every game's page
shows administrators how it was matched and how sure that is, with **Fix match** (search every source and pick), **Lock**
(leave it alone from now on) and **Look up again**. **It has no match** clears what sources filled in.

A game from a gaming PC has no file to read, so it goes by name alone, on sources that can search PC games.

### Keeping requests down

The queue is in the database, so a restart carries on where it stopped. The local database is asked about every game first, which takes seconds and no
requests beyond fetching each console's list once. Pictures and online sources then go side by side, so a slow source holds nothing else up. Games are worked on in batches of 50, one console
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

### The developer id

ScreenScraper's terms identify each program by a developer id, which isn't in this repository. A copy needs its own unless
one was built in. Without one the source says so, and asks for one in Settings; other sources aren't affected.

Official releases don't have one built in today. `scripts/build-release.sh` builds one in when `SCREENSCRAPER_DEV_ID` and
`SCREENSCRAPER_DEV_PASSWORD` are set, and the release workflow sets those from secrets of the same names. The Docker image
isn't built with them, because build arguments stay readable in an image.

## Lyrics and artist information

Music can also find lyrics, and artists' biographies and photos, online. Both are off until the owner turns them on, in first-time setup or
*Settings → Metadata agents*. What's sent to whom is set out there and in [Music](MUSIC.md#lyrics).

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
