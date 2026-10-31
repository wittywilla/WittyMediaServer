# Collections, radio and Your year

Three things that work across the server. All of them are worked out on your server from what's on it.

## Collections and smart playlists

**Collections** in the side menu gathers things from any library.

- **A list you fill:** use **Save** on a film, show or creator video. **Watch later** is one every profile
  has, made the first time it's used.
- **A smart playlist:** a saved rule, worked out each time it's opened, so it keeps itself up to date. For
  example: unwatched comedies from the 90s rated 7 or more, most recently added first, at most 50. The
  rule can use kinds of library, genres, words in the title, artist or creator, years, rating, length,
  watched or not, liked, how recently added, and a sort order including *Most played* and *Shuffle*. The
  page shows what it finds as you build it. A rule returns at most 500 things.

**Play all** goes through a list in order, with videos one after another. A list is yours unless you tick
**Share**; a shared one is visible to every profile, and each sees only what its own limits allow, so a
rule can't get round them. Kids can use Watch later; making collections is for adults.

| Route | What it does |
|---|---|
| `GET`/`POST /api/collections` | List, make |
| `GET /api/collections/{id}?limit=&offset=` | One, with its `items` and `shows` |
| `PUT`/`DELETE /api/collections/{id}` | Change, delete (what's in it stays) |
| `POST`/`DELETE /api/collections/{id}/items` `{items, shows}` | Add, remove |
| `POST /api/collections/watch-later/items` `{items, shows}` | Add to Watch later |
| `POST /api/collections/preview` `{rule}` | What a rule would find |
| `GET /api/collections/holding?item=&show=` | Which of your lists hold something |
| `GET /api/items/{id}?collection=` | An item, with `nextId` being what follows in that list |

A rule is JSON:

```json
{ "libraryTypes": ["movies"], "genres": ["Comedy"], "yearFrom": 1990, "yearTo": 1999, "ratingMin": 7,
  "watched": "no", "addedWithinDays": 0, "sort": "added", "desc": true, "limit": 50 }
```

## Internet radio

A library of the kind **Internet radio** needs no folder. Choose **Add stations** on its page, then:

- **Search:** by name, genre, country or language, with logos and details, and add with one click. Popular
  stations show before you type.
- **Stream address:** type a station's address yourself. Its name and details are filled in the first time it
  plays, if it gives them. **Find details in the directory**, on the station's page, looks it up there.
- **Playlist:** an M3U or PLS file, or its text.

Stations play from the bar along the bottom, which keeps going while you browse. A stream that drops is
tried again a few times by itself, and a sleep timer stops the music when you want. The library has your
favourites and what you played lately at the top, and can be browsed by genre and country. A station's page has
its logo, genres, country, language, quality and website, and what's playing now, shown on the lock screen and
headphone controls too. Stations with no logo get art made from their name.

### What's on now

The song comes from the stream itself, in Icecast and Shoutcast (ICY) metadata, and in HLS from the playlist
or a tag at the start of each segment. It's known only while someone is listening through your server, and
only if the station sends it; others just say **Live**. Ogg and MPEG-TS streams carry theirs in a way that isn't
read yet, so they say **Live** too.

### Stations that go quiet

A station is marked **off the air**, not silently kept, when it fails three times at least ten minutes apart, or when
the directory's own checks can't reach it and it hasn't answered here for a week. One good answer clears it. **Try it
now** and **Check stations** try again whenever you like. If nothing at all answers during a check, your own
connection is probably down, so no station is blamed for it.

### What contacts what

| | When | What is sent |
|---|---|---|
| The station | While someone listens, and when you check it | A normal request for the stream, from your server |
| The station, in the background | Only with **Check radio stations in the background** on (off unless you turn it on): each about once a week, one at a time | The same |
| The directory | When an administrator searches or adds | What you typed, and a count that this station was chosen |
| The directory | Once a day, in small batches, for stations added from it | Only those stations' IDs |
| Logos | The first time one is shown | Fetched by your server and kept, so a browser never contacts the station's site |

The directory is never told what anyone listens to, and an address you typed goes to it only when you ask, and never
if it could hold a private key (a query string or a sign-in). Turn the directory off under **Settings, Media
Indexers and Metadata Agents**; searching then says so, and everything else keeps working.

### The directory

Stations come from [Radio Browser](https://www.radio-browser.info/), a free, community-run database with an open
API. Its entries can be edited by anyone, so everything from it is treated as untrusted: addresses, text and logos
are checked and cleaned before they're saved or shown.

### Safety

- **Relayed:** listeners get the stream from your server, so `http` stations work on secure pages and a TV
  needs just one sign-in. That takes the station's bandwidth through your server; pausing hangs up, so a paused
  station costs nothing.
- **Internet only:** the server refuses to reach addresses on your own network on a station's behalf, so a
  playlist, a redirect or a logo can't be used to poke at your other devices. A station on your own network can't
  be added for the same reason.
- **Audio only:** an address that gives anything but audio is refused, and an HLS station's video variants are
  left out. Encrypted streams are refused, and are never unlocked.
- **Addresses stay private:** only administrators see a station's address, which may hold a key. The relay's HLS
  addresses are signed, so it fetches only what a station's own playlist named.

Routes are in [CLIENT-API.md](CLIENT-API.md#internet-radio). Native apps play a station like anything else, with
`POST /items/{id}/playback`.

## Your year

**Your year**, in the profile menu, adds up what this profile watched and listened to: total time, days,
the longest run of days in a row, month by month, and the top shows, films, artists, creators and genres.

It comes from a small record kept on your server, per profile, per day: only how long something was played.
Only that profile can see it, and nothing is sent anywhere. **Clear my history** on that page forgets it,
along with the numbers For You learns from; your places in videos and books stay. Time counts from the
version that added this, so earlier years stay empty.

`GET /api/me/review?year=` gives it as JSON; `DELETE /api/me/history` clears it.

## Recommended for you

A row on the home page, which can be moved or hidden like the others, suggests unwatched films and shows in
the genres this profile spends its time on. It reads only this profile's own record and doesn't appear until
there is something to go on: `GET /api/me/recommended`.
