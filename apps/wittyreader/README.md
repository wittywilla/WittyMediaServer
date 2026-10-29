# WittyReader

Read and annotate **books, PDFs and comics** on your own device. Nothing is uploaded: your
library, reading positions and notes stay in this browser on this device.

- **Books (EPUB):**
  - Book view, typography, page colours, brightness and a night light.
  - Exact resume, time left and footnotes in a pop-up.
- **PDFs:**
  - Selectable text, zoom, two pages across and dark pages.
  - The document's contents, thumbnails and search.
- **Comics and manga (CBZ):**
  - Pages, spreads or a vertical strip, in either reading direction.
- **Study tools everywhere:**
  - Highlights in any colour with notes, and colour meanings ("Definition", "Question"…).
  - Bookmarks, search, read aloud, citations and Markdown export.
- **Handwriting with pen pressure:**
  - Pen, pencil (widens with tilt), highlighter that can snap to text, and eraser (also a pen's
    eraser end).
  - Undo and palm rejection.
- **Your notes are portable:** export a book's notes to a file and import them on another device
  (or, soon, into WittyMediaServer).
- **Works offline** after the first visit, and installs as an app: books open from the system's
  *Open with* menu.

WittyReader is built from the [WittyEbook](https://github.com/wittywilla/WittyEbook),
[WittyPDF](https://github.com/wittywilla/WittyPDF) and
[WittyComics](https://github.com/wittywilla/WittyComics) plugins, which also run inside
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where your notes follow you
to every device.

> This repository is a read-only mirror of `apps/wittyreader` in WittyMediaServer, where
> development happens. Please open issues and pull requests there.

## Privacy and security

- **Your files stay on the device.** Books are read on the device and kept in the browser's own
  storage. The only outside request WittyReader can make is a dictionary look-up, and only after
  you turn on **Settings → Look up words online** (only the word is sent, to dictionaryapi.dev).
- **Books are untrusted.** EPUB chapters are shown in a sandboxed frame with scripts blocked by a
  strict Content-Security-Policy. PDF scripting and forms are off. The app itself runs under a
  strict policy too.
- **Keep a backup.** Clearing the browser's data for the site removes the library, so export the
  notes you want to keep (**⋯ → Export notes** on a book).

## Run it

You need [Node.js](https://nodejs.org) 20 or newer.

```bash
npm install
npm run dev        # http://localhost:5180
npm run build      # the app, in dist/
```

`dist/` is a plain static site. Host it on any web server over **https** (or open it from
`localhost`); browsers only allow the offline service worker and EPUB reading on secure
addresses.

Desktop builds that open in their own window (using the system's web view) are planned.

## License

Apache-2.0
