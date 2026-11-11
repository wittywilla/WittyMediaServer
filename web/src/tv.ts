// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// TV, remote-control and game-controller input for the whole app. The rule for
// which control is next lives in ui/spatial.ts; docs/DESIGN.md has the contract.
//
// - Arrow keys move focus to the nearest control in that direction (spatial
//   navigation), so every page works with a TV remote's D-pad. Inside the
//   player, arrows belong to the player (DVD menus, seeking) unless focus is
//   on the player's own controls; OK brings focus to those controls.
// - A remote has no Tab key, so nothing may hold on to the arrows for good:
//   text fields give them up at the ends of the text, drop-down lists and
//   sliders keep only what they need while open or in their own direction.
// - Back (TV remotes send GoBack/BrowserBack, Backspace on some boxes) goes
//   back a page.
// - Game controllers are read with the Gamepad API and turned into the same
//   key presses, by the mapping in ui/gamepad.ts. The controller is only polled
//   while one is connected, so this costs nothing otherwise.

import { padKeys } from './ui/gamepad'
import { isBackKey, nearest, ranked, type Dir } from './ui/spatial'

const FOCUSABLE = 'a[href], button:not([disabled]), summary, input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
// Inside the player only these areas take part in spatial navigation.
const PLAYER_CONTROLS = '.bottom, .top, .view-settings, .remote, .remote-bar, .mini-bar, .overlay-msg, .watch-next, .big-play, .skip-intro, .pick-intro, .ctv-mini, .tv-remind'
// The player that fills the screen. A player inside a page (.inline) is just a
// part of that page.
const FULL_PLAYER = '.player:not(.mini):not(.inline)'
// What has its own idea of Back, so that Back closes it rather than leaving the page. Apps that
// show this page read it from <html data-own-back>, rather than each keeping a copy.
export const OWN_BACK = `[role=dialog], [role=alertdialog], [role=menu], .modal, .picker-backdrop, .view-settings, .audio-bar.full, .reader, ${FULL_PLAYER}`
// What OK reaches first when the picture has the focus: whatever is asking
// for an answer, then the controls.
const PLAYER_FIRST = ['.big-play button', '.overlay-msg button', '.pick-intro button', '.skip-intro', '.watch-next button', '.bottom .big', '.bottom button:not([disabled])', '.top button:not([disabled])']

const DIRS: Record<string, Dir> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }

export function typing(el: Element | null): boolean {
  if (!el) return false
  const tag = el.tagName
  if (tag === 'TEXTAREA' || (el as HTMLElement).isContentEditable) return true
  if (tag === 'INPUT') return !['button', 'checkbox', 'radio', 'submit', 'reset', 'range'].includes((el as HTMLInputElement).type)
  return false
}

// Does this arrow still move the caret? At the ends of the text it doesn't,
// and it should move on to the next control instead.
function caretMoves(el: Element, dir: Dir): boolean {
  if ((el as HTMLElement).isContentEditable) return true
  const f = el as HTMLInputElement | HTMLTextAreaElement
  let start: number | null = null, end: number | null = null
  try { start = f.selectionStart; end = f.selectionEnd } catch { /* some input types have no caret */ }
  if (start == null || end == null) return false
  if (el.tagName !== 'TEXTAREA' && (dir === 'up' || dir === 'down')) return false
  return dir === 'up' || dir === 'left' ? end > 0 : start < f.value.length
}

// A drop-down list that is open has the arrows (browsers mostly keep those
// keys to themselves anyway); a closed one must not change value just because
// focus is passing through. OK opens it.
let openSelect: Element | null = null
function openList(el: HTMLSelectElement) {
  openSelect = el
  try { el.showPicker() } catch { /* the browser opens it by itself, or can't */ }
}

function visible(el: HTMLElement, r: DOMRect): boolean {
  if (r.width < 2 || r.height < 2) return false
  if (el.closest('[aria-hidden="true"], [inert]')) return false
  // Inside a closed section (its summary stays reachable, to open it).
  const closed = el.closest('details:not([open])')
  if (closed && !el.closest('summary')) return false
  const s = getComputedStyle(el)
  return s.visibility !== 'hidden' && s.pointerEvents !== 'none' && Number(s.opacity) > 0.05
}

