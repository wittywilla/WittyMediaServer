# Embedding WittyComics

WittyComics is a React component that gets everything through the
[reader host](https://github.com/wittywilla/WittyInk/wiki/Reader-host) from WittyInk. For a
comic, set `format: 'cbz'` and a `comic` field:

```ts
comic: {
  pages: 22,                                  // how many pages
  pageUrl: (n) => `/comics/123/page/${n}`,    // each page's image, 0-based, in reading order
}
reading: 'rtl',                               // optional first layout: 'ltr', 'rtl' or 'webtoon'
```

Notes:

- **Page addresses:**
  - **WittyMediaServer** serves pages from the archive on the server
    (`/api/items/{id}/page/{n}`).
  - **WittyReader** unpacks the archive on the device and uses `blob:` addresses, which it
    releases when the comic closes.
- **Order:** sort pages naturally (`page2` before `page10`), and leave out system files such as
  `__MACOSX/` and names starting with a dot.
- **Positions:** `start` and `saveProgress` use the page index; `total` is the page count.
- **Per-comic settings** are stored on the device under the host's `key`, so give each comic a
  stable key.
- WittyComics doesn't use `annotations`, `define` or `epub`, but a host can still provide them.
