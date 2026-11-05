# The website: wittymediaserver.com

> **Status on 2026-11-04**
>
> **Built, not published.** The site isn't in a repository on GitHub yet: `wittymediaserver.com` is in the list of repositories to make, in [Repositories](../REPOSITORIES.md#to-make).

A plan to move the project's website off the hosted AI site builder and into a project of its own: plain files you upload to your Hostinger plan, that look like WittyMediaServer itself, read the same on every page, and pick up the latest releases, modules and pictures from GitHub with one command.

Written 2026-11-03. Guesses are marked **to verify**.

> **Status, 2026-11-04: built.** The site is in `D:\projects\wittymediaserver.com` (local commits, not on GitHub yet). Every page here exists, plus a page per module and the theme switch. `UPDATING.md` there has the routine. Still to do: the owner creates the GitHub repository and switches the domain from the Website Builder to the hosting plan (phase 6).

---

## What we want

**The site is WittyMediaServer's home, and it covers every project under it:** the server, WittyMediaClient, WittyMediaPlayer, WittyReader and the modules, each with its place, in the one WittyMediaServer name and look.

1. **Ours to keep.** The site's source lives in its own repository, not inside a hosted builder. Anyone can rebuild it from that.
2. **Simple to host.** The finished site is a folder of plain HTML, CSS, JavaScript and pictures. No database, no PHP, no server program. You upload it to Hostinger by dragging the folder (or a ZIP) into the File Manager.
3. **Looks like the product.** The same colours, type, spacing, cards, buttons and side menu as WittyMediaServer's own pages, taken from the server's design tokens, not copied by eye.
4. **Says things one way.** One source for the phrases every page repeats (what the project is, the platforms, the licence, the alpha warning), and a check that catches drift.
5. **Easy to keep current.** Releases, module descriptions and icons, and screenshots come from GitHub when the site is built. Your own news articles are short text files.
6. **Light and private.** Small pages, no trackers, no cookies, so no cookie banner. Fast on a phone.

## What the current site has

Read from https://wittymediaserver.com on 2026-11-03.

| Page | Address | What's on it |
|---|---|---|
| Home | `/` | Tagline, the server and the apps, latest releases, two news items, three screenshots |
| The server | `/server` | What it does, platforms and architectures, two screenshots |
| The apps | `/client` | WittyMediaClient: platforms, how far each has been run, three screenshots |
| Modules | `/modules` | The module catalogue, loaded from GitHub in the page, with filters |
| News | `/news` | GitHub releases, and handwritten articles such as `/news/firefox-disc-playback` |
| Release notes | `/news/release/<Repository>/<tag>` | One release's notes |
| Gallery | `/gallery` | Screenshots from the test server, with credits |