// Scrolled out of sight in a list that the focus isn't in. It still has a position, which can
// be right over something else on the page, so it mustn't be chosen from outside: moving
// along that list reaches it, and from outside, what can be seen in the list comes first.
function hiddenInScroller(el: HTMLElement, r: DOMRect, cur: Element): boolean {
  for (let p = el.parentElement; p && p !== document.body && p !== document.documentElement; p = p.parentElement) {
    const s = getComputedStyle(p)
    if (s.overflowX === 'visible' && s.overflowY === 'visible') continue
    if (p.contains(cur)) return false
    const b = p.getBoundingClientRect()
    if (r.bottom <= b.top + 1 || r.top >= b.bottom - 1 || r.right <= b.left + 1 || r.left >= b.right - 1) return true
  }
  return false
}

// The area focus may move within: the innermost open dialog, the player's
// controls, or the whole page.
function scopeOf(el: Element | null): ParentNode {
  const dialog = el?.closest('[role=dialog], [role=alertdialog], [role=menu], .picker-backdrop, .modal')
  if (dialog) return dialog
  const openDialog = document.querySelector('[role=menu], .picker-backdrop, .modal-backdrop .modal, .view-settings')
  if (openDialog) return openDialog
  // The full audio player covers the page on anything narrower than the docked panel.
  const fullAudio = document.querySelector('.audio-bar.full')
  if (fullAudio && innerWidth < 1000) return fullAudio
  const player = document.querySelector(FULL_PLAYER)
  if (player) return player
  return document
}

// A row, a menu or any other set of controls that belong together: coming back
// to one lands where you left it, not on whatever happens to be nearest.
const GROUPS = '[data-nav-group], [data-nav-row], .row, .topnav, .sidebar, .set-nav, .tabbar, .section-row'
const lastIn = new WeakMap<Element, HTMLElement>()
const groupOf = (el: Element | null) => el?.closest(GROUPS) || null
const COLUMNS = '.sidebar, .set-nav'

// Where the focus last was. A button can take itself away when pressed (a row that's removed,
// a list that closes), and a remote has no other way back to where that was.
let lastFocus: { box: DOMRect; scope: Node; page: string } | null = null

function focusGoneFrom(scope: Node): DOMRect | null {
  const l = lastFocus
  // The same dialog or page: a dialog can be found by more than one of its layers.
  if (!l || l.page !== location.pathname || !l.scope.isConnected || !(l.scope.contains(scope) || scope.contains(l.scope))) return null
  return l.box
}

// A page that knows its own order (a server-driven page: sdui/SduiPage.tsx) answers the arrows inside itself.
// true: it moved focus; false: it has the key and nothing moves; undefined: not its to answer.
type PageMove = (cur: HTMLElement, dir: Dir, focus: (el: HTMLElement) => void) => boolean | undefined
const pageMoves = new Map<HTMLElement, PageMove>()
export function setPageMove(page: HTMLElement, fn: PageMove): () => void {
  pageMoves.set(page, fn)
  return () => { if (pageMoves.get(page) === fn) pageMoves.delete(page) }
}

