# WittyChannels

Your archive of creators' videos, laid out the way people know from video sites: a home of
recommendations, subscriptions, a page per channel, and a watch page with comments and what's next.

WittyChannels shows video collections you already have on disk. It doesn't download anything and never
contacts a video site.

## Setting it up

Add a library of the kind **Creator videos**. One folder per channel, with subfolders as playlists:

```
Creator videos/
  Workshop Weekly/
    avatar.jpg
    banner.jpg
    channel.json
    Series One/
      episode 1 [xyz123].mp4
      episode 1 [xyz123].info.json
      episode 1 [xyz123].jpg
    a one-off.mp4
```

The same sidecar files as [WittyShorts](SHORTS.md) apply, and the same `channel.json`:

```json
{ "name": "Workshop Weekly", "handle": "@workshop", "description": "…", "platform": "…", "url": "https://…" }
```

Sidecars that hold saved comments have them imported, at most 500 per video, marked as *from the archive*,
below comments made here. A subfolder or a sidecar's `playlist_title` makes a playlist. Vertical videos
appear on a shelf of their own, and on a channel's *Vertical* tab.

## The screens

- **Home:** For You, from this profile's own record on this server ([how](SHORTS.md#for-you)).
- **Latest**, **Subscriptions**, **Channels**, **History** and **Liked**, plus search and topics.
  History is per profile; **Clear my history** on Your year empties it.
- **A channel:** its banner and details, with its videos, vertical videos, playlists and About.
- **Watching:** the full player with speed, chapters, audio and subtitle tracks; likes, Share and Save;
  the description and source; comments; and *Up next*, which goes on by itself unless you switch off
  Autoplay. A video picks up where you stopped.

Views on a card add what the source said when archived to plays on this server, so an archive reads
as it did.

Administrators can change a channel's details on its page, and a video's on the watch page. These stay in
the database, and your files aren't touched.

## Profiles and apps

Limits work as in [WittyShorts](SHORTS.md#profiles), and the routes are [the same](SHORTS.md#for-apps).
A channel with nothing a profile may see doesn't exist for that profile.

# Music videos and concerts

A library of the kind **Music videos & concerts** uses the same screens, in its own words: an artist is a
channel, an album a playlist, and **recordings of 20 minutes or more are listed under Concerts**. Their
chapters show as a **setlist** on the watch page.

Name files `Artist - Title.mp4`, or keep a folder per artist. An `.nfo` beside a file is read
(`title`, `artist`, `album`, `year`, `genre`, `tag`, `plot`), and so are the sidecars above, with `artist`
and `album` fields. To list concerts only: `GET /api/videos?library=&length=long`.
