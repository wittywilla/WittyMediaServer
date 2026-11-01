# Themes

A theme changes how WittyMediaServer looks: its colours, its corners, even its font. You don't need to
know any code to make one. There are three ways in, from easiest to most powerful.

- [Choosing a theme](#choosing-a-theme)
- [Make a theme in the app](#make-a-theme-in-the-app): two minutes, no code
- [Make a theme by hand](#make-a-theme-by-hand): ten minutes, never touched CSS before
- [Going further](#going-further): recipes, pictures and fonts
- [Suggesting a look for the TV guide](#suggesting-a-look-for-the-tv-guide)
- [Reference](#reference): every variable and class
- [Rules and limits](#rules-and-limits)

## Choosing a theme

Open **Settings, Appearance**.

- **Click a theme** to use it on this device. Each card is a small piece of the real app in that
  theme's colours, so what you see is what you get. Other devices aren't affected.
- **Automatic** follows your device's light or dark setting.
- **Follow the server's** puts this device back on the server's theme.
- **Make default** (administrators) sets the theme everyone starts with, including the sign-in page.

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

If you were using one of the older colour themes, such as Dark Red, you've been moved to the closest of
these. A theme you made yourself stays, whatever it's called.

## Make a theme in the app

The quickest way, and the result is always easy to read.

1. Open **Settings, Appearance** and press **Make a theme**.
2. Give it a name.
3. Choose **Light** or **Dark**.
4. Pick an **Accent**, the colour of buttons and highlights.
5. Pick a **Background tint**, a hint of colour in the page. Use the same colour as the accent for
   something coordinated, or grey for something neutral.
6. Choose your **Corners**: crisp, soft or round.
7. Press **Use on this device**. Administrators can press **Add for everyone** instead, which puts it on
   the theme list for the whole server.

As you choose, the preview shows the result. The app works out every other shade for you and checks
that all the text stays readable. If your accent would make the words on a button hard to read, it's
adjusted a little and the app says so.

## Make a theme by hand

You don't need to have written CSS before. CSS is just a list of settings, and that's all this is.

**1. Get a starting point.** In **Settings, Appearance**, press the download button on the theme closest
to what you want. You get a ZIP file. Unzip it into a new folder. Inside are two files:

```
theme.json    the theme's name
theme.css     its colours
```

**2. Name it.** Open `theme.json` in any text editor (Notepad is fine). Change these three lines, and
delete the `"builtin": true,` line and everything from `"colors"` on:

```json
{
  "id": "my-theme",
  "name": "My Theme",
  "scheme": "dark"
}
```

**3. Change the colours.** Open `theme.css`. Every line is a setting with a note saying what it does:

```css
--bg: #0f1115;          /* page background */
--accent: #7955ff;      /* buttons, highlights */
```

A colour like `#7955ff` is a code. Search the web for "colour picker", choose a colour and copy its code
in. Change as few or as many as you like. Anything you delete goes back to the normal look.

One thing to watch: `--accent-rgb` is the same colour written as three numbers, and `--bg-rgb` is the
same for the background. Keep them in step. A colour picker shows these too, as `RGB`.

**4. Pack it up.** The two files must be at the top of the ZIP, not inside a folder.

- **Windows**: select both files, right-click, and choose *Compress to ZIP file*.
- **macOS**: select both files, right-click, and choose *Compress*.
- **Linux**: `cd my-theme && zip -r ../my-theme.zip .`

**5. Install it.** In **Settings, Appearance**, press **Install a theme ZIP…** (administrators). Or copy
the ZIP into the server's `themes` folder and reload the page:

| Install | Themes folder |
|---|---|
| Windows (tray app / service) | `%LOCALAPPDATA%\WittyMediaServer\themes` |
| macOS | `~/Library/Application Support/WittyMediaServer/themes` |
| Linux / FreeBSD | `~/.local/share/wittymediaserver/themes` (service: `/var/lib/wittymediaserver/themes`) |
| Docker / Kubernetes | `/config/themes` |
| Custom `-data` / `WMS_DATA_DIR` | `<data>/themes` |

To update it, install a ZIP with the same `id` again. To remove it, press **Remove** on its card.

### A complete small theme

This is all a theme needs. Everything not mentioned keeps its normal value, and hover and tinted shades
are worked out for you.

```css
:root {
  color-scheme: dark;
  --bg: #14090b;     --bg-rgb: 20 9 11;
  --bg-2: #1f0f12;
  --bg-3: #2a1216;
  --bg-4: #3a1b20;
  --fg: #f6e7d0;
  --fg-2: #c9b89f;
  --fg-3: #a8967c;
  --accent: #d4a017; --accent-rgb: 212 160 23;
  --accent-2: #b98b12;
  --accent-fg: #17120a;
  --accent-text: #e0b542;
}
```

### Keeping it readable

Text needs to stand out from what's behind it. The usual measure is contrast, and the bar is 4.5:1 for
text and 3:1 for the outlines of fields. Search the web for "contrast checker" and test `--fg-3` and
`--accent-text` against `--bg-3`, and `--accent-fg` against `--accent`. For a dark theme, use light text;
for a yellow or pale accent, use a dark `--accent-fg`.

The built-in themes all pass, and `python scripts/theme-contrast.py` checks them.

### A light theme

Set `color-scheme: light` and `"scheme": "light"`, so scrollbars and date pickers match, and choose dark
text on light surfaces. Titles over artwork carry a text shadow, which looks better softened:

```css
.hero h1, .detail h1 { text-shadow: 0 1px 6px rgba(255, 255, 255, .6); }
```

## Going further

Most themes only need to change variables, since every colour, radius and shadow goes through them. The
built-in stylesheet loads first, then your `theme.css`, so anything you set wins. `<html>` has
`data-theme="<id>"` if you want to scope a rule.

The **video player is always dark**, so video looks right with light themes. You can still restyle it
under `.player`.

### Pictures and fonts

Put them in the ZIP and use relative addresses:

```
my-theme.zip
├── theme.json
├── theme.css
└── assets/
    ├── backdrop.webp
    └── fonts/Cinzel.woff2
```

```css
@font-face {
  font-family: "Cinzel";
  src: url("assets/fonts/Cinzel.woff2") format("woff2");
  font-display: swap;
}
:root { --font: "Cinzel", Georgia, serif; }
body { background: var(--bg) url("assets/backdrop.webp") center / cover fixed; }
```

Keep images small: WebP and AVIF give the smallest files. Remote images in `url()` load, but then every
viewer's browser contacts that site, so bundle what you can. Check that the licence of any font or image
lets you share it.

### Recipes

**Just change the accent:**

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

**Your own logo:**

```css
.brand img { content: url("assets/logo.svg"); width: 36px; height: 36px; }
```

**Just the logo, no name:**

```css
.brand span, .brand .brand-name { display: none; }
```

**A different accent in the player only:**

```css
.player { --accent: #ffcc00; --accent-rgb: 255 204 0; }
```

**Glassy panels over a wallpaper.** Pretty, but a blur repaints on every frame of scrolling, so expect it
to stutter on TVs and old phones:

```css
body { background: #000 url("assets/wallpaper.webp") center / cover fixed; }
.panel, .sidebar, .topbar, .modal { background: rgb(20 22 30 / .72); backdrop-filter: blur(14px); }
```

### Changing the name in the corner

That's not part of a theme: **Settings, Appearance, Name and logo** (administrators). The licence notice
in Settings, About always remains.

## Suggesting a look for the TV guide

A theme can suggest how the TV guide looks, with two settings in `:root`:

```css
:root {
  --tv-layout: classic;     /* classic (the cable box) or streaming (the modern look) */
  --tv-colours: satellite;  /* cable, satellite, midnight or theme */
}
```

It's only a suggestion: what a viewer picks under **Look** on the guide wins, and the guide tells them
what the theme suggests. Leave these out and the guide starts with what the server's owner chose.
Midnight does this.

## Reference

### theme.json

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | shown in the theme picker |
| `id` | no | letters, digits, `.`, `-`, `_`; at most 64. Defaults to the ZIP's name. Installing with the same id replaces it |
| `scheme` | no | `dark` (default) or `light` |
| `description`, `author`, `version` | no | shown in the picker, or for your records |
| `colors`, `radius` | no | the colours as data, which draws the picker's preview exactly. `scripts/design-tokens.py` writes them for the built-ins |
| `preview` | no | four colours for the preview if `colors` isn't there: `bg`, `surface`, `text`, `accent` |
| `builtin` | | reserved: a ZIP with this is refused |

### Variables

Set these in `:root`. Hover, selected and tinted shades are worked out from them. The sizes, spacing and
motion the interface is built from are tokens you can change too, such as `--card-w` or `--text-base`: see
[DESIGN.md](DESIGN.md).

| Variable | Default | Used for |
|---|---|---|
| `--bg` | `#0f1115` | page background, body |
| `--bg-rgb` | `15 17 21` | `--bg` as space-separated R G B, for translucent shades: `rgb(var(--bg-rgb) / .6)` — the hero and detail-page fades over artwork. **Keep it in sync with `--bg`** |
| `--bg-2` | `#171a21` | side menu, panels, dialogs |
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

`color-scheme` (`dark` or `light`) in `:root` tells the browser how to draw its own controls.

### Classes

The interface is built from these classes. Combine them with the variables above.

#### Layout

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

#### Home, libraries and cards

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

#### Detail pages

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

#### Controls and forms

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

#### Player

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

#### Breakpoints

The built-in stylesheet switches to the mobile layout at `max-width: 900px` (sidebar becomes a
drawer) and tightens the player and episode list at `max-width: 700px`. Use the same widths in
your `@media` rules.

## Rules and limits

- `theme.json` and `theme.css` at the top of the ZIP are required. ZIPs up to 25 MB, files up to 10 MB, up
  to 200 files.
- File types: `.css .json .png .jpg .jpeg .webp .gif .avif .svg .woff2 .woff .ttf .otf .txt .md`. Anything
  else, such as scripts or HTML, makes the install fail. Themes can't contain scripts.
- File names are plain relative paths: no `..`, no absolute paths, no backslashes.
- Only administrators can install or remove themes. CSS can load images from other sites, which tells
  those sites when someone opens the app, so install only themes you trust.
- Built-in themes are rewritten at every start-up, so don't edit one in place: give your copy a new `id`.
- A built-in theme can't be removed. Viewers using a theme that has been removed go back to the server's
  theme after a reload.
- Theme files are public, because the sign-in page is themed too: `GET /api/themes`,
  `GET /api/themes/<id>/theme.css`, `GET /api/themes/<id>/<file>`, `GET /api/themes/<id>/download`.
  Making a theme for the server is `POST /api/themes/custom` (administrators), which accepts only plain
  `#rrggbb` colours.

---
<sub>WittyMediaServer 0.6.0-alpha documentation</sub>