function move(dir: Dir): boolean {
  const cur = document.activeElement as HTMLElement | null
  const scope = scopeOf(cur)
  if (cur && scope === document) {
    for (const [page, fn] of pageMoves) {
      if (!page.contains(cur)) continue
      const done = fn(cur, dir, focus)
      if (done !== undefined) return done
    }
  }
  // data-nav-skip: part of something with its own arrows, such as a programme grid,
  // which is one stop from outside however many cells are drawn.
  const all = Array.from(scope.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => !(el.closest(FULL_PLAYER) && !el.closest(PLAYER_CONTROLS)) && !el.hasAttribute('data-nav-skip'))
  if (!all.length) return false
  // Read every position first, in one go, then look closer only at the likeliest few:
  // a library can have thousands of cards, and this runs on every key press.
  const boxes = new Map(all.map((el) => [el, el.getBoundingClientRect()]))
  if (!cur || cur === document.body || !boxes.has(cur) || !visible(cur, boxes.get(cur)!)) {
    const gone = focusGoneFrom(scope as Node)
    const near = gone && nearest(gone, all.filter((el) => visible(el, boxes.get(el)!) && !hiddenInScroller(el, boxes.get(el)!, document.body)), (el) => boxes.get(el)!)
    if (near) { focus(near); return true }
    // Nothing focused yet: start where this page's main action is, or at the first control of the main content.
    const first = firstOf(all.filter((el) => el.closest('.content, .player, [role=dialog]')), boxes) || all.find((el) => visible(el, boxes.get(el)!))
    if (!first) return false
    focus(first)
    return true
  }
  const others = all.filter((el) => el !== cur && !el.contains(cur) && !cur.contains(el))
  const can = (el: HTMLElement) => visible(el, boxes.get(el)!) && !hiddenInScroller(el, boxes.get(el)!, cur)
  // A row's own heading: the row is what's under it, even with few cards and See all far off to
  // the right, where what's straight down is the next row's See all and to the left the side menu.
  const head = cur.closest('.hub-head, [data-nav-head]')
  if (head && (dir === 'down' || dir === 'left')) {
    const inRow = others.filter((el) => head.parentElement?.contains(el) && !head.contains(el) && can(el))
    const row = inRow[0] && groupOf(inRow[0])
    const left = row && lastIn.get(row)
    const to = left && inRow.includes(left) ? left : nearest(boxes.get(cur)!, inRow, (el) => boxes.get(el)!)
    if (to) { focus(to); return true }
  }
  // A side column is left and entered sideways. Going down a page mustn't fall into the side menu because the
  // menu's last link happens to be lower, and the menu's last link mustn't fall into the page.
  const col = cur.closest(COLUMNS)
  // data-nav-row: one line of things, such as a games shelf, which up and down always leave. What stands on a
  // shelf can be of different heights, and the focused one lifts, so a neighbour's middle can sit lower.
  const line = dir === 'up' || dir === 'down' ? cur.closest('[data-nav-row]') : null
  const onward = (dir === 'left' || dir === 'right' ? others : others.filter((el) => { const c = el.closest(COLUMNS); return c === col || (col?.matches('.set-nav') && el.closest('.topbar')) }))
    .filter((el) => !line?.contains(el))
  let best = ranked(boxes.get(cur)!, onward, (el) => boxes.get(el)!, dir).find(can)
  // A programme grid is never jumped over on the way up or down the page, however far to the side it starts
  // and however close what's beyond it. It's the reason for the page.
  const grid = best && (dir === 'up' || dir === 'down') ? scope.querySelector('[role=grid]') : null
  if (best && grid) {
    const a = boxes.get(cur)!, b = boxes.get(best)!
    const between = (s: DOMRect) => (dir === 'down' ? s.top >= a.bottom && s.bottom <= b.top : s.bottom <= a.top && s.top >= b.bottom)
    best = others.find((el) => grid.contains(el) && can(el) && between(boxes.get(el)!)) || best
  }
  // Past the end of a row: on to its See all if it has one. What's off to the side on some other row isn't
  // where someone going along this one means to go.
  const row = cur.closest('.row, [data-nav-group], [data-nav-row]')
  if (best && dir === 'right' && row && !row.contains(best)) {
    const a = boxes.get(cur)!, b = boxes.get(best)!
    if (b.top >= a.bottom || b.bottom <= a.top) {
      // A row's heading is a .hub-head, or data-nav-head beside a data-nav-row.
      const seeAll = row.closest('.hub')?.querySelector<HTMLElement>('.hub-head a') || row.parentElement?.querySelector<HTMLElement>(':scope > [data-nav-head] a')
      if (seeAll) focus(seeAll)
      return !!seeAll
    }
  }
  if (best) {
    // Into another group: back to where it was left, as long as that is this way too.
    const group = groupOf(best)
    const into = group && group !== groupOf(cur)
    const memory = into ? lastIn.get(group) : undefined
    if (memory?.isConnected && memory !== best && boxes.has(memory) && can(memory) && ranked(boxes.get(cur)!, [memory], (el) => boxes.get(el)!, dir).length) best = memory
    // A group that says where it's entered is entered there, from wherever: TV mode's top bar, on the chosen tab.
    const entry = into ? group.getAttribute('data-nav-entry') : null
    const at = entry ? group!.querySelector<HTMLElement>(entry) : null
    if (at && boxes.has(at) && can(at)) best = at
  }
  if (!best) {
    // Leaving the player's controls upwards hands the arrows back to the player.
    if (cur.closest(FULL_PLAYER) && (dir === 'up' || dir === 'down')) { (cur.closest(FULL_PLAYER) as HTMLElement).focus(); return true }
    return false
  }
  focus(best)
  return true
}

