# Embedding WittyEbook

WittyEbook is a React component. It gets everything through the
[reader host](https://github.com/wittywilla/WittyInk/wiki/Reader-host) from WittyInk. For a book,
the host needs `format: 'epub'` and an `epub` field:

```ts
epub: {
  info: {
    spine: string[]                        // chapter files in reading order, paths inside the EPUB
    toc: { title: string; href: string }[] // contents; href is a path, optionally with #anchor
    words: number[]                        // words in each chapter (for time left; zeros are fine)
  },
  chapterUrl(path: string): string         // where the frame loads a chapter (or any file in the book) from
  search(q: string): Promise<SearchHit[]>  // { chapter, snippet, quote, prefix }
}
```

## Serving chapters

Each chapter is loaded into a frame with `sandbox="allow-same-origin"`, so **no script in a book
can run**. The reader styles and reads the chapter from outside, so chapters must come from the
**same origin** as your app.

Chapters link to their images, styles and fonts by relative paths. `chapterUrl` must therefore
put every file of the book at its path under one base address. For example, if `chapterUrl` gives
`…/book/123/OEBPS/ch1.xhtml`, then `…/book/123/OEBPS/images/a.jpg` must be the book's image.

Send a strict Content-Security-Policy with every file:

```
default-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'
```

Two ways it's done today:

- **WittyMediaServer** serves files straight out of the EPUB with that header
  (`/api/items/{id}/epub/{path}`).
- **WittyReader** unpacks the book on the device and serves it from a service worker, with the
  header set on each stored response.

## Search

`search(q)` returns matches across the whole book:

- `chapter`: the spine index;
- `quote`: the matched text, exactly as written in the book;
- `prefix`: up to 40 characters before it, so the reader can find the right occurrence;
- `snippet`: the words around the match, shown in the results.

Ignore case, and return at most a few hundred matches.

## Positions and notes

- **Positions** are the chapter index plus the fraction of that chapter's text before the first
  word on the page (see [Positions](https://github.com/wittywilla/WittyInk/wiki/Reader-host#positions)).
- **Highlights** are stored by their quote and the text around it, with no `data`.
- **Handwriting** is stored as strokes tied to a character in the chapter, in ems.

## Requirements

- React 18 or newer.
- A bundler that compiles TypeScript and React (for example Vite with `@vitejs/plugin-react`).
- A current browser engine (Chromium-based, including WebView2, Safari or Firefox).
- Pen pressure needs Pointer Events with pressure. Current engines have them; without them, a pen
  draws at an even width.
