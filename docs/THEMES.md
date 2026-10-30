# Themes

WittyMediaServer's look is fully themeable with plain CSS. A theme is a **ZIP file** in the
server's `themes` folder; the server ships 20 ready-made themes in the same format, and you can
download any of them as a starting point for your own.

- [Choosing a theme](#choosing-a-theme)
- [Changing the name in the corner](#changing-the-name-in-the-corner)
- [Where themes live](#where-themes-live)
- [Anatomy of a theme ZIP](#anatomy-of-a-theme-zip)
- [theme.json reference](#themejson-reference)
- [How theme CSS is applied](#how-theme-css-is-applied)
- [Variables reference](#variables-reference)
- [Selector reference](#selector-reference)
- [Images and fonts](#images-and-fonts)
- [Recipes](#recipes)
- [Light themes](#light-themes)
- [Installing, updating and removing](#installing-updating-and-removing)
- [Rules, limits and security](#rules-limits-and-security)
- [Developing a theme quickly](#developing-a-theme-quickly)
- [Built-in themes](#built-in-themes)

## Choosing a theme

*Settings → Appearance*:

- **Use here** applies a theme to *this browser/device only* (remembered in the browser).
  *Follow the server default* undoes it.
- **Make default** (administrators) sets the theme everyone gets unless they chose their own,
  including the sign-in page.
- **ZIP** downloads the theme — the easiest way to start your own.
- **Install theme ZIP…** (administrators) uploads a theme.

## Changing the name in the corner

*Settings → Appearance → Name* (administrators) replaces "WittyMediaServer" in the sidebar, on
the sign-in page and in the browser tab title. Leave it empty to go back to the default. The
Apache-2.0 attribution in *Settings → About & legal* always remains, as the licence requires.

A theme can additionally restyle the name: it is rendered as `.brand .brand-name` (custom name)
or `.brand span` with `.brand span b` for the highlighted "Media" (default name); the logo is
`.brand img`. See [Replace the logo](#replace-the-logo).

## Where themes live

| Install | Themes folder |
|---|---|
| Windows (tray app / service) | `%LOCALAPPDATA%\WittyMediaServer\themes` |
| macOS | `~/Library/Application Support/WittyMediaServer/themes` |
| Linux / FreeBSD | `~/.local/share/wittymediaserver/themes` (service: `/var/lib/wittymediaserver/themes`) |
| Docker / Kubernetes | `/config/themes` |
| Custom `-data` / `WMS_DATA_DIR` | `<data>/themes` |

Every `*.zip` in that folder is a theme; the file name (without `.zip`) is the theme **id**.
Folders and loose `.css` files are ignored — **themes must be packed as ZIPs**, which keeps the
folder tidy and makes a theme a single file to share, back up or delete.

The built-in themes are (re)written into this folder at every start-up; a newer WittyMediaServer
version updates them. Don't edit a built-in ZIP in place — copy it under a new name (and a new
`id`) instead.

## Anatomy of a theme ZIP

```
my-cinema.zip
├── theme.json          required: name and colour scheme
├── theme.css           required: your CSS
├── assets/             optional: anything theme.css references
│   ├── backdrop.jpg
│   ├── logo.svg
│   └── fonts/Cinzel.woff2
└── README.md           optional: notes for people using your theme
```

`theme.json` and `theme.css` must be at the **top level** of the ZIP (not inside a folder).
On Windows: select the files *inside* your theme folder → right-click → *Send to → Compressed
(zipped) folder*. On macOS/Linux: `cd my-cinema && zip -r ../my-cinema.zip .`

Allowed file types: `.css .json .png .jpg .jpeg .webp .gif .avif .svg .woff2 .woff .ttf .otf
.txt .md`. Anything else (scripts, HTML, executables) makes the upload fail.

## theme.json reference

```json
{
  "id": "my-cinema",
  "name": "My Cinema",
  "description": "Velvet red and gold, like an old movie palace.",
  "author": "Your Name",
  "version": "1.0",
  "scheme": "dark",
  "preview": { "bg": "#14090b", "surface": "#2a1216", "text": "#f6e7d0", "accent": "#d4a017" }
}
```

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | shown in the theme picker |
| `id` | no | the theme id (letters, digits, `.`, `-`, `_`; max 64). Defaults to the uploaded file name. Determines the stored file name `<id>.zip`. Installing a theme with an existing id replaces it |
| `scheme` | no | `dark` (default) or `light`; used by the picker's *Dark / Light* filter. Also set `color-scheme` in your CSS (see [Light themes](#light-themes)) |
| `description`, `author`, `version` | no | shown in the picker / for your records |
| `preview` | no | the four colours of the picker's miniature: page background, card surface, text and accent |
| `builtin` | — | reserved for the shipped themes; uploads containing it are rejected |

## How theme CSS is applied

1. The built-in stylesheet loads first and defines every [variable](#variables-reference)
   with the default (violet, dark) values on `:root`.
2. Your `theme.css` loads **after** it as `<link id="wms-theme" href="/api/themes/<id>/theme.css">`,
   so anything you declare wins at equal specificity.
3. `<html>` gets `data-theme="<id>"`, so a theme can also scope rules:
   `html[data-theme="my-cinema"] .hero { … }`.

Most themes only need to change variables. Every colour, radius and shadow in the interface goes
through them, so a 20-line `:root { … }` block restyles the whole app. Use selectors for layout
or decoration beyond that.

The **video player is always dark** (it re-declares the text and surface variables on
`.player`) so videos look right with light themes. You can still restyle it with selectors
under `.player`.

## Variables reference

Declare these on `:root`. The sizes, spacing and motion the interface is built from are design
tokens that themes can override too, for example `--card-w` or `--text-base`: see [DESIGN.md](DESIGN.md).
Hover, selected and tinted colours are worked out from the colours below.

| Variable | Default | Used for |
|---|---|---|
| `--bg` | `#0f1115` | page background, body |
| `--bg-rgb` | `15 17 21` | `--bg` as space-separated R G B, for translucent shades: `rgb(var(--bg-rgb) / .6)` — the hero and detail-page fades over artwork. **Keep it in sync with `--bg`** |
| `--bg-2` | `#171a21` | sidebar, panels, modals, top bar |
| `--bg-3` | `#1f232c` | inputs, selects, cards, hovered rows, active nav item, theme/season tiles |
| `--bg-4` | `#2a2f3a` | borders, chip outlines, scrollbar thumb, button hover |
| `--border` | 10% of `--fg` | hairlines around panels, lists and tiles. Built on `--fg` it shows on any background; a darker-than-the-page colour disappears on dark themes |
| `--control-line` | half of `--fg` | outlines of fields and switches; keep it 3:1 on `--bg`, `--bg-2` and `--bg-3` (WCAG 1.4.11; `theme-contrast.py --fix` sets it) |
| `--fg` | `#e8eaf0` | primary text |
| `--fg-2` | `#a9afbd` | secondary text (meta lines, overviews, inactive tabs) |
| `--fg-3` | `#6f7686` | hints, captions, table headers, section labels |
| `--accent` | `#8b6cff` | primary buttons, active tabs/chips, progress bars, selected items, focus rings, DVD menu fallback highlight |
| `--accent-2` | `#6a4df0` | hover/pressed state of accent buttons |
| `--accent-fg` | `#fff` | text and icons on accent backgrounds |
| `--accent-text` | `--accent` | the accent as text (links, highlighted words, the active item); keep it readable on `--bg`, `--bg-2` and `--bg-3` |
| `--accent-rgb` | `139 108 255` | `--accent` as R G B for glows: `rgb(var(--accent-rgb) / .25)`. **Keep it in sync with `--accent`** |
| `--brand-accent` | `var(--accent-text)` | the highlighted "Media" in the default name |
| `--danger` | `#ff5c6c` | errors, delete buttons |
| `--ok` | `#3ad29f` | success messages |
| `--warn` | `#e0a030` | warnings, the rating star |
| `--radius` | `10px` | the whole interface's corners: every other radius is worked out from it, so `0` makes everything square |
| `--shadow` | `0 10px 30px rgba(0,0,0,.45)` | panels, modals, popovers |
| `--card-shadow` | `0 10px 24px rgba(0,0,0,.5)` | poster cards on hover |
| `--overlay` | `rgba(0,0,0,.6)` | backdrop behind dialogs |
| `--sidebar` | `240px` | sidebar width |
| `--font` | `"Inter", system-ui, …` | every text |

`color-scheme` (`dark` or `light`) on `:root` tells the browser how to draw native controls
(scrollbars, date pickers, the file dialog button, autofill).

## Selector reference

The interface is built from these classes. Combine them with the variables above.

### Layout

| Selector | Element |
|---|---|
| `.app`, `.app.nav-open` | the whole signed-in interface; `nav-open` while the sidebar shows |
| `.sidebar` | left navigation column |
| `.brand`, `.brand img`, `.brand span`, `.brand span b`, `.brand .brand-name` | logo and name in the top-left corner |
| `.nav-section` | "Libraries" / "Manage" headings |
| `.nav-link`, `.nav-link.active`, `.nav-link svg` | sidebar links |
| `.sidebar-footer` | version line |
| `.main` | everything right of the sidebar |
| `.topbar`, `.topbar .search`, `.avatar` | top bar, search box, user circle |
| `.content` | the scrolling page area |
| `.toolbar`, `.toolbar h1`, `.toolbar .count` | page title row (library, settings, search) |
| `.backdrop-dim` | dimmer behind the mobile sidebar |

### Home, libraries and cards

| Selector | Element |
|---|---|
| `.hero`, `.hero .bg`, `.hero .shade`, `.hero .info`, `.hero h1`, `.hero .meta`, `.hero .dots` | the rotating featured banner |
| `.hub`, `.hub-head` | a home row and its title |
| `.row` | horizontally scrolling row of cards |
| `.grid` | library grid |
| `.card`, `.card.wide` | poster card (`wide` = 16:9 episode card) |
| `.card .poster`, `.card .poster.generated`, `.card .poster img`, `.card .poster .ph` | artwork box, generated-thumbnail variant, image, placeholder text |
| `.card .play`, `.card .badge`, `.card .badge.disc`, `.card .badge.count`, `.card .progress i` | hover play button, badges (DVD, unwatched count, ✓), resume bar |
| `.card .title`, `.card .sub` | caption lines |
| `.chips`, `.chip`, `.chip.active` | genre filters |
| `.empty` | empty-state message |

### Detail pages (movies, discs, shows, episodes)

| Selector | Element |
|---|---|
| `.detail-hero`, `.detail-hero .bg`, `.detail-hero .shade` | backdrop at the top |
| `.detail`, `.detail .poster`, `.detail h1`, `.detail .tagline`, `.detail .meta`, `.pill` | layout, poster, title, tagline, meta line, rating pill |
| `.detail .actions`, `.disc-actions`, `.disc-actions .insert`, `.disc-hint` | button row; DVD variant with *Insert DVD* |
| `.overview`, `.facts`, `.fact .k`, `.fact .v` | overview text and the facts grid |
| `.crumbs` | Show › Season › Episode breadcrumb |
| `.section-title` | sub-headings |
| `.titles-table`, `.titles-table .main-row` | disc title list; main feature row |
| `.chapters` | chapter buttons |
| `.season-tabs`, `.season-tab`, `.season-tab.active`, `.season-poster` | season picker |
| `.view-toggle`, `.seg`, `.seg button.active` | Cards / Playlist toggle (and other segmented controls) |
| `.episodes`, `.episode`, `.episode .thumb`, `.episode .num` | episode cards |
| `.episode-table` | compact playlist layout |

### Controls and forms

| Selector | Element |
|---|---|
| `.btn`, `.btn.primary`, `.btn.ghost`, `.btn.danger`, `.btn.sm`, `.btn.icon` | buttons |
| `.input`, `.select`, `textarea.input` | form fields |
| `.form`, `.row2`, `.field-row` | form layouts |
| `.panel` | settings card |
| `.tabs`, `.tabs a.active` | settings tabs |
| `.list`, `.list-item` | settings lists (libraries, users) |
| `.modal-backdrop`, `.modal`, `.modal.wide` | dialogs |
| `.tip`, `.tip-body` | "?" tooltips |
| `.toast` | notifications |
| `.error`, `.ok`, `.muted` | message and text helpers |
| `.spinner` | loading indicator |
| `.art-grid`, `.art-choice` | artwork picker |
| `.theme-grid`, `.theme-card`, `.theme-card.active` | theme picker |
| `.hw-list`, `.hw-item`, `.profile` | transcoding settings |
| `.auth-card` | sign-in / first-run card |

### Player

| Selector | Element |
|---|---|
| `.player`, `.player.controls-hidden` | full-screen player; `controls-hidden` after inactivity |
| `.player .stage`, `.player video` | video area |
| `.player .top`, `.player .bottom` | top bar and control bar (fade out when idle) |
| `.seek`, `.seek .fill`, `.seek .buf`, `.seek .knob`, `.seek .chap` | timeline, played part, buffered part, handle, chapter ticks |
| `.ibtn` | round icon buttons |
| `.popover` | audio / subtitle / chapter menus |
| `.remote`, `.remote .rk`, `.remote-dpad`, `.remote-dpad .enter`, `.remote-digits` | on-screen DVD remote |
| `.dvd-layer`, `.dvd-layer .hit.selected.fallback` | DVD menu buttons; the outline used when a disc has no highlight artwork |
| `.sub-layer` | DVD subtitles |
| `.skip-intro`, `.watch-next` | Skip intro button, Watch next card |
| `.insert-disc`, `.insert-disc .disc`, `.insert-disc .tray`, `.insert-disc .drive-face` | *Insert DVD* animation |
| `.big-play`, `.center-spinner`, `.overlay-msg` | click-to-play button, buffering spinner, messages |

### Breakpoints

The built-in stylesheet switches to the mobile layout at `max-width: 900px` (sidebar becomes a
drawer) and tightens the player and episode list at `max-width: 700px`. Use the same widths in
your `@media` rules.

## Images and fonts

Reference files in your ZIP with **relative** URLs; they are served from
`/api/themes/<id>/…`:

```css
@font-face {
  font-family: "Cinzel";
  src: url("assets/fonts/Cinzel.woff2") format("woff2");
  font-display: swap;
}
:root { --font: "Cinzel", Georgia, serif; }
body { background: var(--bg) url("assets/backdrop.jpg") center / cover fixed; }
```

Keep images reasonably small (each file ≤ 10 MB, the whole ZIP ≤ 25 MB); WebP/AVIF give the
smallest files. Google Fonts and other external URLs also work in `@import`/`url()`, but then
every viewer's browser contacts that site — bundle fonts in the ZIP for privacy and offline use.

## Recipes

### Just change the colours

```css
:root {
  --accent: #e50914;
  --accent-2: #b20710;
  --accent-rgb: 229 9 20;
}
```

### Square, flat look

```css
:root { --radius: 0; --shadow: none; --card-shadow: none; }
```

### Bigger posters

```css
:root { --card-w: 200px; }
```

### Replace the logo

```css
.brand img { content: url("assets/logo.svg"); width: 36px; height: 36px; }
```

### Hide the name and keep only the logo

```css
.brand span, .brand .brand-name { display: none; }
```

### Wider sidebar with a gradient

```css
:root { --sidebar: 280px; }
.sidebar { background: linear-gradient(180deg, #1b1030, var(--bg-2)); }
```

### Glassy panels over a wallpaper

Pretty, but a blur repaints on every frame of scrolling: expect it to stutter on TVs and old phones.

```css
body { background: #000 url("assets/wallpaper.webp") center / cover fixed; }
.panel, .sidebar, .topbar, .modal { background: rgb(20 22 30 / .72); backdrop-filter: blur(14px); }
```

### Different accent for the player only

```css
.player { --accent: #ffcc00; --accent-rgb: 255 204 0; }
```

### Style the DVD remote like a silver remote

```css
.remote { background: linear-gradient(180deg, #d9dce2, #a9aeb8); }
.remote .rk { color: #111; background: linear-gradient(180deg, #f2f3f5, #c8ccd4); box-shadow: 0 2px 0 #7d828c; }
```

## Light themes

Set both the variables and the colour scheme:

```css
:root {
  color-scheme: light;
  --bg: #f4f5f8; --bg-rgb: 244 245 248;
  --bg-2: #ffffff; --bg-3: #eaedf2; --bg-4: #dadfe7; --border: #dfe3ea;
  --fg: #15171c; --fg-2: #4a5160; --fg-3: #7c8494;
  --shadow: 0 10px 30px rgba(20, 25, 40, .12);
  --card-shadow: 0 10px 24px rgba(20, 25, 40, .18);
  --overlay: rgba(20, 24, 32, .45);
}
```

Use accents dark enough for white text (`--accent-fg: #fff`), or set `--accent-fg` to a dark
colour for yellow/lime accents. Aim for WCAG AA contrast: 4.5:1 for `--fg-3`, `--accent-text`, `--danger`, `--ok` and `--warn` on every
surface, 3:1 for `--control-line`, and for `--accent-fg` on `--accent`; the built-in themes all pass, as
`python scripts/theme-contrast.py` checks. Titles over artwork carry a text shadow; the built-in light themes
soften it with `.hero h1, .detail h1 { text-shadow: 0 1px 6px rgba(255,255,255,.6) }`.

## Installing, updating and removing

- **Install**: *Settings → Appearance → Install theme ZIP…*, or copy the ZIP into the themes
  folder and reload the page.
- **Update**: install a ZIP with the same `id` (or overwrite the file). Browsers re-check theme
  files on every load, so viewers get the new version on their next page load.
- **Remove**: *Remove* on the theme card, or delete the ZIP. Built-in themes can't be removed
  (they're recreated at start-up). Viewers using a removed theme fall back to the server default
  after a reload.

## Rules, limits and security

- Top-level `theme.json` and `theme.css` are required; ZIPs ≤ 25 MB, files ≤ 10 MB, ≤ 200 files.
- File names must be plain relative paths (no `..`, no absolute paths, no backslashes).
- Only CSS, JSON, images, fonts and text files are accepted — themes cannot contain scripts.
  Theme files are served with `X-Content-Type-Options: nosniff` and a Content-Security-Policy
  that blocks scripts (also inside SVGs).
- Only administrators can install or remove themes. CSS can load images and fonts from other
  sites (`url(https://…)`), which tells those sites when someone opens WittyMediaServer — only
  install themes you trust, and bundle assets in the ZIP.
- Theme endpoints are public (the sign-in page is themed too): `GET /api/themes`,
  `GET /api/themes/<id>/theme.css`, `GET /api/themes/<id>/<file>`, `GET /api/themes/<id>/download`.

## Developing a theme quickly

1. Download a built-in theme close to what you want (*ZIP* button) and unzip it into a folder.
2. Edit `theme.json` (new `id` and `name`) and `theme.css`.
3. While iterating, open your browser's developer tools on WittyMediaServer, select the
   `<link id="wms-theme">` stylesheet, and edit rules live; inspect any element to find its
   class (see the [selector reference](#selector-reference)).
4. Zip the folder's *contents*, install, and reload. Repeat with the same `id` to update.

## Built-in themes

| Id | Name | Scheme | Accent |
|---|---|---|---|
| `default` | WittyMediaServer | dark | violet |
| `dark` | Dark | dark | soft white |
| `oled` | OLED Black | dark (true black) | violet |
| `dark-red`, `dark-blue`, `dark-green`, `dark-orange`, `dark-pink`, `dark-teal`, `dark-purple`, `dark-gold` | Dark *colour* | dark | as named |
| `light` | Light | light | violet |
| `light-red`, `light-blue`, `light-green`, `light-orange`, `light-pink`, `light-teal`, `light-purple`, `light-gold` | Light *colour* | light | as named |

Their sources live in [`themes/`](../themes) in the repository; each `theme.css` lists every
variable with a comment and is a good template.

---
<sub>WittyMediaServer 0.4.0-alpha documentation</sub>