// Where a remote starts on a page: its main action if that's on the screen, or else its first control that isn't
// a field, as landing in a text box can open a TV's on-screen keyboard.
function firstOf(list: HTMLElement[], boxes: Map<HTMLElement, DOMRect>): HTMLElement | undefined {
  const shown = list.filter((el) => visible(el, boxes.get(el)!) && !hiddenInScroller(el, boxes.get(el)!, document.body))
  const onScreen = (el: HTMLElement) => { const r = boxes.get(el)!; return r.top >= 0 && r.bottom <= window.innerHeight }
  return shown.find((el) => el.matches('[data-tv-first]')) || shown.find((el) => el.matches('.btn.primary') && onScreen(el)) || shown.find((el) => !typing(el) && !el.matches('select') && !el.closest('.hub-head, .mh-shelf-head') && onScreen(el)) || shown[0]
}

// Up with nothing above it, on the page itself: an app showing this page under a bar of its own (Android TV's
// top bar over the guide) can't see that from outside, as the page has the arrows. So, as for Back, it says
// where to send it, <html data-app-up="/address">, and the page goes there for the app to catch. Not from a
// dialog, a menu or the player, whose arrows stay in them.
function upToApp(target: Element | null, e: KeyboardEvent) {
  const out = document.documentElement.dataset.appUp
  if (!out || !target || target === document.body || scopeOf(target) !== document) return
  e.preventDefault()
  window.location.assign(out)
}

// focusFirst: for a page that has just arrived on a screen across the room, where a remote has
// nothing to see until something has the focus.
// A TV's web view gives focus to some link of its own choosing when it opens. That wasn't anyone's decision,
// and it doesn't count, until someone presses something.
let lastKeyAt = -1
export function focusIsAnyonesChoice(el: Element | null, arrived: number): boolean {
  if (!el || el === document.body || el.id === 'main') return false
  return lastKeyAt > arrived || !el.closest('.sidebar, .topbar, .legal-note, .page-foot')
}

export function focusFirst(): boolean {
  const main = document.getElementById('main')
  if (!main || document.querySelector('[role=dialog], [role=alertdialog], [role=menu]')) return false
  // Still arriving: there's nothing yet that the page is about, so leave it for the next look.
  if (main.querySelector('.loading-box, .skeleton-grid')) return false
  const all = Array.from(main.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => !el.hasAttribute('data-nav-skip') && !el.closest('.legal-note'))
  // A settings page starts on the page itself, not in the menu beside it.
  const page = all.filter((el) => !el.closest('.set-nav'))
  const list = page.length ? page : all
  const first = firstOf(list, new Map(list.map((el) => [el, el.getBoundingClientRect()])))
  if (!first) return false
  first.focus({ preventScroll: true })
  return true
}

// Focused items are kept well inside the screen (TVs crop the edges), with
// rows scrolling sideways to follow.
function focus(el: HTMLElement) {
  el.focus({ preventScroll: true })
  const r = el.getBoundingClientRect(), h = window.innerHeight
  const block = r.top < h * 0.15 || r.bottom > h * 0.85 ? 'center' : 'nearest'
  el.scrollIntoView({ block, inline: 'nearest', behavior: reducedMotion() ? 'auto' : 'smooth' })
}

const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

// focusPlayerControls moves focus from the picture to the player's controls:
// the top row or the rest. False when there are none to go to.
export function focusPlayerControls(where: 'top' | 'first' = 'first'): boolean {
  const player = document.querySelector('.player:not(.mini)')
  for (const sel of where === 'top' ? ['.top button:not([disabled])'] : PLAYER_FIRST) {
    const el = player?.querySelector<HTMLElement>(sel)
    if (el) { el.focus({ preventScroll: true }); return true }
  }
  return false
}
// While a game is being played its controls belong to the game; its menus use the
// normal navigation again.
const inGame = () => !!document.querySelector('.wp-playing, .wp-capture')

