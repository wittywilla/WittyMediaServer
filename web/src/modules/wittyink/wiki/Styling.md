# Styling

WittyInk's own styles (`reader.css`) come in automatically when you import the package. They
cover:

- the full-screen reader frame, the top bar and the footer;
- the side panels, the pop-ups and the ink toolbar.

The readers also use a few general classes and colour variables that an app normally already
has. If yours doesn't, import the small kit:

```ts
import '@wittywilla/wittyink/kit.css'
```

WittyMediaServer doesn't import it: its own design system and themes define the same names.

## Colour variables

| Variable | Used for |
|---|---|
| `--bg`, `--bg-2`, `--bg-3`, `--bg-4` | Backgrounds, from the page to raised panels and controls. |
| `--fg`, `--fg-2`, `--fg-3` | Text, from main to muted. |
| `--accent`, `--accent-2`, `--accent-fg` | Buttons, selected items, sliders, focus rings. |
| `--danger` | Remove buttons. |
| `--border`, `--shadow` | Hairlines and panel shadows. |
| `--font` | The interface font (book text has its own). |

## Classes

| Class | What it is |
|---|---|
| `.btn` (with `.primary`, `.ghost`, `.danger`, `.sm`) | Buttons. |
| `.ibtn` (with `.sm`) | Round icon buttons. |
| `.input`, `.select` | Text boxes and drop-downs. |
| `.seg` with `button.active` | Segmented choices (layout, zoom, page colour). |
| `.check-row` | A checkbox with its label. |
| `.muted`, `.small`, `.grow`, `.error`, `.spinner` | Muted text, small text, a growing flex item, an error box and a loading spinner. |

Book pages have their own colours (light, sepia, dark, black), set by the reader. They're not
affected by the app's theme.
