# WittyShorts

Vertical videos, one at a time, from files you already have: likes, comments, sharing, creators, your
own metadata and a For You page worked out on your server alone.

WittyShorts plays files you supply. It doesn't download from video sites and never contacts one.

## Setting it up

Add a library of the kind **Vertical videos** and choose its folder. One folder per creator works best:

```
Vertical videos/
  Quick Bakes/
    lemon tart [abc123].mp4
    lemon tart [abc123].info.json
    lemon tart [abc123].jpg
  Trail Runner/
    ridge run.mp4
```

Any video format the server plays will do, and there is no minimum size. Thumbnails are made when first
needed, unless a picture with the video's name is beside it.

## Sidecar files

Beside a video, in this order:

| File | Read for |
|---|---|
| `<name>.info.json`, `<name>.json` | What video archivers save: title, description, uploader or channel, upload date, view and like counts, tags, categories, age limit, web address, saved comments |
| `<name>.metadata.json` | Your own values, which win over the others |

Your own file can hold any of these fields:

```json
{
  "title": "Lemon tart",
  "creator": "Quick Bakes",
  "platform": "Where it came from",
  "url": "https://example.com/v/123",
  "date": "2024-05-31",
  "tags": ["baking", "dessert"],
  "contentRating": "PG",
  "custom": { "Recorded by": "Sam" }
}
```

Without a sidecar, the creator is the first folder below the library and the title is the file's name
without the id archivers add. A `channel.json` in a creator's folder (`name`, `handle`, `description`,
`platform`, `url`) and an `avatar.jpg` and `banner.jpg` beside it fill in the creator's page. Viewers
are shown *platform of origin* as text; a source address is a link, and only `http` and `https`
addresses are accepted.

Administrators can edit all of this on a video with **Edit**. Edits stay in the database, so your files
aren't touched, and later scans leave them alone.

## Watching

The feed plays one video and shows the others as pictures until they arrive, so a low-end phone has as
much to do as for a single video. Swipe or use ↑ and ↓. **L** likes, **C** opens comments, **M** mutes,
and Space or K pauses. With a remote, ↑ and ↓ change video, → reaches the buttons, ← comes back.

*Following* lists the creators you follow, *Liked* what you liked, *Creators* everyone. Playback speed
is on the side, and stays for the videos that follow.

## For You

The order of the feed is worked out on the server, from what this profile did on this server and nothing
else. There is no account anywhere else, and nothing leaves the server.

- A like adds to your taste for that video's tags and creator; a dislike (✕ on the watch page) takes away
  and hides it; so do a comment and a full watch (add), and a skip (take away a little).
- Older actions count less: half as much every 45 days.
- A video scores its taste, plus how new it is, plus a little chance so the feed keeps offering something
  different, minus having been seen recently.
- One creator doesn't fill the screen while others are waiting.

Under each video the feed says why it is there: *Because you like #cats*, *More from …*, *New on this
server*. To start again from nothing, use **Clear my history** on Your year. Each profile's feed is its
own, and one profile's likes don't change another's.

## Profiles

Kids and Guest profiles see only what their limits allow. A video's rating comes from a sidecar's
`contentRating` or `age_limit`, or from **Edit**; unrated videos are hidden from them unless you allow
unrated titles. Kids can like but not comment, and a Guest can only watch.

## For apps

| Route | What it gives |
|---|---|
| `GET /api/videos/feed?library=&seed=&limit=&offset=` | For You, one page. The first answer has the `seed` to send for the next pages, so a session's order holds |
| `GET /api/videos?library=&creator=&playlist=&tag=&platform=&search=&shape=vertical\|wide&length=long\|short&subscribed=&liked=&history=&sort=latest\|oldest\|popular\|title&limit=&offset=` | Lists, with `more` |
| `GET /api/videos/{id}`, `GET /api/videos/{id}/related` | One video, and what to watch after it |
| `PUT /api/videos/{id}` | Custom metadata (administrators) |
| `GET /api/videos/tags?library=` | Common tags |
| `GET /api/creators?library=`, `GET /api/creators/{id}`, `PUT /api/creators/{id}`, `GET /api/creators/{id}/art/avatar\|banner`, `PUT /api/creators/{id}/subscribe` | Creators |
| `PUT /api/items/{id}/reaction` `{value: 1, 0 or -1}` | Like, neither, dislike |
| `GET`/`POST /api/items/{id}/comments`, `PUT`/`DELETE /api/comments/{id}` | Comments |
| `POST /api/items/{id}/view` `{seconds, completed, skipped}` | Report a video when moving on: this is what For You learns from |
| `GET /api/items/{id}/poster?w=` | A JPEG thumbnail |

Videos are ordinary items: `GET /api/items/{id}/play` says how to play one, direct or HLS.
