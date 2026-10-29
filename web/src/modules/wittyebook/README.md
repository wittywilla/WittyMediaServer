# WittyEbook

A reader for reflowable ebooks (EPUB today, more formats later), built for long and careful
reading:

- **Book view:** two pages side by side, one page, or scrolling.
- **Typography:** font, size, line spacing, margins and justification.
- **Pages:** light, sepia, dark or black pages, with brightness and a warm night light.
- **Study tools:** highlights in any colour with notes, colour meanings, bookmarks, and
  footnotes in a pop-up.
- **Handwriting with pen pressure:** pen, pencil, highlighter and eraser
  (from [WittyInk](https://github.com/wittywilla/WittyInk)).
- **Reading aids:** search in the book, definitions, read aloud, citations and Markdown export.
- **Time left** in the chapter and the book, from the reader's own pace.
- **Exact resume:** the reading position is a place in the text, so it opens on the same words
  at any screen or font size.

WittyEbook runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and
the [WittyReader](https://github.com/wittywilla/WittyReader) app.

> This repository is a read-only mirror of `web/src/modules/wittyebook` in WittyMediaServer,
> where development happens. Please open issues and pull requests there.

## Install

```bash
npm install github:wittywilla/WittyEbook
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. React 18 or
newer. WittyInk is installed with it.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyEbook from '@wittywilla/wittyebook'

<HostContext.Provider value={host}><WittyEbook /></HostContext.Provider>
```

The host's `epub` field supplies the book:

```ts
epub: {
  info: { spine, toc, words },        // chapter paths, contents, words per chapter
  chapterUrl: (path) => string,       // where the frame loads a chapter from
  search: (q) => Promise<SearchHit[]>,
}
```

Chapters are loaded into a sandboxed frame with **no scripts**. Whatever serves them should also
send a strict Content-Security-Policy, for example:

```
default-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'
```

WittyMediaServer serves chapters from the server. WittyReader unpacks the book on the device and
serves it from a service worker with the same policy.

## License

Apache-2.0
