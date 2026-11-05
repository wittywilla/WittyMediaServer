# Design: rebuilding the WittyPhotos world map

> **Status on 2026-11-04**
>
> **In progress:** [#69](https://github.com/wittywilla/WittyMediaServer/issues/69), in 0.7.0-alpha. The rebuilt map is [an idea](https://github.com/wittywilla/WittyMediaServer/discussions/35).

For WittyPhotos 0.4.0 (the owner: "This might be for the next major release of the module"). The owner: "The world map is broken, and needs to be rebuilt."

Not built. Not tested. The owner's report doesn't say *how* it's broken, and this plan doesn't run anything to find out (as asked), so step 1 is to find out.

---

## What the map is today

From `docs/PHOTOS.md`:

- **Map** is one of the gallery's views: "pictures with a place, grouped as you zoom. Choosing a group zooms in, or lists its pictures."
- **It needs nothing online.** A small outline of the world's land comes with the app, from Natural Earth (public domain). The groups come from the server.
- **Optional online map:** an admin can give an online tile provider's address in **Settings → Sharing & photos → Photo map**, off by default, with the privacy note that each viewer's device asks that provider for the area being looked at.

## Step 1: find out what's broken

Ask the owner what they saw (a screenshot and the device), then check each of these on the demo library (`D:\WittyDemoMedia`, credited pictures with places) and on a phone, a TV and a desktop:

| Possible fault | How it would look |
|---|---|
| Groups in the wrong places | Projection or latitude/longitude order mixed up somewhere between EXIF, the database and the drawing |
| Groups that don't split as you zoom, or vanish | Server grouping at the wrong cell size for the zoom, or the client asking for the wrong box |
| Pictures across the date line (Pacific) | A box that crosses longitude 180 asked for as one box, returning nothing |
| Can't pan or zoom with touch, keyboard or remote | Input handling |
| Blank map with an online provider set | Content security policy not allowing that provider, or a template address mistake |
| Slow with many photos | Too many markers drawn, or every photo sent |
| Outline misaligned with the groups | The outline and the groups drawn in different projections |

If it's one small bug, fix it in a WittyPhotos patch release (and in the server, if the fault is there) and stop. If it's structural, rebuild as below.

## Step 2: the rebuild

### Goals

1. **Correct** everywhere, including the poles, the date line and photos with odd EXIF (`0,0`, missing reference letters, altitude only).
2. **Offline first,** as today. Online maps stay optional and off.
3. **Quick with 100,000 placed photos** on a low-end server and an old TV browser.
4. **Usable by mouse, touch, keyboard, remote and screen reader.**
5. **Better than "dots on land":** place names offline, a timeline filter, and a good list view.

### Drawing the map

- **MapLibre GL JS** (BSD-3-Clause) as the map engine, loaded only when the map view opens. It draws vector maps on the graphics card, handles the date line, touch and zoom, and works with offline data. **Licence to show** in Settings → About.
- **A fallback for browsers without WebGL** (old TVs): a plain SVG world from the same offline outline, with the same groups, pan by arrows, zoom by + and −. Lighter, less pretty, always works.
- **Offline base map:** Natural Earth (public domain) at small scale for land, borders, lakes and large cities, packed as a single vector file served by the server (a few megabytes). No street detail offline; that's what the optional online provider is for.
- **Optional online maps:** keep today's setting and warning. Accept a vector style address too. Show the provider's required attribution on the map.

### Grouping on the server

- Store each photo's position once, at scan time, already cleaned:
  - Reject `0,0` (almost always a camera with no fix), out-of-range values and missing reference letters; keep them listed as "Place unclear" for the owner to look at, never shown in the sea off Africa.
  - Keep a **cell key** per photo for each zoom level (a geohash or a quadkey), indexed. Grouping is then a `GROUP BY` on a prefix, which is cheap in SQLite.
- `GET /api/photos/map?bbox=…&zoom=…&from=…&to=…&album=…` returns groups: centre, count, a cover photo id, and the box of the group. A box crossing the date line is split into two queries on the server.
- At the deepest zoom, single photos with thumbnail ids.
- The answer is small (a few hundred groups at most for any view) and cached per view.

### Place names offline

- An optional download, asked in setup and in Settings, of **GeoNames cities** (CC BY 4.0, attribution shown) or Natural Earth populated places (public domain, fewer names).
- Each photo gets a nearest-city name at scan time, offline, from a small spatial index. That gives "Paris, France" in the list, search by place, and albums by place, without asking anything online.
- Off: places show as coordinates and country only (from the Natural Earth outline).

### The view

- **Map and list together:** on desktop, the map with a list of the groups in view beside it; on a phone, the map with a sheet that pulls up; on a TV, the list first, with the map following the highlighted group.
- **Timeline filter:** a range slider for when the photos were taken, which updates the groups.
- **Choosing a group** zooms to its box, or, at the deepest level or with few photos, opens them in the gallery viewer.
- **Keyboard and remote:** arrows pan, + and − zoom, Tab moves between groups in reading order, Enter opens. Focus is visible on the map.
- **Screen reader:** the list *is* the accessible version: "Paris, France, 214 photos, June 2019 to May 2024". The map canvas is hidden from the accessibility tree with a label pointing to the list.
- **Privacy:** sharing a photo or album never shares its position unless the owner chose so (check the existing share options do this; **to verify**). A per-library setting hides the map for libraries where places are private.

### Apps

The route is JSON, so native apps draw their own map with platform maps or MapLibre Native (BSD-3). Until then, the list view works in every app.

## Performance budget

- Opening the map: under 1 second to the first groups on a low-end server with 100,000 placed photos.
- Panning: one request per settled view, cancelled if the view moves again.
- The map engine's bundle loads only for this view.

## Order of work

1. Find out what's broken (step 1).
2. Clean positions and cell keys at scan time, with a migration that fills them for existing photos as a background task (shown in the activity panel).
3. The map route and its tests (date line, poles, `0,0`, empty, 100,000 photos).
4. The new view, with the SVG fallback.
5. Offline place names (optional download).
6. Accessibility, TVs, phones, both themes.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
