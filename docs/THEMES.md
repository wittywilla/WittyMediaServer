# Themes

A theme changes how WittyMediaServer looks: its colours, its corners, even its font. This guide helps you pick one, or make your own in a few lines with no code.

## Changing your theme

1. Open **Settings → Appearance**.
2. Click a theme. It applies straight away, on this device only, and other devices aren't affected.

**Automatic** follows your device's light or dark setting. **Follow the server's** puts this device back on the server's theme. If you're an administrator, **Make default** sets the theme everyone starts with, including on the sign-in page.

| Theme | Looks like |
|---|---|
| Witty | The house look: deep slate with a violet accent |
| Midnight | Blue-black, with a clear blue accent |
| Cinema | Warm near-black and amber, for a dim room |
| Evergreen | Dark forest tones with a fresh green |
| True black | Pure black, kind to OLED screens at night |
| Light | Clean, bright grey with a violet accent |
| Paper | Warm off-white and rust, easy on the eyes by day |
| High contrast | Black and white with bright yellow, for the clearest reading |

If you were using one of the older colour themes, such as Dark Red, you've been moved to the closest of these. A theme you made yourself stays.

## Making a theme in the app

This is the quickest way, and the result is always easy to read.

1. On **Settings → Appearance**, press **Make a theme**.
2. Give it a **Name**, and choose **Dark** or **Light**.
3. Pick an **Accent**, the colour of buttons and highlights, and a **Background tint**, a hint of colour in the page.
4. Choose **Corners**: **Crisp**, **Soft** or **Round**.
5. Press **Use on this device**. If you're an administrator, you can press **Add for everyone** instead.

The app works out every other shade and checks that the text stays readable. If your accent would make the words on a button hard to read, it's adjusted a little and the app says so.

## Making your own by hand

A theme is a ZIP with two small text files. CSS is only a list of settings, so you don't need to have written it before.

### The shortest working theme

Make a new folder with these two files in it. Any text editor will do.

`theme.json`, which names it:

```json
{
  "id": "my-theme",
  "name": "My theme",
  "scheme": "dark"
}
```

`theme.css`, which sets its colours:

```css
:root {
  color-scheme: dark;
  --bg: #14090b;     --bg-rgb: 20 9 11;
  --accent: #d4a017; --accent-rgb: 212 160 23;
  --accent-2: #b98b12;
  --accent-fg: #17120a;
  --accent-text: #e0b542;
}
```

