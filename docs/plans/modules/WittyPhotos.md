# WittyPhotos: update plan

> **Status on 2026-11-04**
>
> **In progress:** the map, [#69](https://github.com/wittywilla/WittyMediaServer/issues/69). **Planned:** [the rebuilt map](https://github.com/wittywilla/WittyMediaServer/discussions/35), [batch edit](https://github.com/wittywilla/WittyMediaServer/discussions/34), [local AI](https://github.com/wittywilla/WittyMediaServer/discussions/33).

**Now:** 0.3.0. Helper, API 3. Reads what a photo says about itself (EXIF, sizes, the preview inside RAW files) without decoding it. The gallery itself (timeline, albums, folders, map, upload, sharing) is in the server; the module is the reader.

**Channel:** alpha (with Module API 4).

| Release | Theme |
|---|---|
| 0.3.x | If the map fault is a small bug, the fix |
| **0.4.0** (next major) | The world map rebuilt (server and module together) |
| Later | Batch rotate and edit, local AI, faces and places |

---

## 0.4.0: the map

The owner: "The world map is broken, and needs to be rebuilt. This might be for the next major release of the module."

Full design: [../design/photos-map-rebuild.md](../design/photos-map-rebuild.md). Find out what's broken first; then clean positions at scan time (reject `0,0` and bad references), server-side grouping by cell keys, a vector map engine loaded only for the map view with an SVG fallback for old TVs, offline Natural Earth base map, optional online maps as today, optional offline place names, a list view that doubles as the accessible version, and a timeline filter.

**What's in the module and what's in the server:** reading and cleaning GPS from EXIF, XMP and RAW previews is the module's (so better GPS reading ships as a module update); grouping, the route and the view are the server's (so they ship with 0.7.0-alpha).

**Also in 0.4.0:**

- GPS from video files (phones write it into MP4 and MOV metadata), so clips appear on the map.
- HEIC and AVIF metadata coverage checked against real phone files.

## Later

- **Batch rotate and edit** (roadmap: not started): rotate, crop and simple adjustments, saved as edits beside the original (never changing the file), with a way to export.
- **Local AI** (roadmap: a plan document to write first): image recognition on the device for search ("beach", "dog"), then faces grouped on the device, all off by default, with a downloaded model whose licence is shown. **People & places** and `GET /api/photos/ai` are the placeholders.
- **Duplicates and near-duplicates** finder (hashes the module can compute without decoding whole files where possible).
- **Memories:** "on this day" rows on Home.
- **Live photos and motion photos** played as their clip.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
