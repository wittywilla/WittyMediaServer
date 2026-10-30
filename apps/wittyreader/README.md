<p align="center">
  <img src="public/icon.svg" width="88" alt="">
</p>

<h1 align="center">WittyReader</h1>

<p align="center">
  <b>Read and annotate books, PDFs and comics — on your own device.</b><br>
  Highlights, notes and a pen with pressure. Nothing is uploaded: your library and notes stay with you.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Installable app" src="https://img.shields.io/badge/installs%20as-an%20app-8b6cff.svg">
  <img alt="Works offline" src="https://img.shields.io/badge/works-offline-success.svg">
  <img alt="No uploads" src="https://img.shields.io/badge/uploads-none-informational.svg">
</p>

<p align="center">
  <a href="#run-it">Run it</a> ·
  <a href="wiki/Getting-started.md">Getting started</a> ·
  <a href="wiki/Privacy-and-security.md">Privacy</a> ·
  <a href="wiki/Home.md"><b>Full guide</b></a>
</p>

> Read-only mirror of `apps/wittyreader` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## Highlights

| | Your library | |
|---|---|---|
| ➕ | **Add books your way** | the **Add books** button, Ctrl+O, drag and drop, or your system's *Open with* once installed |
| ▶️ | **Continue reading** | the book you're in, front and centre, with your progress |
| 🔎 | **Find anything** | search, filter by books, PDFs or comics, and sort by recent, added, title or author |
| 🖼️ | **Covers and titles from the files** | EPUB metadata and covers, PDF first pages, comic `ComicInfo.xml`; books without a cover get one of their own |
| ♻️ | **No duplicates** | adding the same file again finds the copy you have |

| | Reading | |
|---|---|---|
| 📖 | **Books (EPUB)** | with [WittyEbook](https://github.com/wittywilla/WittyEbook): book view, typography, page colours, night light, time left |
| 📄 | **PDFs** | with [WittyPDF](https://github.com/wittywilla/WittyPDF): zoom, dark pages, outline, thumbnails, search |
| 💬 | **Comics and manga (CBZ)** | with [WittyComics](https://github.com/wittywilla/WittyComics): pages, spreads, strips, either direction |
| ✍️ | **Study tools** | from [WittyInk](https://github.com/wittywilla/WittyInk): highlights, notes, bookmarks, citations, read aloud, a pen with pressure |

| | Private and portable | |
|---|---|---|
| 🔒 | **On the device** | books, positions and notes live in this browser; the only outside request is a dictionary look-up, off until you turn it on |
| 🛡️ | **Books can't run code** | EPUB files are sandboxed with scripts blocked; PDF scripts and forms are off; the app runs under a strict security policy |
| 📦 | **Notes go with you** | export a book's notes to a file and import them on another device, or save a readable Markdown summary |
| ✈️ | **Offline** | works offline after the first visit, and installs as an app |

## Run it

You need [Node.js](https://nodejs.org) 20 or newer.

```bash
npm install
npm run dev        # http://localhost:5180
npm run build      # the finished app, in dist/
```

`dist/` is a plain static site: host it on any web server over **https**, or open it from `localhost`.
Browsers only allow the offline service worker, which also shows EPUBs, on secure addresses. See
[Hosting and building](wiki/Hosting-and-building.md).

| Guide | |
|---|---|
| [Getting started](wiki/Getting-started.md) | adding books and finding your way |
| [Your library](wiki/Your-library.md) · [Reading](wiki/Reading.md) | the shelf, and every reading tool |
| [Notes and backups](wiki/Notes-and-backups.md) | exporting and importing notes |
| [Privacy and security](wiki/Privacy-and-security.md) · [Troubleshooting](wiki/Troubleshooting.md) | |

## The Witty family

| Project | Role |
|---|---|
| **WittyReader** | this app: the readers, on your own device |
| [WittyEbook](https://github.com/wittywilla/WittyEbook) · [WittyPDF](https://github.com/wittywilla/WittyPDF) · [WittyComics](https://github.com/wittywilla/WittyComics) · [WittyInk](https://github.com/wittywilla/WittyInk) | the readers and their shared tools |
| [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the same readers, plus films, TV, discs and music, served to your whole home |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyReader, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.
