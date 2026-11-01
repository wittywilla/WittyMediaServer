# Vertical videos

Use this guide to set up a feed of vertical videos from files you already have, with likes, comments, creators and a For You page worked out on your server alone.

Vertical videos, also called WittyShorts, plays files you supply. It doesn't download from video sites and never contacts one.

The Android and Apple apps can't open these libraries yet, so use a browser or the desktop app.

## Setting it up

1. Go to **Settings → Libraries**, or use first-time setup, and choose **Vertical videos**.
2. Choose its folder. One folder per creator works best:

```
Vertical videos/
  Quick Bakes/
    lemon tart [abc123].mp4
    lemon tart [abc123].info.json
    lemon tart [abc123].jpg
  Trail Runner/
    ridge run.mp4
```

Any video format the server plays will do, and there's no minimum size. Thumbnails are made when first needed, unless a picture with the video's name is beside it.

## Adding details with sidecar files

A sidecar is a small file beside a video that adds its details. These are read, in this order:

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

Without a sidecar, the creator is the first folder below the library, and the title is the file's name without the id archivers add.

**A creator's page** is filled in from their folder:

- `channel.json` (or `creator.json`), with `name`, `handle`, `description`, `platform` and `url`.
- A picture named `avatar`, `channel`, `profile`, `folder`, `poster` or `cover`.
- A picture named `banner`, `fanart`, `backdrop` or `header`.

Viewers see the platform of origin as text. A source address is a link, and only `http` and `https` addresses are accepted.

**Editing:** admins can change all of this with **Edit** beside a video. Edits stay in the database, so your files aren't touched, and later scans leave them alone.

## Watching

The feed plays one video and shows the others as pictures until they arrive, so a low-end phone has as much to do as for a single video. A video of 15 seconds or less plays twice before moving on. Longer ones move on when they end.

| Tab | What it shows |
|---|---|
| **For You** | The feed, in an order made for you |
| **Following** | The creators you follow. Press **Subscribe** under **About** on a video to follow its creator |
| **Liked** | What you liked |
| **Creators** | Everyone in the library |

Beside the video are **Like**, **Comments**, **Share**, **Speed**, **About** and, for admins, **Edit**. Speed stays for the videos that follow. Press the video to pause it, or use its pause and sound buttons.

| Key | What it does |
|---|---|
| ↑ ↓ | Previous or next video |
| L | Like |
| C | Comments |
| M | Sound on or off |
| → | Move to the buttons. ← comes back |

Swiping works on a touch screen. With a remote, ↑ and ↓ change video, → reaches the buttons and ← comes back.

**Sharing:** **Share** works as it does for photos, including links for someone without an account. See [Sharing](PHOTOS.md#sharing).

## How For You works

The order of the feed is worked out on the server, from what this profile did on this server and nothing else. There's no account anywhere else, and nothing leaves the server.

- A like, a comment and a full watch add to your taste for that video's tags and creator. A like counts most.
- A dislike takes away and hides the video. It's the ✕ beside **Like**, on a video's watch page in a [creator videos](CREATOR-VIDEOS.md) library. A skip takes away a little.
- Older actions count less: half as much every 45 days.
- A video scores your taste, plus how new it is, plus a little chance so the feed keeps offering something different, minus having been seen recently.
- One creator doesn't fill the screen while others are waiting.

Under each video the feed says why it's there: *Because you like #cats*, *More from …*, *New on this server* or *Watch again*.

**To start again from nothing,** use **Clear my history** on [Your year](COLLECTIONS.md#your-year). It clears the numbers behind For You, your history and your year, and leaves your place in videos and books. Each profile's feed is its own, and one profile's likes don't change another's.

## Profiles

Kids and Guest profiles see only what their limits allow. A video's rating comes from a sidecar's `contentRating` or `age_limit`, or from **Edit**. Unrated videos are hidden from them unless you allow titles without a rating, under **Settings → Profiles**. Kids can like but not comment, and a Guest can only watch.

## For developers

Every route is in [Client API](CLIENT-API.md#every-route). Videos are ordinary items: `GET /api/items/{id}/play` says how to play one, direct or HLS. These notes add what the route list doesn't say.

| Route | What to know |
|---|---|
| `GET /api/videos/feed?library=&seed=&limit=&offset=&shape=` | For You, one page. The first answer has the `seed` to send for the next pages, so a session's order holds |
| `GET /api/videos` | Lists, with `more`. Takes `library`, `creator`, `playlist`, `tag`, `platform`, `search`, `subscribed`, `liked`, `history`, `limit` and `offset`. `shape` is `vertical` or `wide`. `length` is `long` or `short`, split at 20 minutes. `sort` is `latest`, `oldest`, `popular` or `title` |
| `PUT /api/items/{id}/reaction` `{value}` | 1 is like, 0 is neither, -1 is dislike |
| `POST /api/items/{id}/view` `{seconds, completed, skipped}` | Report a video when moving on. This is what For You learns from |
| `POST /api/libraries/{id}/upload` | Multipart upload, field `file`. A Vertical videos library takes videos. The web app has no upload button for this kind yet |

---

<sub>WittyMediaServer documentation</sub>
