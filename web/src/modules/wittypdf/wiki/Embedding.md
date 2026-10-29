# Embedding WittyPDF

WittyPDF is a React component that gets everything through the
[reader host](https://github.com/wittywilla/WittyInk/wiki/Reader-host) from WittyInk. For a PDF,
set `format: 'pdf'` and a `pdf` field:

```ts
pdf: { url: 'https://example.com/paper.pdf' }   // streamed in parts if the server supports range requests
// or
pdf: { data: new Uint8Array(bytes) }            // a file already in memory (it's handed to pdf.js's worker)
```

Load WittyPDF lazily so pdf.js is only downloaded when needed:

```tsx
const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))

<HostContext.Provider value={host}>
  <Suspense fallback={<Spinner />}><WittyPDF /></Suspense>
</HostContext.Provider>
```

## Bundler

WittyPDF imports pdf.js's worker with Vite's `?url` suffix
(`pdfjs-dist/legacy/build/pdf.worker.min.mjs?url`), so use **Vite** (or a bundler that
understands that suffix). If your app also uses pdf.js directly, for example to make cover
images, add `resolve.dedupe: ['pdfjs-dist']` so both share one copy.

## What the host receives

- **Positions** (`start`, `saveProgress`): the page index plus how far down the page; `total` is
  the page count.
- **Highlights**: `chapter` is the first page, and `data` is JSON
  `[[page, x, y, width, height], …]` (fractions of the page, one box per line, possibly on
  several pages).
- **Ink**: `chapter` is the page, and `data` is the stroke `{ t, w, o, a: 0, pts }` in PDF
  points from the page's top-left corner.
- **Bookmarks**: `chapter` and `pos` are the page.

## Requirements

- React 18 or newer, and Vite.
- `pdfjs-dist` 4.10.38 (installed with WittyPDF).
- Any engine pdf.js's build for older browsers supports.
