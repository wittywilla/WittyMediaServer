# WittyComics

A reader for comics and manga:

- **Layouts:** one page, a two-page spread (the cover shown alone), or a vertical strip for
  webtoons.
- **Direction:** left to right or right to left (manga), fit to height or width.
- **Controls:** tap zones, swipes and keys that follow the reading direction.
- **Smooth reading:** the next pages are fetched ahead.

WittyComics runs inside [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and
the [WittyReader](https://github.com/wittywilla/WittyReader) app, with the host interface from
[WittyInk](https://github.com/wittywilla/WittyInk).

> This repository is a read-only mirror of `web/src/modules/wittycomics` in WittyMediaServer,
> where development happens. Please open issues and pull requests there.

## Install

```bash
npm install github:wittywilla/WittyComics
```

TypeScript and React source, for a bundler such as Vite with `@vitejs/plugin-react`. React 18 or
newer.

## Use

```tsx
import { HostContext } from '@wittywilla/wittyink'
import WittyComics from '@wittywilla/wittycomics'

<HostContext.Provider value={host}><WittyComics /></HostContext.Provider>
```

The host's `comic` field lists the pages: `{ pages: number, pageUrl: (n) => string }`.
`reading` can be `ltr`, `rtl` or `webtoon` to choose the first layout (each book remembers the
reader's own choice).

## License

Apache-2.0
