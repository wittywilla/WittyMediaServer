# Design 2: web features with no matching element on the canvas

Written 2026-11-12 by W2 (web and desktop). Method: the pages and parts under `web/src` (`pages/*`, `components/*`, `ui/*`, `audio/*`, `player/*`, `remote/*`, `canvas/*`, `extras/*`, settings pages) were compared with the text of every board in the canvas (`project/*.dc.html`, local copy in the design scratchpad) by keyword search, then read by hand where a count was low. A keyword that never appears is listed here; a feature that appears only as a word (one board) is listed as "thin". This is a first pass: check each line against the canvas before dropping it.

## Pages and parts with nothing on the canvas

| Web feature (file) | State on the canvas | Needs |
|---|---|---|
| Podcasts library page and episodes (`pages/Podcasts.tsx`) | no board | list, episode rows, resume, in each look |
| Internet radio (`pages/RadioLibrary.tsx`, `/radio/:id`, `pages/MusicOnline.tsx`) | no board | station grid, now playing strip |
| Audiobooks (`pages/AudioLibrary.tsx`) | no board | chapter list, sleep timer (the canvas has no sleep timer anywhere) |
| Creator pages (`pages/creators`) | no board | person/creator header and credits (S12's cast and crew pages also need a look) |
| Fix matches (`pages/FixMatches.tsx`, `pages/fixmatches/*`) | no board | review list with candidate picker, keyboard help |
| Library review (`pages/ReviewPage.tsx`) | no board | |
| Activity panel and slot (`components/Activity*.tsx`) | no board | top-bar slot, panel, jobs list |
| Key help dialog (`ui/KeyHelp.tsx`) and the input legend | Dock and legend exist; the help dialog does not | draw it |
| Legal notes and acknowledgement bar (`components/LegalNote.tsx`, `LegalAckBar`) | no board | the bar and footer in every look |
| Screen time (`shell/ScreenTime.tsx`) | no board | the limit and "time is up" screens |
| Switch profile and sign-in (`shell/SwitchProfile.tsx`, `pages/AuthPage.tsx`) | thin: first-use board only | profile picker with PIN, sign-in, two-factor, in each look |
| Pair a device and device pairing (`pages/PairPage.tsx`, `components/PairDevice.tsx`) | thin | QR and code screens |
| Share links and the shared page (`components/ShareDialog.tsx`, `pages/SharedPage.tsx`) | thin | dialog, public page |
| Collections and collection detail (`pages/CollectionsPage.tsx`) | thin | |
| Save to collection, rating field, artwork picker, download button, upload button (`components/*`) | none or thin | dialogs and small controls in each finish |
| Add library, folder picker, library header (`components/AddLibrary.tsx`, `FolderPicker.tsx`, `LibraryHeader.tsx`) | no board | |
| Skip intro, auto skip, pick-intro bar, find intros (`player/AutoSkip.tsx`, `PickIntroBar.tsx`) | one mention | player overlays |
| Subtitle and audio pickers, view settings, play info panel, track picker (`player/ViewSettings.tsx`, `PlayInfoPanel.tsx`, `TrackPicker.tsx`) | player bar only | the pickers and panels |
| CD player and disc graphics (`player/CdPlayer.tsx`, `CdGraphics.tsx`) | thin | |
| Watch party (`remote/Party*.tsx`, `WatchParty.tsx`) and Play on (`remote/PlayOn*.tsx`) | one mention each | party panel, invites, device list |
| Games: library, details, review, rigs, couch, emulator prompts (`pages/games/*`, `Rigs.tsx`, `Couch.tsx`, `GameReview.tsx`) | WittyPlay Online boards (S5) cover the new experience; the existing web pages have no counterpart | decide which replace which |
| Photos timeline and map (`pages/photos/*`) | `photoTimeline`, `photoMap` as native views only | frame in each look |
| Books and comics library pages (`pages/Books.tsx`) | readers drawn, library pages thin | |
| Setup wizard (`pages/SetupWizard.tsx`, `pages/setup/*`) | OOBE boards (14 steps) | steps differ from today's; see below |
| Design page (`pages/DesignPage.tsx`) | no board | admin-only reference; keep |
| Help pages (`pages/HelpPage.tsx`, `help/*`) | no board | |
| Item, show and album detail pages (`ItemPage.tsx`, `ShowPage.tsx`) | detail boards | S12 owns the credits sections |
| WittyTV: recordings, sets, channel editor, tuner setup, reminders, search panel, encrypted help (`pages/tv/*`) | guide, banner, More info exist; these do not | recordings and sets are required by section 9 |
| Settings (all of `pages/settings/*`, `AppearanceSettings.tsx`, transcoding, logs, updates, backup, repair, licences) | settings board shows structure | Settings stays hand-built; only the new rows (Look, Theme, WittyTV look, Name and logo, Layout, Spacing) are drawn |

## Server contracts the web cannot build against yet

On `v0.8.0-alpha` there is no `GET /sdui/experiences`, no `experience=` on `/theme` or `/sdui/navigation`, no `setup_required` gate, no theme import route, no `brand.mark`, no required-look policy, no `finish`/`scene`/type-set tokens, and no `Carousel`, `CrossBar`, `Widget`, `NumberedRow` components or the new `navStyles`. The web builds items 1, 2, 3 and 8 of the owner's brief only once those exist (`DESIGN-2-SDUI-GAPS.md` section 7, steps 1 to 5).
