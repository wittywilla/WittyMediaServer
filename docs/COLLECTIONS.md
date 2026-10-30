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

A library of the kind **Internet radio** needs no folder. Add stations on its page, by stream address or
from an M3U or PLS playlist, and play them from the bar along the bottom, which keeps going while you
browse.

- **When anyone is online:** your server contacts a station only while someone is listening to it. There's
  no directory and no lookup; every address is one an administrator added.
- **Relayed:** listeners get the stream from your server, so `http` stations work on secure pages and a TV
  needs just one sign-in. That takes the station's bandwidth through your server.
- **Internet only:** the server refuses to reach addresses on your own network on a station's behalf, so a
  playlist can't be used to poke at your other devices. A station on your own network can't be added for
  the same reason. An address that gives anything but audio is refused too.

Routes: `GET /api/radio?library=`, `POST /api/radio` `{libraryId, name, url, genre}`, `POST /api/radio/import`
`{libraryId, playlist}`, `DELETE /api/radio/{id}`, and `GET /api/radio/{id}/stream` for the audio itself.

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
