# The readers: WittyEbook, WittyPDF, WittyComics and WittyInk: update plans

> **Status on 2026-11-04**
>
> **Planned:** [read aloud and your server's books](https://github.com/wittywilla/WittyMediaServer/discussions/64). Nothing in progress.

Four web modules that make up books in the server and in WittyReader.

| Module | Now | Does |
|---|---|---|
| WittyEbook | 0.2.0 | Reflowable EPUB: book view, typography, study tools, handwriting |
| WittyPDF | 0.2.1 | PDFs: selectable text, zoom, outline, thumbnails, search; pdf.js only loads when a PDF opens |
| WittyComics | 0.2.0 | Comics and manga: pages, spreads, vertical strips, either direction |
| WittyInk | 0.2.0 | What they share: annotations, notes, definitions, citations, handwriting |

All on the **alpha** channel. Books can't run code: EPUBs sandboxed with scripts blocked, PDF scripts and forms off.

---

## Shared next major (0.3.0 for each)

- **Read-aloud** with the device's own speech (Web Speech API, no online voices by default), following the text, with speed and voice choice. Big accessibility win.
- **Sync of positions and notes** across devices through the server (the server already has books; WittyReader keeps things on the device and stays that way unless the person signs in to a server).
- **Native apps:** books in WittyMediaClient on Android and Apple (roadmap: not started) by embedding these bundles in a web view with the same `ReaderHost`, before any native reader.
- **Controller and remote reading** on TVs: page turns, zoom, panels.

## WittyEbook

- 0.2.x: real-book sweep (open EPUBs from public-domain sources: layout, fonts, footnotes).
- 0.3.0: EPUB 3 media overlays (read-along audio) where the book has them; fixed-layout EPUB.
- Later: dictionaries offline as an optional download (Wiktionary-derived, with licence shown).

## WittyPDF

- 0.2.x: large-file streaming measured on a low-end box.
- 0.3.0: reflow mode for text PDFs on phones; annotations saved through WittyInk into the server rather than the file.
- Later: forms (fill, never run scripts) if people ask.

## WittyComics

- 0.2.x: CBR (RAR) reading, if a decoder with a usable licence exists; otherwise say clearly that CBZ/CB7 are supported.
- 0.3.0: panel-by-panel view (guided reading) by image analysis on the device; double-page detection.
- Later: series reading order from metadata (ComicInfo.xml).

## WittyInk

- 0.3.0: export notes and highlights (Markdown, plain text), import from common formats.
- Later: shared notes between profiles (a book club on one server), off by default.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
