# Reading comics

## The screen

- **Top bar:**
  - back and the title;
  - the layout choice (**Page**, **Spread**, **Scroll**);
  - the reading direction;
  - **Fit width** or **Fit height**.
- **Footer** (in Page and Spread): a page slider and *page / total*.

The bars hide after a few seconds. Tap the middle of the screen, or move the mouse, to show them.

## Layouts

| Layout | What you see | Good for |
|---|---|---|
| **Page** | One page, as large as fits. | Phones, tablets held upright. |
| **Spread** | Two pages side by side. The cover is shown alone, so the following pairs match the printed book. | Wide screens, two-page art. |
| **Scroll** | Every page in one vertical strip; scroll down to read. Pages load as you approach them. | Webtoons and vertical comics. |

## Reading direction

The direction button switches between **Left to right** and **Right to left** (manga). The
direction changes:

- which side of the screen goes forward;
- which arrow key goes forward;
- which way a swipe turns;
- which end of the page slider is the start.

In a right-to-left spread, the first page of each pair is on the right, as in a printed manga.

## Fit

- **Fit height** (the default): the whole page is on screen.
- **Fit width**: the page fills the width; scroll down to read the rest. Good for tall pages on
  a wide screen.

## Turning pages

| How | Forward | Back |
|---|---|---|
| **Tap or click** | the outer third in the reading direction (right for left-to-right) | the other outer third |
| **Swipe** | towards the start of the line (leftward for left-to-right) | the other way |
| **Keys** | the arrow in the reading direction, **Space**, **Page Down** | the other arrow, **Page Up** |
| **Jump** | **End** for the last page | **Home** for the first page |

In Spread, each turn moves two pages. **Esc** (or a remote's Back button) closes the comic. In
Scroll, just scroll.

## Remembered for each comic

WittyComics remembers the layout, direction and fit **for each comic** on the device. A manga
stays right to left while other comics stay left to right. The first time a comic opens, it
starts with the app's suggestion:

- **WittyReader** reads it from the comic's `ComicInfo.xml`;
- **WittyMediaServer** reads it from `ComicInfo.xml` (`Manga: Yes`) or from a folder whose name
  contains "Manga".

## Smooth reading

The next three pages are fetched while you read the current one, so turning is instant on slow
connections and low-end devices.
