# WittyReader

**Read and annotate books, PDFs and comics on your own device.** WittyReader is a reading app
for EPUB books, PDFs and CBZ comics, with highlights, notes and handwriting with pen pressure.
Nothing is uploaded: your library, reading positions and notes stay on this device.

📖 **[Read the full guide](wiki/Home.md)** in the `wiki` folder.

> This repository is a read-only mirror of `apps/wittyreader` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

### Your library

- **Add books:** the **Add books** button, **Ctrl+O**, dragging files onto the window, or, once
  installed, your system's **Open with** menu. Add several at once; a single file opens straight
  away.
- **A shelf of covers**, with each book's format and how far you've read, most recent first.
- **Covers and titles are read from the files themselves:**
  - EPUB: title, author, year and cover;
  - PDF: title, author and the first page as the cover;
  - comics: series, issue, writer, year and reading direction from `ComicInfo.xml`, with the
    first page as the cover.
- **No duplicates:** adding the same file again finds the copy already in your library.
- **Per book (⋯):** continue reading, export notes, or remove the book and its notes from the
  device.

### Reading

- **Books (EPUB),** with [WittyEbook](https://github.com/wittywilla/WittyEbook):
  - book view, typography, page colours, brightness and night light;
  - time left, footnote pop-ups and exact resume.
- **PDFs,** with [WittyPDF](https://github.com/wittywilla/WittyPDF):
  - zoom, one or two pages, dark pages;
  - contents, thumbnails, search and exact resume.
- **Comics and manga (CBZ),** with [WittyComics](https://github.com/wittywilla/WittyComics):
  - pages, spreads or a vertical strip;
  - either reading direction, remembered per comic.

### Study tools and handwriting (from [WittyInk](https://github.com/wittywilla/WittyInk))

- **Highlights** in any colour with **notes** and **colour meanings**, **bookmarks**,
  **search**, **citations**, **read aloud** and **Markdown export**.
- **Handwriting:**
  - pen, pencil, highlighter and eraser, with **pen pressure** and pencil tilt;
  - 54 colours, palm rejection, undo and redo.

### Your notes, portable

- **Export** a book's notes (highlights, notes, bookmarks and handwriting) to a file, and
  **import** them on another device.
- In each book, **Export** in the notes panel saves a readable Markdown summary.

### Private and safe

- **On the device:** books, positions and notes live in this browser's storage. The only outside
  request is a dictionary look-up, off until you turn it on.
- **Books can't run code:** EPUB chapters are shown in a sandbox with scripts blocked. PDF
  scripts and forms are off. The app itself runs under a strict security policy.
- **Works offline** after the first visit, and **installs as an app**.

## Run it

You need [Node.js](https://nodejs.org) 20 or newer.

```bash
npm install
npm run dev        # http://localhost:5180
npm run build      # the finished app, in dist/
```

`dist/` is a plain static site: host it on any web server over **https**, or open it from
`localhost`. Browsers only allow the offline service worker, which also shows EPUBs, on secure
addresses. See
[Hosting and building](wiki/Hosting-and-building.md).

Desktop builds that open in their own window are planned.

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyReader. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyReader, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
