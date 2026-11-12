# The family's marks

Every logo, icon and background of WittyMediaServer, WittyMediaClient, WittyMediaPlayer, WittyReader, the modules and
the website is drawn by one program, `scripts/genicon`, from the drawings in `internal/brandart`. Nothing is drawn by
hand and no picture is edited after it is made: change a drawing, then run `scripts/sync-brand.sh` in each repository.

## The mark: the W in its orbit

Each app is a thick ring with a gap at the top right, a dot (its "moon") sitting in the gap, and what the app is
inside the ring. The marks sit on the website's dark slate tile and are drawn in a gradient of the app's own colours,
the way the website's own icon always was.

| App | Inside the ring | Ring | Dot |
|---|---|---|---|
| WittyMediaServer | the W | lilac into violet | amber |
| WittyMediaClient | a screen on a stand, with a small ring and W on it | violet into sky blue | mint |
| WittyMediaPlayer | a play button | coral into violet | sky blue |
| WittyReader | an open book | mint into sky blue | coral |

The server and the client are told apart by their outline (a round mark against a screen on a stand) and by their
dot. Every app's dot is its own colour, so the four differ even where they are small. Module icons use the same tile
with a glow and a mark in their catalogue category's colour, and the server's amber dot in the corner, since modules
are part of the server.

Every colour comes from the design tokens: the house theme's background, surfaces, accent, "ok" (mint), "warn"
(amber) and "danger" (coral), and the Midnight theme's accent (sky blue), all read from `themes/*/theme.json` (written
by `scripts/design-tokens.py`). Change the theme and the icons follow at the next sync. Words on banners, Top Shelf
and the social picture use the website's colouring ("Witty", "Media" in the accent, "Server"), set in Go Bold until
the website's font, Inter, is added to the generator.

**Small sizes.** At 16 to 32 pixels the mark fills more of its tile, its lines get heavier, and the client's screen
drops the W inside its small ring. **One colour.** The ring, the W and the dot in a single colour on nothing, with the
cuts as holes: the macOS menu-bar icon (a template image), Android's themed icon, iOS's tinted icon. Grey while the
server is stopped. **Safe area.** The mark's farthest point is 0.325 of the icon from its centre, inside the circle
every Android launcher mask shows and clear of iOS's and tvOS's corners.

## Other designs, one flag away

The other designs are kept, complete, in `internal/brandart` (`marks.go`, `concepts2.go`). Any of them draws every
file below:

```bash
WITTY_BRAND_CONCEPT=monogram scripts/sync-brand.sh
```

| Concept | What it is |
|---|---|
| `orbitw` | in use: the W in its orbit |
| `monogram` | a heavy rounded W on a rack unit; the client's W inside a screen |
| `orbit` | a core with an orbit and its moon. Close to the Ionic framework's logo: don't use it on its own |
| `nodes` | the W as five dots joined by short links, the last one lit |
| `wave` | the W as one smooth wave on a rack unit |
| `dial` | a knob with a gauge and a lit dot |
| `pills` | five rounded bars whose tops trace a W, like a sound meter |

To make one the default, change `DefaultConcept` in `internal/brandart/marks.go` and run every repository's sync.
`go run ./scripts/genicon -sheet sheet.png` draws all of them side by side at real pixel sizes on dark, light and TV
backgrounds.

## What is written where

`go run ./scripts/genicon -target <name> -o <repository>` writes one repository's set (`-check` changes nothing and
fails if anything is out of date). Each repository's `scripts/sync-brand.sh` runs it with its own target, finding this
repository beside it (or at `WITTY_SERVER`).

| Target | Files |
|---|---|
| `server` | `web/public`: `icon.svg`, `favicon.ico` (16, 32, 48), `icon-192.png`, `icon-512.png`, `icon-maskable-512.png`, `apple-touch-icon.png` (180, opaque) |
| `server-package` | written by `scripts/build-release.sh` and `packaging/macos/build-pkg.sh` at build time: `icon.ico` (16–256), `icon.icns` (every macOS size, on Apple's 824 grid), `wittymediaserver.png` and `.svg`, the `hicolor` tree (16–512 and scalable), Windows Installer's `banner.bmp` (493×58) and `welcome.bmp` (493×312), the macOS installer's background |
| `client` | Apple: `apple/Brand/iOS.xcassets` (the 1024 icon with dark and tinted versions; the Apple Vision Pro icon as three depth layers; the launch screen's mark), `apple/Brand/tvOS.xcassets` (the layered Apple TV icon at 400×240 @1x/@2x and 1280×768 for the App Store, each in back, middle and front layers so it moves in depth when focused; Top Shelf 1920×720 and wide 2320×720 at @1x/@2x; the backdrop behind the TV's sign-in screens), `apple/App/BrandArt.swift`. Android: launcher icons for every density, round icons, the adaptive icon (background, foreground, monochrome), the TV banner (320×180 dp), the splash icon and launch background, the TV sign-in backdrop, `ui/BrandArt.kt`, and the Play icon and feature graphic in `android/brand`. Desktop: `desktop/build` icons for macOS, Windows and Linux, the disk image's background (@1x, @2x) and the Windows installer's pictures |
| `player` | `resources/WittyMediaPlayer.ico`, `resources/icon.svg`, `resources/icon-256.png`, the Windows installer's pictures |
| `reader` | the web app's icons in `public/`, and `desktop/build` as for the client |
| `catalog` | `icons/<module>.png` (128) for every module in `catalog.json` |
| `website` | `images/brand`: the web icons, `social.png` (1200×630, the link preview), `hero-dark.svg` and `hero-light.svg`, and each app's mark as SVG |

The apps draw their marks inside their own screens from generated code (`BrandArt.swift`, `BrandArt.kt`), so they stay
sharp at every size; the web client and the website show `icon.svg`. The tray draws the server's mark at run time
(`internal/trayicon`).

## How it is drawn

`internal/brandart` describes every shape as outlines of lines and cubic curves (`geom.go`): circles, rounded
rectangles, thick arcs with round ends, the W as one outline. Cut-outs run the other way round from the shape around
them, so one non-zero fill rule gives the holes everywhere. The same outlines are written as SVG, as Android vector
drawables and as Kotlin and Swift drawing code, and rasterised with anti-aliasing (`golang.org/x/image/vector`) to PNG,
then packed into ICO, ICNS, BMP or JPEG (`formats.go`). Words are drawn as outlines from the Go fonts, so no picture
depends on the fonts a machine has. Module marks are distance functions on a 128 grid (`modules.go`), each an original
drawing: never a console's, disc format's, service's or studio's logo.

`internal/brandart`'s tests check that every target builds in every design, that each picture is the size its
platform asks for, that store and home-screen icons are opaque and parallax layers are not, that every mark stays
inside the launcher's safe circle, that the server and client marks differ, and that the web client's icons in the
repository are what the generator draws now.