function onKey(e: KeyboardEvent) {
  lastKeyAt = performance.now()
  if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || inGame()) return
  const target = e.target as Element | null
  if (e.key === 'Enter' && target) {
    // OK on the picture brings up the controls. Disc menus keep it for their
    // buttons: the player uses it first if one is selected. A disc's Java menu draws
    // its own buttons and has the remote, so its keys must stay with it.
    if (target.matches('.player:not(.mini)') && !target.querySelector('.dvd-layer .hit, .bdj-layer')) {
      if (focusPlayerControls()) e.preventDefault()
      return
    }
    // In a book the same: OK brings up its toolbar.
    if ((target === document.body || target.matches('.reader')) && document.querySelector('.reader')) {
      const bar = document.querySelector<HTMLElement>('.reader-bar button:not([disabled])')
      if (bar) { e.preventDefault(); bar.focus({ preventScroll: true }) }
      return
    }
    // Nothing has the focus yet on a page still arriving: OK shows where it starts, rather than doing nothing.
    if ((target === document.body || target.id === 'main') && !document.querySelector(FULL_PLAYER) && document.documentElement.classList.contains('using-controller')) {
      if (focusFirst()) e.preventDefault()
      return
    }
    if (target.tagName === 'SELECT') { e.preventDefault(); openList(target as HTMLSelectElement); return }
    // A native checkbox or radio button answers only to the space bar, and a remote has none.
    if (target.matches('input[type=checkbox], input[type=radio]')) { e.preventDefault(); (target as HTMLElement).click(); return }
    // Things that act as links or buttons without being them (a card row).
    if (target.matches('[tabindex="0"][role=link], [tabindex="0"][role=button]')) { e.preventDefault(); (target as HTMLElement).click(); return }
  }
  const dir = DIRS[e.key]
  if (dir) {
    if (target === openSelect && target?.tagName === 'SELECT') return
    if (typing(target) && caretMoves(target!, dir)) return
    // Sliders use the arrows along their length, and let the others go.
    if (target?.closest('[role=slider], input[type=range]') && (dir === 'left' || dir === 'right')) return
    // In the player, arrows drive DVD menus and seeking unless a control has focus.
    if (target?.closest(FULL_PLAYER) && !target.closest(PLAYER_CONTROLS) && !target.closest('[role=dialog]')) return
    // With nothing focused the arrows are the player's too: on live TV they change channel.
    if ((!target || target === document.body) && document.querySelector(FULL_PLAYER)) return
    if (target?.closest('.picker-grid')) return // the tile picker has its own grid navigation
    // In a book, arrows turn pages and scroll unless focus is on its toolbars.
    const reader = document.querySelector('.reader')
    if (reader) {
      // The music player's bar can sit under a book too, and counts as one of its toolbars.
      const bar = target?.closest('.reader-bar, .reader-foot, .audio-bar:not(.full), [role=dialog], .rd-panel')
      if (!bar) return
      // The book's toolbars hide themselves until they have the focus, so the arrows can't see them:
      // going up or down between them, or up from the music player, goes straight to the next one.
      const across = dir === 'down' && bar.matches('.reader-bar') ? '.reader-foot'
        : dir === 'up' && bar.matches('.reader-foot') ? '.reader-bar'
        : dir === 'up' && bar.matches('.audio-bar:not(.full)') ? '.reader-foot' : ''
      const to = across && document.querySelector<HTMLElement>(`${across} button:not([disabled]), ${across} input:not([disabled])`)
      if (to) { to.focus({ preventScroll: true }); e.preventDefault(); e.stopImmediatePropagation(); return }
      if (move(dir)) { e.preventDefault(); e.stopImmediatePropagation(); return } // or the page would turn too
      if (dir === 'up' || dir === 'down') { (target as HTMLElement).blur(); e.preventDefault(); e.stopImmediatePropagation() }
      return
    }
    const left = move(dir)
    if (left || target?.tagName === 'SELECT') e.preventDefault()
    else if (dir === 'up') upToApp(target, e)
    return
  }
  if (isBackKey(e) || (e.key === 'Backspace' && !typing(target))) {
    if (document.querySelector(FULL_PLAYER) || document.querySelector('[role=dialog], [role=alertdialog], .modal')) return // handled there
    e.preventDefault()
    // At the first page shown in an app's web view, Back belongs to the app. A controller's B can't reach
    // it any other way: the web view keeps a controller's buttons for the page.
    if (document.documentElement.dataset.appBack && (window.history.state as { idx?: number } | null)?.idx === 0) {
      window.location.assign(document.documentElement.dataset.appBack)
      return
    }
    window.history.back()
  }
}

// ---------------------------------------------------------------- controllers

function press(key: string) {
  const target = (document.activeElement as HTMLElement | null) || document.body
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
  target.dispatchEvent(ev)
  // Synthetic key events do not activate buttons; do it when nothing else used the key.
  if (!ev.defaultPrevented && (key === 'Enter' || key === ' ') && target !== document.body && target.matches('button, a[href], [role=button], [role=radio], [role=switch], summary, input[type=checkbox]')) {
    if (key === 'Enter' || !target.closest('.player')) target.click()
  }
  if (!ev.defaultPrevented && key === 'Escape') {
    const back = new KeyboardEvent('keydown', { key: 'GoBack', bubbles: true, cancelable: true })
    target.dispatchEvent(back)
  }
}

