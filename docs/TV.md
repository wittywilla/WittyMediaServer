# TV shows

TV libraries organise episodes by **show → season → episode**, fetch metadata from
[TheTVDB](https://thetvdb.com), and add binge features: *Next Up*, *Skip intro* and *Watch next*.

- [Setting up](#setting-up)
- [Folder layout](#folder-layout)
- [Show pages](#show-pages)
- [Networks and genres (for TV channels)](#networks-and-genres-for-tv-channels)
- [Fixing a wrong match](#fixing-a-wrong-match)
- [Anime](#anime)

## Setting up

1. *Settings → Metadata agents* → paste a **TheTVDB API key** ([get one](https://thetvdb.com/api-information);
   subscriber keys also need the PIN) and **Save**. Without a key, shows still work with names
   from folders and episode titles from file names.
2. *Settings → Libraries → Add library* → type **TV shows** → pick your TV folder → **Save & scan**.

## Folder layout

Any of these layouts work:

```
TV Shows/
├── Codename Kids Next Door/
│   ├── tvshow.nfo, poster.jpg, fanart.jpg        (optional)
│   └── Season 02/
│       ├── Codename - Kids Next Door - S02E01 - Operation C.A.T.S.mkv
│       └── Codename - Kids Next Door - S02E15-E16 - Two Parter.mkv
├── The Office (2005)/The.Office.S03E10.A.Benihana.Christmas.mkv
└── Lost/2x05 - ...And Found.avi
```

Season folders (`Season 2`, `S02`, `Specials`), `S01E01`, `s1.e1`, `1x01`, multi-episode files
(`S01E01E02`, `S01E01-E02`, `S01E01-02`) and `05 - Title.mkv` inside a season folder are all
understood. Files that cannot be numbered still appear under the show as *Other videos*.

## Show pages

Show pages offer **Cards** (thumbnails and overviews) or a compact **Playlist** layout. Each
show remembers its own choice; *Settings → Playback & discs → TV episode layout* sets the default.
**Skip intro** appears during an episode's intro when the file has an `Intro`/`Opening`
chapter, or when you set the intro range for the show (*Edit* on the show page). Near the
credits, **Watch next** offers the next episode, and when a show (or a movie) ends it
recommends another show or a similar movie.

## Networks and genres (for TV channels)

Every show carries its **network** (e.g. *Cartoon Network*) and its **genres** (e.g.
*Animation, Comedy*). These are the inputs for channel-style playback planned for a future
release. To obtain them:

1. *Settings → Metadata agents* → paste a **TheTVDB API key** (free for personal use; subscriber keys
   also need the PIN) and save.
2. Add (or rescan) a **TV shows** library. Each show is matched by folder name and year, and
   its original network, genres, status and content rating are stored.
3. Wrong match? On the show page choose **Edit** and enter the TheTVDB series id (the number in
   the show's thetvdb.com URL), or press **Refresh** to fetch again.
4. No key, or a show TheTVDB does not know? Put a `tvshow.nfo` in the show folder:
   `<tvshow><title>…</title><studio>Cartoon Network</studio><genre>Animation</genre><genre>Comedy</genre></tvshow>`
   (`<studio>` is the network). You can also type genres in the show's **Edit** dialog.

Read them back over the API (all endpoints need a token):

| Request | Returns |
|---|---|
| `GET /api/shows/networks?library=N` | `[{name, shows, genres, showIds}]`: every network with its shows and the genres found on them |
| `GET /api/shows?library=N&network=Cartoon%20Network` | the shows of one network |
| `GET /api/shows?library=N&genre=Comedy` | the shows of one genre (combine both to build a channel) |
| `GET /api/shows/{id}` | the show (`network`, `genres`, `status`, …), its seasons and episodes in order |

The network and genres are also written to `tvshow.nfo` (`<studio>`, `<genre>`) when metadata
saving is on, so they survive a database reset or a move to another server.


## Fixing a wrong match

If a show is matched to the wrong series, open it → **Edit** → enter the TheTVDB series id (the
number in the show's thetvdb.com address) → **Match & refresh**. Episode titles, overviews,
air dates and stills are re-applied to every episode. You can also put the id in `tvshow.nfo`
(`<uniqueid type="tvdb">76163</uniqueid>`) and rescan.

## Anime

Anime can live in its own **Anime** library, in your TV folders, or both:

- **An Anime library with its own folders** gets its information from
  [AniList](https://anilist.co) (free, no key): English, rōmaji or Japanese titles (*Settings →
  Server → Anime titles*), synopsis, studio, genres, score, cover and banner, and episode names
  where AniList has them. Titles AniList marks as adult are rated 18, so Kids and Guest
  profiles never see them.
- **Anime kept in a TV library** (any show with the genre *Anime*, which TVmaze, TheTVDB and
  AniList all set; you can also add it under *Edit → Genres*) also appears in your Anime
  library. *Settings → Metadata agents → Where anime from TV libraries appears* chooses **both** (the
  default) or **only the Anime library**.
- An Anime library may have **no folders at all**: it then simply collects the anime from your
  TV libraries.

File names with **absolute episode numbers**, as most anime releases use, are understood:

```
Frieren/[SubsPlease] Frieren - 012 [1080p].mkv     → episode 12
Cowboy Bebop/Cowboy Bebop - 05v2.mkv               → episode 5
One Piece/One Piece - 1071 - The Gear 5.mkv        → episode 1071, "The Gear 5"
```

Release-group tags in square brackets are ignored, and a year is never taken for an episode
number. `S01E12`-style names keep working as usual.

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
