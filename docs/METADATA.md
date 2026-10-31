# Metadata, NFO files and artwork

WittyMediaServer reads and writes metadata in the standard **NFO and artwork** formats that most other media software
already use, so you can move an existing library in — or out — without re-tagging anything.

- [Where metadata comes from](#where-metadata-comes-from)
- [What is saved next to your media](#what-is-saved-next-to-your-media)
- [Choosing artwork](#choosing-artwork)
- [Editing by hand](#editing-by-hand)
- [Coming from another media server](#coming-from-another-media-server)

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

**Works out of the box.** TV shows and movies get information without any account or key.
TVmaze data is shared under CC BY-SA 4.0, Wikidata is public domain, Wikipedia text is CC BY-SA
4.0; title pages credit their source. Turn either off in *Settings → Metadata agents*. Adding a TMDB or
TheTVDB key later upgrades titles the next time they are scanned or refreshed.

TVmaze and Wikidata do not always have a **content rating**, which Kids and Guest profiles rely
on. Set one under **Edit → Content rating** on any movie or show (a show's rating covers its
episodes).

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

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
