# Translation

The web app's own words follow the language the server is set to: **Settings** or first-time setup's first screen,
"Language and time zone". It applies to every page, for everyone, including the sign-in page. Until a language is
chosen, each browser shows its own language when the app has it, and English when it doesn't.

There are 26 languages: English, Spanish, French, German, Italian, Portuguese, Dutch, Swedish, Danish, Norwegian,
Finnish, Polish, Czech, Hungarian, Greek, Turkish, Russian, Ukrainian, Japanese, Korean, Chinese, Hindi, Arabic,
Hebrew, Thai and Vietnamese, with Portuguese as written in Brazil (pt-BR) and in Portugal (pt-PT). Arabic and
Hebrew pages are laid out right to left. Disc menus and sound tracks use the plain two-letter languages.

Titles, names and descriptions of your media are never translated: they're shown as your files and the metadata
sources give them.

## For people working on the app

The layer is in `web/src/i18n`. The English text is the key, so anything not translated yet still shows, in English.

```ts
import { t, tn, tk, tx, num, date } from '../i18n'
t('Sign in')
t('Delete {name}?', { name })
tn('{count} film', '{count} films', count)   // the right plural form for each language
tk('Movies')                                 // marks text in a list made when the file loads; shown with tx(label)
tx(textFromTheServer)                        // translated when the catalogue knows it, otherwise as it is
tc('broadcaster', 'Network')                // English that means more than one thing: keyed "broadcaster|Network"
num(1234), date(when, { dateStyle: 'medium' })
```

- Write the English as a plain quoted string, with `{name}` fill-ins rather than template strings, so the check script
  finds it.
- Catalogues are in `web/src/i18n/locales/<language>/<part>.json` (`pt-BR` and `pt-PT` for Portuguese): English → translation. A message with a count
  gives the language's plural forms (`one`, `few`, `many`, `other` and so on). Only the chosen language's files are
  loaded.
- `node scripts/i18n-check.mjs` (from `web/`) counts what each language has; `node scripts/i18n-check.mjs de` lists
  what German is missing.
- Changing the language draws the whole app again, so components don't need to do anything to follow it.
- Text the server sends (error messages, labels of server-driven pages, status lines) is shown with `tx()`: translated
  when the catalogue knows it, otherwise as it is. Text the server filled in ("Devices away from home can connect
  through your router.") is matched back to its key with `{names}`, and a `{name}` that is itself one of the server's
  messages is translated too.

### The server's words

The server sends English, and marks each sentence a person reads with `internal/i18n`, so the web app can find it:

```go
writeErr(w, 400, i18n.T("That invite has expired."))
s.Home = i18n.F("Devices at home: {where}", "where", where)   // fill-ins by name, as in t()
it.Subtitle = i18n.N("{count} album", "{count} albums", n)    // a count: the app picks the language's form
return i18n.Error("Can't open {name}", "name", file)           // an error whose message reaches people
```

`writeErr` messages, and fields such as `Label`, `Desc`, `Summary`, `Message`, `Hint` and `Detail`, count as marked
without `i18n.T`; so does a sentence kept in a `const`.

`go run ./scripts/i18n-server` (from the repository root) finds them all. CI runs it, and it fails when:

- a sentence of English is written somewhere it can't follow (passed to a function, kept in a variable) without
  `i18n.T`. Wrap it, or, when no person reads it (a log line, a file format), add it to
  `scripts/i18n-server/allow.txt` with the reason;
- `web/src/i18n/server-keys.json` (every key) or `server-plurals.json` (each count's English for one) is out of date:
  `go run ./scripts/i18n-server -write`.

It also lists how many keys each language has no translation for yet: they show in English until the translation
pass. `go run ./scripts/i18n-server -missing de` lists German's, with where each is said; `-strict` fails on any,
for that pass. The translations go in `server.json` beside the language's other catalogues.

Shown by the operating system rather than the web app (the tray menu, the "already running" dialog, the uninstaller,
the command line), the server's words stay English.

### Right to left

In Arabic and Hebrew the page runs right to left, and text that isn't the app's own runs in its own direction inside
it: a title, file name, artist or "2 hr 10 min" keeps its digits and dashes where they are.

- `web/src/bidi.css` lists the classes that hold such text (titles, names, file names, a title's facts); each takes
  its direction from its first letter (`unicode-bidi: plaintext`) and stays lined up with the page. A new component
  that shows titles or names adds its class there, or puts `dir="auto"` on the element, or `<bdi>` around the text.
- What `t()` fills into a sentence is isolated (U+2068 … U+2069) in right-to-left languages, so `t('Open {name}')`
  needs nothing more.
- A line put together from parts: `parts.map(bidi).join(' · ')` (`bidi` from `../i18n`).
- `npm run test:rtl` (from `web/`, with `WMS_BIN` set to a built server) starts a server of its own with a small
  library of Latin, Hebrew and mixed names, walks the pages in Arabic and Hebrew, saves a screenshot of each, and
  fails when any text that starts left to right is shown reordered or cut at its start. CI runs it and keeps the
  screenshots.

The apps do the same for lines made of parts: `factsLine()` in the Android app (`ui/Bidi.kt`) and in the Apple apps
(`WittyKit/Bidi.swift`).

## Modules

A module's web part gets the language from its host (web host API 5, `host.has('i18n')`):

```ts
host.i18n.language()        // 'de', or 'pt-BR' / 'pt-PT' (a BCP 47 tag)
host.i18n.useLanguage()     // the same, as a React hook
host.i18n.onLanguage(f)     // told when it changes
host.i18n.rtl()             // true for Arabic and Hebrew
host.i18n.number(n), host.i18n.date(d, opts), host.i18n.relative(v, unit), host.i18n.list(items)
```

Each module brings its own words; the app's catalogues are the app's.

---

<sub>WittyMediaServer documentation</sub>
