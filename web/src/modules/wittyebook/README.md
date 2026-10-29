# WittyEbook

**A reader for reflowable ebooks, built for long and careful reading.** WittyEbook opens EPUB
books (EPUB 2 and 3). It gives you a book-like page you can set to your own taste, the study
tools serious readers need, and handwriting with pen pressure.

It runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) (your books
libraries) and the [WittyReader](https://github.com/wittywilla/WittyReader) app.

📖 **[Read the wiki](https://github.com/wittywilla/WittyEbook/wiki)** for the full guide.

> This repository is a read-only mirror of `web/src/modules/wittyebook` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

### Reading

- **Layouts:** **Book view** (two pages side by side with a spine), **One page**, **Scroll**, or
  **Auto**, which uses book view when the window is wide.
- **Turning pages:** tap or click the outer edges, swipe, or use the arrow keys, Space and Page
  Up/Down. Turns have a light animation (switched off if your system asks for reduced motion).
- **Progress:** a slider across the whole book, the percentage read, and **time left** in the
  chapter and the book, worked out from your own reading speed.
- **Exact resume:** the reading position is a place in the text, so the book reopens on the same
  words on any screen, at any font size. In WittyMediaServer it follows you to every device.
- **Contents** from the book's own table of contents, including sections inside chapters.
- **Footnotes open in a pop-up** instead of taking you away from the page. Links within the book
  go to the right place, and web links open in a new tab.

### Text and display

- **Fonts:** the publisher's font, serif, sans-serif, an extra-readable font, or monospace.
- **Text size:** 60 to 250 %.
- **Line spacing, margins**, and **justification with hyphenation**.
- **Page colours:** light, sepia, dark and black.
- **Brightness** and a warm **night light**, layered over the page without changing the book.
- **Remembered** on each device, for every book.

### Study tools (from [WittyInk](https://github.com/wittywilla/WittyInk))

- **Highlights** in any colour, with **notes** and **colour meanings**. Highlights are tied to
  the words, not the page, so they survive font and layout changes.
- **Selection toolbar:** highlight, note, define, copy, cite, search, read aloud.
- **Notes panel:** filters by kind, colour or text; jump to any entry; **export to Markdown**.
- **Bookmarks** with a ribbon on the page.
- **Search the whole book**, with snippets; the match flashes when you jump to it.
- **Definitions** (when the app provides a dictionary), and **citations** with the chapter.
- **Read aloud**, paragraph by paragraph, turning pages and chapters as it goes.
- **Undo and redo.**

### Handwriting

- **Tools:** pen, pencil, highlighter and eraser, with **pen pressure**, pencil tilt, 36 ink and
  18 highlighter colours, thickness and opacity.
- **Palm rejection**, and a pen that writes at any time.
- **Highlighter that snaps to text:** a highlighter line across text becomes a real highlight
  covering whole words.
- **Strokes stay with their words** when the text reflows.

### Safe with any book

Chapters are shown in a sandboxed frame where **no script can run**. A strict
Content-Security-Policy blocks scripts, plugins and forms. Links never navigate the frame.

## Install

```bash
npm install github:wittywilla/WittyEbook
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. Needs React
18 or newer. WittyInk is installed with it.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

<HostContext.Provider value={host}><WittyEbook /></HostContext.Provider>
```

The host's `epub` field supplies the book: its chapters, contents and word counts, where each
chapter is loaded from, and search. See
[Embedding](https://github.com/wittywilla/WittyEbook/wiki/Embedding).

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyEbook. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyEbook, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