// How often a connected controller is read. On a timer and not on animation frames, because
// frames can stop for up to half a second while timers keep their time, and a press that
// starts and ends in that gap would be lost. Asking for no frames also lets the browser draw
// nothing while nothing moves.
const POLL_MS = 12
let pollTimer = 0
const held = new Map<string, number>() // key -> next repeat time

// What the last read found, so a pad with nothing new costs a comparison and no work: this runs
// about 80 times a second on the weakest thing it's installed on. A pad's timestamp moves when its
// state does; every so often it's read anyway, for a browser that never moves it.
const FULL_READ_EVERY = 8
const lastStamps: number[] = []
let lastDown = new Set<string>()
let sinceFullRead = 0

// The pads seen to report their D-pad on one axis (see gamepad.ts).
const hatPads = new Set<string>()
// Read each time, as it was before the mapping moved out: cheap, and a test can change it.
const keysDown = (pads: readonly (Gamepad | null)[]) => padKeys(pads, { android: /Android/.test(navigator.userAgent), hats: hatPads })

function poll() {
  const pads = navigator.getGamepads ? navigator.getGamepads() : []
  let connected = false
  let changed = ++sinceFullRead >= FULL_READ_EVERY
  for (let i = 0; i < pads.length; i++) {
    const p = pads[i]
    const stamp = p ? p.timestamp || 0 : -1
    if (p) connected = true
    if (stamp === 0 || stamp !== lastStamps[i]) { lastStamps[i] = stamp; changed = true }
  }
  if (!connected) { clearInterval(pollTimer); pollTimer = 0; held.clear(); lastDown = new Set(); lastStamps.length = 0; return }
  if (document.hidden) { held.clear(); return } // nobody is looking, and browsers stop updating pads
  if (changed) { lastDown = keysDown(pads); sinceFullRead = 0 }
  const down = lastDown
  if (inGame()) {
    // Whatever is down belongs to the game, including the buttons that open its menu: they
    // aren't presses in that menu until they've been let go and pressed again.
    held.clear()
    for (const k of down) held.set(k, Infinity)
    return
  }
  if (!down.size && !held.size) return
  const now = performance.now()
  for (const k of down) {
    const next = held.get(k)
    if (next === undefined) { press(k); held.set(k, now + 420) } // first press, then repeat after a pause
    else if (now >= next && k.startsWith('Arrow')) { press(k); held.set(k, now + 110) }
  }
  for (const k of held.keys()) if (!down.has(k)) held.delete(k)
}

function startPolling() {
  if (pollTimer) return
  document.documentElement.classList.add('using-controller')
  pollTimer = window.setInterval(poll, POLL_MS)
}

let installed = false

export function installTVInput() {
  if (installed) return
  installed = true
  // A TV app around this page answers Back itself when the page has nothing to close, and asks here.
  document.documentElement.dataset.ownBack = OWN_BACK
  window.addEventListener('keydown', onKey)
  window.addEventListener('focusin', (e) => {
    const el = e.target as HTMLElement
    const group = groupOf(el)
    if (group) lastIn.set(group, el)
    // The page itself takes focus when it changes. That's no place to come back near: its centre
    // would decide the first press.
    if (el.matches(FOCUSABLE)) lastFocus = { box: el.getBoundingClientRect(), scope: scopeOf(el) as Node, page: location.pathname }
  }, { passive: true })
  // A list is open from the moment it is pressed until a choice or leaving it.
  window.addEventListener('mousedown', (e) => { openSelect = (e.target as Element)?.tagName === 'SELECT' ? (e.target as Element) : null }, { passive: true })
  for (const ev of ['change', 'focusout'] as const) window.addEventListener(ev, () => { openSelect = null })
  window.addEventListener('gamepadconnected', startPolling)
  // Mouse use hides the controller focus styling again, unless this screen is set up for across the room.
  window.addEventListener('mousedown', () => { if (!document.documentElement.classList.contains('ten-foot')) document.documentElement.classList.remove('using-controller') }, { passive: true })
  if (navigator.getGamepads && Array.from(navigator.getGamepads()).some(Boolean)) startPolling()
}
