# Translation

The web app's own words follow the language the server is set to: **Settings** or first-time setup's first screen,
"Language and time zone". It applies to every page, for everyone, including the sign-in page. Until a language is
chosen, each browser shows its own language when the app has it, and English when it doesn't.

There are 26 languages: English, Spanish, French, German, Italian, Portuguese, Dutch, Swedish, Danish, Norwegian,
Finnish, Polish, Czech, Hungarian, Greek, Turkish, Russian, Ukrainian, Japanese, Korean, Chinese, Hindi, Arabic,
Hebrew, Thai and Vietnamese. Arabic and Hebrew pages are laid out right to left.

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
- Catalogues are in `web/src/i18n/locales/<language>/<part>.json`: English → translation. A message with a count
  gives the language's plural forms (`one`, `few`, `many`, `other` and so on). Only the chosen language's files are
  loaded.
- `node scripts/i18n-check.mjs` (from `web/`) counts what each language has; `node scripts/i18n-check.mjs de` lists
  what German is missing.
- Changing the language draws the whole app again, so components don't need to do anything to follow it.
- Text the server sends (error messages, labels of server-driven pages) is translated in the app with `tx()` when it's
  fixed text the catalogue knows. Text the server builds with names or numbers in it shows in English for now.

## Modules

A module's web part gets the language from its host (web host API 5, `host.has('i18n')`):

```ts
host.i18n.language()        // 'de'
host.i18n.useLanguage()     // the same, as a React hook
host.i18n.onLanguage(f)     // told when it changes
host.i18n.rtl()             // true for Arabic and Hebrew
host.i18n.number(n), host.i18n.date(d, opts), host.i18n.relative(v, unit), host.i18n.list(items)
```

Each module brings its own words; the app's catalogues are the app's.

---

<sub>WittyMediaServer documentation</sub>
