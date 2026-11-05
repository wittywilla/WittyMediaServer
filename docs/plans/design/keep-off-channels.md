# Design: keeping a title off TV channels

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/48). Not in 0.7.0-alpha.

The owner, 2026-11-02: "allow individual listings (like a DVD for instance) to be unavailable for the TV Guide to use (for instance, my 3d blu ray)".

For WittyTV, in 0.7.0-alpha. Not built. Not tested.

---

## What exists today

From `docs/CHANNELS.md`: a library channel's programmes come from **rules** (library, genre, network, decade, rating, show, season, watched state, length, kind, hand-picked items, clips), from **one-click** suggestions (Movies, Saturday morning, Genres, Studios, collections, Decades, networks, Marathons), and from discs' **main features and scenes** under **Discs and scenes**. Commercials libraries never become programmes, and encrypted discs are refused.

There's **no way to say "never this one"**. A 3D Blu-ray in a Discs library can land on a Movies or Studios channel, where it plays its main feature in 2D at best, or as two squashed views side by side. Some titles shouldn't be on channels for other reasons too: a film someone hasn't watched yet and doesn't want spoiled in passing, a home video, a disc whose main feature is detected wrongly, something not suited to a channel the whole house can tune to.

---

## The design

### 1. One switch on the title: Keep off TV channels

- On the item page's menu, in the edit dialog (under **Playback** or **Organise**), in bulk edit for many at once, and later in the right-click menu (next major): **Keep off TV channels**.
- Works at every level: a film, a disc, a show (all its episodes), a season, or one episode. A show kept off keeps all its episodes off, and an episode can be kept off alone.
- Admins only: channels are shared by everyone, so this is the owner's decision, not each profile's.
- The item page shows a small **Off TV channels** label while it's on, with the switch beside it, so it's never a mystery why a film never comes up.

### 2. What it does

- **Rules, one-click channels, network channels, marathons and day parts** leave it out of their pools. `GenerateDay` filters it in the pool's SQL query (one extra condition on an indexed column), so idle channels cost nothing extra.
- **Discs and scenes** in the editor lists it greyed with "Kept off TV channels", and its main feature and scenes aren't offered.
- **Hand-picked into a channel anyway:** the editor asks "This title is kept off TV channels. Put it on this channel anyway?" and, if yes, that channel holds an exception for it. The title stays off every other channel. This keeps the switch simple ("off everywhere") while still allowing a deliberate 3D night on a 3D channel.
- **Already scheduled:** a title switched off leaves every schedule from the next programme that hasn't started yet. Nothing on air is cut. The channel page offers **Rebuild after what's on now** (it exists) to fill the gap at once; otherwise the next day's schedule simply doesn't include it.
- **Watch history, Continue watching, search and the library** are unaffected. Only channels change.

### 3. Per channel as well

Each channel's editor gets **Never on this channel**: a list of titles (and shows) left out of that channel only, for the "I like this film, just not on Saturday morning" case. Same picker, same behaviour.

### 4. Sensible defaults, so nobody has to find the switch

Rules that leave out, by default, titles that rarely belong on a channel:

| Leave out by default | Why | Where it's known |
|---|---|---|
| **3D titles** (3D Blu-rays and 3D files) | A channel plays one shared 2D stream; 3D comes out as half a picture or two squashed views | The Blu-ray's 3D flag; a file's stereo metadata (the play info already reports `stereo`) |
| Titles the server can't play without a disc menu | They'd show a still or nothing | The disc module's main-feature detection, when it isn't sure |
| Very long titles over a channel's longest slot | They'd never fit a grid and push everything back | Length |

Each is a tick in the rule editor (**Include 3D titles**, off by default), and shown in one-click suggestions as "3D titles left out". The owner's 3D Blu-ray would be off every channel with no action needed.

### 5. Seeing and undoing it

- **Settings → Channels & tuners → Kept off TV channels:** every title switched off, with **Put back** on each, and the default rules with their counts ("3D titles: 4 left out").
- A library filter **Kept off TV channels**, so it can be done in bulk from a library page.

### 6. Data and API

- A column `items.channels_off` (or a small `channel_exclusions` table keyed by item, which also holds show and season level), added to `tableParts` in `internal/backup/backup.go` so backups keep it.
- A channel's definition gains `exclude: [item ids]` (never on this channel) and `allow: [item ids]` (the exceptions from section 2), and the rule's `include3d`, all versioned with the channel's history as edits are now.
- `PUT /api/items/{id}/channels-off` (admin), and the field in item answers (`channelsOff: true`) so apps can show the label.
- Client API: the guide is unchanged; it simply never lists the title.

### 7. Accessible and portable

The switch is a normal toggle with a name ("Keep off TV channels"), works by keyboard, remote and controller, and is read out with its state. The same switch comes to WittyMediaClient's native apps with their metadata editing.

---

## Where it goes

| Release | Work |
|---|---|
| **0.7.0-alpha** | The switch on titles (film, disc, show, season, episode), pools and the disc list honouring it, the hand-pick exception, the label, the Settings list, **Leave out 3D titles** by default, backup |
| **0.8.0-alpha** (next major) | **Never on this channel** per channel, the other default rules, the library filter and bulk edit, the right-click menu entry |

## Done when

The owner's 3D Blu-ray never appears on any channel, without them doing anything; any other title can be switched off from its own page in two presses; switching it off removes it from every schedule from the next programme on; and **Put back** reverses it.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