Every one of those addresses keeps working on the new site (see [Keeping old links working](#keeping-old-links-working)).

### Every project, not only two

The current site has pages for the server and WittyMediaClient only. The new one adds:

- **A projects page** listing every project with a one-line description, its state (released, early, not started) and links to its page and repository. It's built from one list, `content/projects.json`, kept in step with `docs/REPOSITORIES.md`, so a new project means one new entry.
- **A page each for WittyMediaPlayer and WittyReader**, in the same shape as the apps page, saying plainly how far each has got. Their releases appear in news once they have any.
- **Modules stay on the modules page**, from the catalog, each linking to its repository. Supporting libraries such as WittyModule and WittyDisc are listed on the projects page under "Behind the scenes" rather than given pages.
- **The menu** groups them: the server, the apps, modules, news, projects.

### Things to fix while we're at it

Found on a first read. The content pass in phase 4 looks for more.

- **Release previews show raw Markdown.** 0.6.2-alpha and 0.6.0-alpha show "## Which file to download" instead of a sentence about the release.
- **News says "Release notes from the project changelog"**, but the list comes from GitHub releases, and CHANGELOG.md has nothing after 0.6.0-alpha.
- **"Plasma Bigscreen" and "Plasma Big Screen"** both appear on the apps page. KDE writes it "Plasma Bigscreen".
- **"catalog" on the site, "catalogue" in the docs.** Pick one. (The catalogue's own repository is WittyModuleCatalog, so "catalog" may be the better fit **to verify** with the docs.)
- **The module page is empty until the catalogue loads** ("Loading the catalog from GitHub…"). Without JavaScript, or when GitHub is slow, a visitor sees nothing.
- **The server page and the apps page each describe the other** in slightly different words. One shared sentence for each will do.
- **No download page.** Download buttons lead to a GitHub release whose first heading is "Which file to download". A page that names the right file for each computer would save a step.

---

## How it works

```
GitHub (releases, module catalogue, screenshots)   content/ (your articles and page text)
                     \                                  /
                      \                                /
                       ->   node build.mjs   <--------
                                  |
                                  v
                          site/   (plain files)
                                  |
                       drag into Hostinger File Manager
```

### The repository

A new repository, **wittymediaserver.com**, at `D:\projects\wittymediaserver.com`, with this layout:

| Folder | What's in it |
|---|---|
| `content/pages/` | The text of each page, in Markdown, one file per page |
| `content/news/` | Your articles: `2026-11-01-firefox-disc-playback.md`, with a title, date and summary at the top |
| `content/site.json` | Phrases every page shares, the menu, the platform table, links |
| `templates/` | The page frame (top bar, menu, footer) and the parts pages reuse: card, release, module, screenshot |
| `style/` | The site's CSS, built on the server's theme |
| `build.mjs` | Fetches from GitHub, fills the templates, writes `site/` |
| `check.mjs` | Checks words, links and pictures (see [Saying things one way](#saying-things-one-way)) |
| `cache/` | What was last fetched from GitHub, so a build works offline. Not committed |
| `site/` | The finished site, the folder you upload. Not committed |

It needs only Node, which is already on your machine. No framework and no packages to install, apart from perhaps one small Markdown library (**to verify**: write our own small one, or use `marked`, which is MIT).

Licence: Apache-2.0, like the server. Screenshots keep their own credits from `docs/images/CREDITS.md`. The README ends with the usual **Legal** section.

### What comes from GitHub

| What | Where from | How it shows |
|---|---|---|
| Releases | The GitHub releases of WittyMediaServer, WittyMediaClient, and later WittyMediaPlayer and WittyReader | News, the home page, and a page per release. The preview is the release's first real sentence, never a heading |
| Download page | The latest release's files | The right file for Windows, macOS, Linux, FreeBSD and Docker, with sizes |
| Modules | `catalog.json` and `icons/` in WittyModuleCatalog | The modules page, built into the page so it shows without waiting. Name, icon, summary, description, group, what it needs, link to its repository |
| Screenshots | `docs/images/*.webp` and `CREDITS.md` in WittyMediaServer | Home, server, apps and gallery pages, each with its credit |
| Theme | `themes/default/theme.css`, `themes/light/theme.css` and `themes/tokens.json` in WittyMediaServer | The site's colours, type, spacing and shapes |

A build reads releases through GitHub's public API, or through `gh` when it's signed in, which allows more requests. Everything fetched is saved to `cache/`, so a second build is quick and a build with no internet still works.

The modules and news pages can also check GitHub when someone opens them, and add anything newer than the build, as the current site does. That's a small script, and the page is complete without it. **Decision for you:** keep that live check, or rely on rebuilding (see [Questions for you](#questions-for-you)).

### Looking like the product

**The current front page is the starting point.** You like its layout, so the rebuild keeps it (the tagline and screenshot at the top, the server and the apps side by side, what's new, latest releases, a look around) and improves on it, rather than starting a new design. The other pages follow its lead.

**The first priority is rebuilding the site** as our own files, with the same pages and look. Flashier touches, such as the theme switch below, come after that's live.

- The site uses the same CSS variable names as the server (`--bg`, `--bg-2`, `--fg`, `--accent`, `--accent-text` and so on), so the server's theme files drop straight in.
- Dark with the house "Witty" theme (deep slate, violet accent), as now. The CSS is written so themes can be added later without rework.
- The layout borrows the server's pieces: the top bar, poster and wide cards, pills for groups, the module card from **Settings → Modules → Browse the store**, and the same focus ring, so it works with a keyboard.
- Type, spacing, corner rounding, shadows and motion come from `tokens.json`. When the server's design changes, a rebuild picks it up.
- Follows `docs/DESIGN.md`: sentence case, plain words, "you", no exclamation marks, curly quotes, 4.5:1 contrast.

### Saying things one way

- **One source for shared text.** `content/site.json` holds the one-line description of the server, of the apps and of modules, the platform list, the licence line and the alpha warning. Pages include them instead of retyping them.
- **A word list** in `check.mjs`: the spellings we use (Blu-ray, WittyMediaClient, Plasma Bigscreen, catalog or catalogue), words to avoid, and names that must never appear, including other media servers'. A build stops if it finds one.
- **The same rules as the product's own writing** in `docs/DESIGN.md#writing`.
- **Links and pictures checked:** every link inside the site goes somewhere, every picture exists and has a description and a credit.
- **A content pass** in phase 4: every page read side by side, rewritten to one voice, and checked against what the docs and the latest release actually say (nothing promised that isn't built; tested and untested said plainly, as the docs do).

### Updating the site

What you do, once it's built:

1. Open a terminal in the site's folder, `D:\projects\wittymediaserver.com`.
2. Run `node build.mjs`. It fetches the latest releases, modules and pictures, and writes `site/`.
3. Look at it: `node build.mjs --preview` opens it in your browser.
4. In Hostinger's File Manager, open `public_html`, and drag the contents of `site/` in, replacing what's there. (Or upload `site.zip` and choose **Extract**.)

To write a news article, copy any file in `content/news/`, change the title, date and text, and do the steps above. `UPDATING.md` in the repository will have these steps with pictures.

Later, and only if you want it, nothing needs doing by hand. Two ways, both fine on the Business plan:

- **Hostinger's Git deployment** (hPanel → Advanced → Git, **to verify**). A GitHub Action builds the site when a release is published and puts the result on a `live` branch. Hostinger pulls that branch into `public_html`, by itself through a webhook. No passwords leave Hostinger, so this is the one to prefer.
- **FTP from the Action.** The Action uploads `site/` straight to Hostinger. It needs your FTP details stored as a secret in the repository, which you'd add yourself.

### Keeping old links working

The build writes each page as a folder with an `index.html` (`news/firefox-disc-playback/index.html`), so `/news/firefox-disc-playback` keeps working on any host, with no server rules. Release pages keep their `/news/release/<Repository>/<tag>` addresses.

### Light and private

- No trackers, analytics, cookies or downloaded fonts. The font list is the server's: Inter where it's installed, the system's font otherwise.
- Pictures are WebP at the sizes they're shown, with lazy loading below the fold.
- A page should be under 100 KB before pictures. The modules page, with every module built in, under 200 KB (**to verify**).
- A strict Content Security Policy in a `<meta>` tag, since a page that only shows text needs nothing from elsewhere. The live GitHub check, if kept, is the one exception, allowed to `api.github.com` only.

---

## Phases

| # | Phase | Who | Done when |
|---|---|---|---|
| 0 | **Decisions and access.** Answers to [the questions](#questions-for-you), and the repository made on GitHub | You | We know the plan type and the name |
| 1 | **Save the current site.** Every page's text and picture saved into `reference/`, so nothing is lost when the builder goes | Claude | Every page above is saved |
| 2 | **The skeleton.** Repository, `build.mjs`, the page frame and the theme from the server, one page working | Claude | The home page builds and looks like the product, dark and light |
| 3 | **Every page.** Home, server, apps, modules, news, release pages, gallery, and the new download, projects, WittyMediaPlayer and WittyReader pages | Claude | Every old address has a page, and the build fetches from GitHub |
| 4 | **The content pass.** Shared text, the word list, a rewrite to one voice, every fact checked against the docs | Claude, then you read it | `check.mjs` passes, and you're happy with the words |
| 5 | **Trying it.** Phone, tablet, TV browser, keyboard only, slow connection, no JavaScript | Claude, and you on your own devices | Works on each, with screenshots |
| 6 | **Going live.** Upload to Hostinger, point the domain at it, check every old link | You, with step-by-step help | wittymediaserver.com serves the new site |
| 7 | **Later: the theme switch.** See [After launch: changing the theme](#after-launch-changing-the-theme) | Claude, and you choose which themes | Picking a theme changes the page and its screenshots |
| 8 | **Later.** The automatic build and upload, translated pages, a short "what's new" that matches the server's update notes | When you want | |

Phases 1 and 2 can start as soon as the repository exists. The old site stays up until phase 6. Phases 1 to 6 are the rebuild and come first; 7 and 8 are extras once it's live.

### After launch: changing the theme

A switch on the site that changes it to any of the server's themes (Witty, Light, Cinema, Evergreen, Midnight, OLED, Paper, Contrast), remembered in the visitor's browser, with the screenshots changing to match: pick Paper, and the pictures show the server in Paper.

- **The colours are easy.** Each theme is only a list of colours in the same variable names the site uses, so the switch swaps one file.
- **The pictures are the real work.** Each screenshot is taken once per theme from the test server with the demo media (`D:\WittyDemoMedia`), into `docs/images/<theme>/` in WittyMediaServer, so the docs could use them too (**to verify** that the docs want that). Eight themes times about 15 pictures the site shows is about 120 pictures, so it's scripted: the same script that retakes the docs pictures, run once per theme.
- **Only the pictures on screen load.** A picture has one address per theme, and the browser fetches the theme's version when it's shown. A theme without a picture falls back to the Witty one, so themes can be added a few at a time.
- **No flash on load:** a few lines at the top of each page apply the saved theme before anything draws.
- It could start with dark and light only, with light following the visitor's device, and add the rest later.

Per the project's GitHub practice, the work starts as a discussion in the new repository's **Ideas**, and each phase as an issue with a pull request that closes it.

---

## Questions for you

1. ~~**Which Hostinger plan is it?**~~ **Answered 2026-11-03: the Business web hosting plan.** It has a File Manager, `public_html` and FTP, so the drag-and-drop upload in this plan works as written. The current site is the plan's included Website Builder, so going live (phase 6) means telling hPanel to serve the domain from the uploaded files instead of the builder (**to verify** where that switch is in hPanel when we get there).
2. ~~**The repository's name.**~~ **Answered 2026-11-03: `wittymediaserver.com`**, a repository of its own, after the address. The site itself is called WittyMediaServer.
3. **Live GitHub check:** keep it on the news and modules pages, or show only what the last build fetched? Keeping it means the site is never out of date. Dropping it makes the pages simpler and fully private.
4. **"catalog" or "catalogue"?** The site and the code say catalog, some docs say catalogue.
5. **New pages:** a download page (recommended), a short FAQ, a page for WittyMediaPlayer and WittyReader once they have releases?
6. **Anything you dislike about the current look or text** beyond what's listed above? And what you'd most like improved on the front page? Screenshots or a few words are enough.

---

<sub>Future plans, written 2026-11-03.</sub>
