# Metadata, NFO files and artwork

WittyMediaServer reads and writes metadata in the formats **Kodi, Jellyfin, Emby and Plex**
already use, so you can move an existing library in — or out — without re-tagging anything.

- [Where metadata comes from](#where-metadata-comes-from)
- [What is saved next to your media](#what-is-saved-next-to-your-media)
- [Choosing artwork](#choosing-artwork)
- [Editing by hand](#editing-by-hand)
- [Coming from Plex, Jellyfin, Emby or Kodi](#coming-from-plex-jellyfin-emby-or-kodi)

## Where metadata comes from

Sources are applied in this order; later ones only fill gaps:

| # | Source | Example |
|---|---|---|
| 1 | `<file>.metadata.json` next to the file | `{"title":"Shrek","year":2001,"overview":"…","genres":["Animation"],"poster":"https://…"}` |
| 2 | Kodi/Jellyfin NFO | `Shrek (2001).nfo`, `movie.nfo`, `tvshow.nfo`, episode `<name>.nfo` |
| 3 | Local artwork | `poster.jpg`, `<name>-poster.jpg`, `folder.jpg`, `fanart.jpg`, `season01-poster.jpg`, `<episode>-thumb.jpg` |
| 4 | File and folder names | `Shrek (2001).mkv`, `Show - S01E02 - Title.mkv`, `SHREK.ISO` (disc label) |
| 5 | TMDB (movies) / TheTVDB (TV), if you added a key | overview, genres, rating, network, posters, backdrops, episode titles |
| 6 | Generated thumbnail | a frame 20 % into the video |

## What is saved next to your media

With *Settings → Server → Save metadata and artwork next to media files* on (the default) (*Settings → Server → Save metadata and artwork next to
media files*, on by default), in the layout Plex, Jellyfin, Emby and Kodi read:

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

**Edit** on any page changes title, year, overview, genres, poster and backdrop. With metadata
saving on, the NFO is rewritten at once (a hand-written NFO is kept as `.nfo.bak` the first time).

## Coming from Plex, Jellyfin, Emby or Kodi

- **Jellyfin / Emby / Kodi**: point a library at the same folders. Existing NFOs and artwork are
  read as-is, including TMDB/TheTVDB ids, so matches are exact.
- **Plex**: Plex keeps metadata in its own database, but reads and keeps local artwork
  (`poster.jpg`, `fanart.jpg`). WittyMediaServer matches by folder/file names like Plex does.
  After a scan with metadata saving on, your folders gain NFOs that Jellyfin/Kodi can read too.

---
<sub>WittyMediaServer 0.2.0-alpha documentation</sub>