Then [pack it as a ZIP](#packing-it-as-a-zip) and install it. Everything you leave out keeps its normal value, and hover and tinted shades are worked out for you.

### The settings most people change

A colour like `#d4a017` is a code. Search the web for "colour picker", choose a colour and copy its code in.

| Setting | What it changes |
|---|---|
| `--bg` | The page background |
| `--bg-2`, `--bg-3`, `--bg-4` | Panels and side menu, then fields and cards, then hovered things |
| `--fg`, `--fg-2`, `--fg-3` | Main text, secondary text, hints and captions |
| `--accent` | Buttons, highlights, progress bars and selected items |
| `--accent-2` | The accent when hovered or pressed |
| `--accent-fg` | Text on the accent |
| `--accent-text` | The accent as words: links and highlighted text |
| `--radius` | Corners everywhere. `0` makes everything square |
| `--font` | Every piece of text |

**Two to keep in step:** `--bg-rgb` is `--bg` written as three numbers, and `--accent-rgb` is `--accent` the same way. A colour picker shows these too, as RGB.

**For a light theme,** set `color-scheme: light` and `"scheme": "light"`, and choose dark text on light surfaces.

**To keep it readable,** text needs to stand out from what's behind it. The bar is a contrast of 4.5:1 for text. Search the web for "contrast checker", and test `--fg-3` and `--accent-text` against `--bg-3`, and `--accent-fg` against `--accent`. The built-in themes all pass.

### Starting from a built-in theme

To change a theme that's nearly right, press the download button on its card in **Settings → Appearance**. Unzip it into a new folder. Then, in `theme.json`, change `id` and `name`, and delete the `"builtin": true,` line, as a ZIP with that line is refused. `theme.css` has a note beside each setting.

## TV looks

WittyTV has themes of its own, and an app theme can suggest one, with one more setting in `:root`:

```css
:root {
  --tv-theme: modern;  /* modern, classic, satellite, teletext, listings, bigpicture, outline or compact */
}
```

Modern, Big picture, Outline and Compact take their colours from your theme's tokens, so they look like part of it. The others have their own.

It's only a suggestion. What a viewer picks comes first, then the app theme's suggestion, then what the server's owner chose for new viewers, and **Look** on the guide says which is showing. Custom can't be suggested, because it's each viewer's own. The built-in Midnight theme suggests Modern this way. A theme from before there were TV themes may set `--tv-layout` and `--tv-colours` instead, and those still work. See [Channels and live TV](CHANNELS.md#the-look).

## Packing it as a ZIP

`theme.json` and `theme.css` must be at the top of the ZIP, not inside a folder.

- **Windows:** select the files, right-click, and choose **Compress to ZIP file**.
- **macOS:** select the files, right-click, and choose **Compress**.
- **Linux or FreeBSD:** `cd my-theme && zip -r ../my-theme.zip .`

To install it, press **Install a theme ZIP…** on **Settings → Appearance**. You need to be an administrator. To change it later, install a ZIP with the same `id` again. To take it away, press **Remove** on its card.

You can also copy the ZIP into the server's `themes` folder and reload the page. The ZIP's file name is then the theme's id.

| Install | Themes folder |
|---|---|
| Windows | `%LOCALAPPDATA%\WittyMediaServer\themes` |
| macOS | `~/Library/Application Support/WittyMediaServer/themes` |
| Linux, your own account | `~/.local/share/wittymediaserver/themes` |
| Linux package, as a service | `/var/lib/wittymediaserver/themes` |
| FreeBSD package, as a service | `/var/db/wittymediaserver/themes` |
| Docker | `/config/themes` |
| A data folder you chose | `<data>/themes` |

## Limits

- **No remote images or fonts.** A theme uses its own files. A stylesheet that names an address on another site is refused, even in a comment, because a stylesheet that can load from elsewhere can read what's typed into a page. That includes `//` anywhere in it, so don't use that for comments. Pictures kept in the stylesheet as `data:` addresses are fine.
- **No scripts.** A theme can hold only `.css .json .png .jpg .jpeg .webp .gif .avif .svg .woff2 .woff .ttf .otf .txt .md`, and anything else makes the install fail.
- **Size.** ZIPs up to 25 MB, each file up to 10 MB, and up to 200 files. Keep images small, as WebP and AVIF give the smallest files.
- **File names** are plain relative paths: no `..`, no absolute paths, no backslashes.
- **Ids** are letters, digits, `.`, `-` and `_`, up to 64 characters. A built-in theme's id is taken, so give your copy a new one. A built-in theme can't be removed.
- **Who can install.** Only administrators can install or remove themes on the server. Anyone can use **Make a theme** on their own device.
- **Theme files are public,** because the sign-in page is themed too, so don't put anything private in one.
- **Apps use the colours.** The native apps read a theme's settings, not its own CSS rules or pictures, so a theme that has those looks plainer there.

Viewers using a theme that has been removed go back to the server's theme after a reload. Check that the licence of any font or image lets you share it.

## Going further

Every colour, corner and shadow goes through the settings above, so most themes need nothing else. For more, here's what you can do.

### Pictures and fonts

Put them in the ZIP and use relative addresses:

```
my-theme.zip
  theme.json
  theme.css
  assets/backdrop.webp
  assets/fonts/MyFont.woff2
```

```css
@font-face {
  font-family: "MyFont";
  src: url("assets/fonts/MyFont.woff2") format("woff2");
  font-display: swap;
}
:root { --font: "MyFont", Georgia, serif; }
body { background: var(--bg) url("assets/backdrop.webp") center / cover fixed; }
```

### Recipes

**Only change the accent:**

```css
:root { --accent: #e50914; --accent-2: #b20710; --accent-rgb: 229 9 20; --accent-text: #ff5a5f; }
```

**Square and flat:**

```css
:root { --radius: 0; --shadow: none; --card-shadow: none; }
```

**Bigger posters:**

```css
:root { --card-w: 200px; }
```

**Softer titles on a light theme,** since titles over artwork carry a text shadow:

```css
.hero h1, .detail h1 { text-shadow: 0 1px 6px rgba(255, 255, 255, .6); }
```

**Your own logo:**

```css
.brand img { content: url("assets/logo.svg"); width: 36px; height: 36px; }
```

**Only the logo, no name:**

```css
.brand span, .brand .brand-name { display: none; }
```

**A different accent in the player only:**

```css
.player { --accent: #ffcc00; --accent-rgb: 255 204 0; }
```

**Glassy panels over a wallpaper.** These look good, but a blur repaints on every frame of scrolling, so expect them to stutter on TVs and old phones:

```css
body { background: #000 url("assets/wallpaper.webp") center / cover fixed; }
.panel, .sidebar, .topbar, .modal { background: rgb(20 22 30 / .72); backdrop-filter: blur(14px); }
```

The name in the corner isn't part of a theme. To change it, use **Name and logo** on **Settings → Appearance**. The license notice in **Settings → About** always remains.

## Every setting

Set these in `:root`. Hover, selected and tinted shades are worked out from them. The sizes, spacing and motion the interface is built from are settings you can change too, such as `--card-w` or `--text-base`: see [Design standards](DESIGN.md).

| Setting | Normal value | Used for |
|---|---|---|
| `--bg` | `#0f1115` | Page background |
| `--bg-rgb` | `15 17 21` | `--bg` as R G B, for the fades over artwork: `rgb(var(--bg-rgb) / .6)`. Keep it in step with `--bg` |
| `--bg-2` | `#171a21` | Side menu, panels, dialogs |
| `--bg-3` | `#1f232c` | Inputs, selects, cards, hovered rows, tiles |
| `--bg-4` | `#2a2f3a` | Hover on `--bg-3`, chip outlines, scrollbar thumb |
| `--border` | 10% of `--fg` | Hairlines around panels, lists and tiles. Built on `--fg` it shows on any background |
| `--border-strong` | 20% of `--fg` | Emphasised lines |
| `--control-line` | half of `--fg` | Outlines of fields and switches. Keep it 3:1 on `--bg`, `--bg-2` and `--bg-3` |
| `--fg` | `#e8eaf0` | Main text |
| `--fg-2` | `#a9afbd` | Secondary text: meta lines, overviews, inactive tabs |
| `--fg-3` | `#848b99` | Hints, captions, table headers, section labels |
| `--accent` | `#7955ff` | Primary buttons, progress bars, selected items |
| `--accent-2` | `#6a4df0` | Hover and pressed state of accent buttons |
| `--accent-fg` | `#fff` | Text and icons on the accent |
| `--accent-text` | `--accent` | The accent as text. Keep it readable on `--bg`, `--bg-2` and `--bg-3` |
| `--accent-rgb` | `121 85 255` | `--accent` as R G B for glows: `rgb(var(--accent-rgb) / .25)`. Keep it in step with `--accent` |
| `--brand-accent` | `--accent-text` | The highlighted "Media" in the default name |
| `--danger` | `#ff5c6c` | Errors, delete buttons |
| `--ok` | `#3ad29f` | Success messages |
| `--warn` | `#e0a030` | Warnings, the rating star |
| `--radius` | `10px` | The whole interface's corners: every other radius is worked out from it |
| `--shadow` | `0 10px 30px rgba(0,0,0,.45)` | Panels, dialogs, popovers |
| `--card-shadow` | `0 10px 24px rgba(0,0,0,.5)` | Poster cards on hover |
| `--overlay` | `rgba(0,0,0,.6)` | Backdrop behind dialogs |
| `--sidebar` | `240px` | Side menu width |
| `--card-w` | `148px` | Poster card width |
| `--font` | `"Inter", "Segoe UI Variable Text", system-ui, …` | Every piece of text |
| `--tv-theme` | not set | See [TV looks](#tv-looks) |

`color-scheme` (`dark` or `light`) in `:root` tells the browser how to draw its own controls.

## Every field in theme.json

| Field | Required | Meaning |
|---|---|---|
| `name` | Yes | Shown in the theme picker |
| `id` | No | Defaults to the ZIP's name. Installing with the same id replaces it |
| `scheme` | No | `dark` (the default) or `light` |
| `description`, `author`, `version` | No | Shown in the picker, or for your records |
| `colors`, `radius` | No | The colours as data, which draws the picker's preview exactly |
| `preview` | No | Four colours for the preview if `colors` isn't there: `bg`, `surface`, `text`, `accent` |
| `builtin` | | Reserved: a ZIP with this is refused |

## Every class

The interface is built from these classes. Combine them with the settings above. The built-in stylesheet loads first, then your `theme.css`, so anything you set wins, and `<html>` has `data-theme="<id>"` if you want to scope a rule. The video player is always dark, so that video looks right with light themes, and you can still restyle it under `.player`.

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

### Detail pages

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
| `.insert-disc`, `.insert-disc .disc`, `.insert-disc .tray` | *Insert DVD* animation |
| `.big-play`, `.center-spinner`, `.overlay-msg` | click-to-play button, buffering spinner, messages |

### Breakpoints

The built-in stylesheet switches to the mobile layout at `max-width: 900px` (the side menu becomes a drawer) and tightens the player and episode list at `max-width: 700px`. Use the same widths in your `@media` rules.

## For developers

`python scripts/theme-contrast.py` checks the built-in themes against the contrast rules, and `--fix` adjusts them. `scripts/design-tokens.py` writes `colors` and `radius` in `theme.json` for the built-ins, which are written out as ZIPs at start-up when their `version` changes.

Theme files need no sign-in: `GET /api/themes`, `GET /api/themes/<id>/theme.css`, `GET /api/themes/<id>/<file>`, `GET /api/themes/<id>/tokens.json` and `GET /api/themes/<id>/download`. Making a theme on the server is `POST /api/themes/custom` (administrators), which accepts only plain `#rrggbb` colours. See [Client API](CLIENT-API.md#themes-and-design-data).

---

<sub>WittyMediaServer documentation</sub>
