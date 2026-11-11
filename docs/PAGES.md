# Pages and the menu

Home, each library, a title's page, Search and the main menu are described by
the server, and every app draws them from that description. Two things follow.
Anyone can arrange a page for themselves (**Customise this page**), and an
administrator can change what the pages and the menu hold for everyone
(**Settings → Pages & menu**). Settings itself is not one of these pages: it
stays as it is.

This guide is about the web app, which is also what the desktop app shows.
The full model, for people writing apps, is in [SDUI.md](SDUI.md).

## Customise a page for yourself

At the foot of a page that can be arranged is **Customise this page** (on a
TV remote or a controller: the options button, Start).

- **Order.** Drag a section by its handle, press the up and down arrow keys
  on the handle, or use the Move buttons.
- **Show or hide.** The switch on each row. A hidden section stays in the
  list, so it can be shown again. One marked "nothing in it now" is on the
  page as soon as it has something to show.
- **Pin.** A pinned section sits above the rest.
- **Card size.** Small, medium or large cards, for a row or grid.
- **Spacing.** Compact, comfortable or spacious, for the whole page.
- **Open on.** For a page with tabs, the tab it opens on.

Each change is saved at once and belongs to your profile, so it follows you
to every device and app. **Undo** takes the last change back; **Reset** puts
the page back as everyone has it. A section marked **Always shown** can't be
hidden or moved: whoever runs the server locked it.

The main menu is arranged the same way in **Settings → Appearance → Arrange
the menu**. Settings can be moved but is always shown.

**Not offered it?** Whoever manages your profile can turn rearranging off
(**Profiles → Limits → What they can do → Rearranging pages**), an
administrator can turn it off for a page, or for the whole server. What you
had arranged is kept, and comes back if it is turned on again.

## Change pages for everyone

**Settings → Pages & menu**, for administrators. Nothing you do here reaches
anyone until you publish it.

1. **Pages** lists every page: the built-in ones, the ones you changed and
   the ones you made. Open one to edit it, or choose **New page**.
2. In a page, the list at the left is its sections, top to bottom. **Add**
   offers every kind of section with a line about what it is; **Ready-made**
   offers finished ones (Continue watching, a Top 10, a row for one genre).
   Drag sections to reorder them, or use the arrow keys or the Move buttons.
3. Select a section to set it up. Every field says what it does. **Shows**
   is where its items come from (recently added in a library, a genre, a
   collection); **Only when** limits it to some profiles, devices or times
   of day.
4. Problems are listed as you go, each beside the field it is about, with
   what to do; where the fix is mechanical, **Apply fix** does it.
5. **Preview** shows the page as a phone, a tablet, a desktop window and a
   television draw it. **Open the draft in the app** shows you, and only
   you, the whole app with your draft in place.
6. **Publish** makes the draft what everyone gets. **Discard draft** throws
   it away.

Also there:

- **The menu**: which entries the main menu has, in what order, and where
  each sits on a phone, a tablet, a desktop and a television. Settings can
  be moved, never removed.
- **A page for some people only**: a variant of a page with a condition (for
  Kids profiles, for televisions) replaces the page's sections when the
  condition holds.
- **Reset to default** on a built-in page you changed brings the built-in
  one back, including whatever a later version of the server improves in it.
- **History** keeps the last ten published versions, says what changed
  between two, and can go back to one.
- **Export** saves a page, or everything, as a file; **Import** reads one,
  showing what would change before anything does.
- **JSON**, for people who want it: the page as text, checked before it is
  applied.

If someone else publishes while you are editing, you are told when you save,
and can read their version and make your change again.

## What the pages are made of

A page is a list of sections: a spotlight, rows and grids of cards, a
title's header, a show's seasons, an album's tracks, filters, tabs, banners,
notices and text, plus a few layout pieces for pages of your own. Sections
never hold colours or sizes: those come from the theme and from the device,
which is why a page looks right on a phone and on a television without
being made twice.

Some pages are the app's own from top to bottom and are only linked to:
Settings, the players, the book reader, a game's page, the TV guide's grid,
the photo timeline and map.

---

<sub>WittyMediaServer documentation</sub>
