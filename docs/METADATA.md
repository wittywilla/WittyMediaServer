# Metadata, NFO files and artwork

WittyMediaServer reads and writes metadata in the standard **NFO and artwork** formats that Kodi and most other media servers
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
| 1 | `<file>.metadata.json` next to the file | `{"title":"Shrek","year":2001,"overview":"…","genres":["Animation"],"poster":"https://…"}` |
| 2 | Kodi-style NFO | `Shrek (2001).nfo`, `movie.nfo`, `tvshow.nfo`, episode `<name>.nfo` |
| 3 | Local artwork | `poster.jpg`, `<name>-poster.jpg`, `folder.jpg`, `fanart.jpg`, `season01-poster.jpg`, `<episode>-thumb.jpg` |
| 4 | File and folder names | `Shrek (2001).mkv`, `Show - S01E02 - Title.mkv`, `SHREK.ISO` (disc label) |
| 5 | **TMDB** (movies) / **TheTVDB** (TV), if you added a key | overview, genres, rating, network, posters, backdrops, episode titles |
| 6 | **TVmaze** (TV) without a TheTVDB key — built in, no key | overview, genres, network, status, posters, backdrops, season posters, episode titles and stills |
| 7 | **Wikidata** + Wikipedia (movies) without a TMDB key — built in, no key | year, genres, US rating, runtime, plot summary (no posters) |
| 8 | Generated thumbnail | a frame 20 % into the video |

**Works out of the box.** TV shows and movies get information without any account or key.
TVmaze data is shared under CC BY-SA 4.0, Wikidata is public domain, Wikipedia text is CC BY-SA
4.0; each title page links to its source. Turn either off in *Settings → Server*. Adding a TMDB or
TheTVDB key later upgrades titles the next time they are scanned or refreshed.

TVmaze and Wikidata do not always have a **content rating**, which Kids and Guest profiles rely
on. Set one under **Edit → Content rating** on any movie or show (a show's rating covers its
episodes).

## What is saved next to your media

With *Settings → Server → Save metadata and artwork next to media files* on (the default) (*Settings → Server → Save metadata and artwork next to
media files*, on by default), in the standard layout that Kodi and other media servers read:

| Media | Written |
|---|---|
| Movie / disc image in its own folder | `<name>.nfo`, `poster.jpg`, `fanart.jpg` |
| Movies sharing a folder (e.g. a folder of ISOs) | `<name>.nfo`, `<name>-poster.jpg`, `<name>-fanart.jpg` |
| `VIDEO_TS` folder | `movie.nfo`, `poster.jpg`, `fanart.jpg` inside the movie folder |
| TV show | `tvshow.nfo`, `poster.jpg`, `fanart.jpg`, `season01-poster.jpg`… in the show folder |
| Episode | `<name>.nfo` (`<episodedetails>`), `<name>-thumb.jpg` |

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

- **Servers that use NFO files (and Kodi)**: point a library at the same folders. Existing NFOs
  and artwork are read as-is, including TMDB/TheTVDB ids, so matches are exact.
- **Servers that keep metadata in their own database**: local artwork (`poster.jpg`,
  `fanart.jpg`) is still read, and WittyMediaServer matches by the usual folder/file naming
  (`Movie (Year)`, `Show/Season 01/Show S01E01`). After a scan with metadata saving on, your
  folders gain NFOs that other NFO-based tools can read too.

---
<sub>WittyMediaServer 0.2.0-alpha documentation</sub>
