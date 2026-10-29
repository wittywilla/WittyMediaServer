# WittyPDF

A PDF reader for papers, textbooks and scans:

- **Reading:** pages with selectable text, fit width, fit page or step zoom (Ctrl + scroll, + and
  −), one or two pages across, and pages as printed, sepia or dark.
- **Getting around:** the document's contents, page thumbnails, search across the whole
  document, go to page, and exact resume.
- **Study tools:** highlights in any colour with notes, colour meanings, bookmarks, definitions,
  citations with page numbers, read aloud and Markdown export.
- **Handwriting with pen pressure:** pen, pencil, highlighter and eraser
  (from [WittyInk](https://github.com/wittywilla/WittyInk)). Ink is kept in page units, so it
  stays put at any zoom.

Built on [pdf.js](https://mozilla.github.io/pdf.js/) (its legacy build, so older browsers and TV
web views work). Only pages near the screen are drawn, and their canvases are size-capped and
released when they scroll away, so long books stay light on low-end hardware.
**PDF scripting and XFA forms are off**, and fonts are never compiled with `eval`.

WittyPDF runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the
[WittyReader](https://github.com/wittywilla/WittyReader) app.

> This repository is a read-only mirror of `web/src/modules/wittypdf` in WittyMediaServer, where
> development happens. Please open issues and pull requests there.

## Install

```bash
npm install github:wittywilla/WittyPDF
```

TypeScript and React source, for **Vite** with `@vitejs/plugin-react`; the pdf.js worker is
included with Vite's `?url` import. React 18 or newer. pdf.js is large, so load WittyPDF lazily:

```tsx
import { lazy, Suspense } from 'react'
import { HostContext } from '@wittywilla/wittyink'
const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))

<HostContext.Provider value={host}>
  <Suspense fallback={null}><WittyPDF /></Suspense>
</HostContext.Provider>
```

The host's `pdf` field is what pdf.js opens: `{ url }` (range requests are used when the server
supports them) or `{ data }` for a file already in memory.

## License

Apache-2.0. The text-layer styles are adapted from pdf.js (Apache-2.0, Mozilla Foundation).
