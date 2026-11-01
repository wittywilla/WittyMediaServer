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
//   key presses. The browser's "standard" mapping covers XInput/Xbox,
//   DualShock, DualSense and most generic pads; Nintendo layouts get A/B
//   swapped so the right-hand button confirms; non-standard pads fall back
//   to their first axes and buttons. The controller is only polled while one
//   is connected, so this costs nothing otherwise.

import { isBackKey, nearest, ranked, type Dir } from './ui/spatial'

const FOCUSABLE = 'a[href], button:not([disabled]), summary, input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
// Inside the player only these areas take part in spatial navigation.
const PLAYER_CONTROLS = '.bottom, .top, .view-settings, .remote, .remote-bar, .mini-bar, .overlay-msg, .watch-next, .big-play, .skip-intro, .pick-intro, .ctv-mini, .tv-remind'
// The player that fills the screen. A player inside a page (.inline) is just a
// part of that page.
const FULL_PLAYER = '.player:not(.mini):not(.inline)'
// What OK reaches first when the picture has the focus: whatever is asking
// for an answer, then the controls.
const PLAYER_FIRST = ['.big-play button', '.overlay-msg button', '.pick-intro button', '.skip-intro', '.watch-next button', '.bottom .big', '.bottom button:not([disabled])', '.top button:not([disabled])']

const DIRS: Record<string, Dir> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }

function typing(el: Element | null): boolean {
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
const GROUPS = '[data-nav-group], .row, .topnav, .sidebar, .set-nav, .tabbar, .section-row'
const lastIn = new WeakMap<Element, HTMLElement>()
const groupOf = (el: Element | null) => el?.closest(GROUPS) || null

// Where the focus last was. A button can take itself away when pressed (a row that's removed,
// a list that closes), and a remote has no other way back to where that was.
let lastFocus: { box: DOMRect; scope: Node; page: string } | null = null

function focusGoneFrom(scope: Node): DOMRect | null {
  const l = lastFocus
  // The same dialog or page: a dialog can be found by more than one of its layers.
  if (!l || l.page !== location.pathname || !l.scope.isConnected || !(l.scope.contains(scope) || scope.contains(l.scope))) return null
  return l.box
}

function move(dir: Dir): boolean {
  const cur = document.activeElement as HTMLElement | null
  const scope = scopeOf(cur)
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
    // Nothing focused yet: start at the first control of the main content.
    const first = all.find((el) => el.closest('.content, .player, [role=dialog]') && visible(el, boxes.get(el)!)) || all.find((el) => visible(el, boxes.get(el)!))
    if (!first) return false
    focus(first)
    return true
  }
  const others = all.filter((el) => el !== cur && !el.contains(cur) && !cur.contains(el))
  const can = (el: HTMLElement) => visible(el, boxes.get(el)!) && !hiddenInScroller(el, boxes.get(el)!, cur)
  let best = ranked(boxes.get(cur)!, others, (el) => boxes.get(el)!, dir).find(can)
  if (best) {
    // Into another group: back to where it was left, as long as that is this way too.
    const group = groupOf(best)
    const memory = group && group !== groupOf(cur) ? lastIn.get(group) : undefined
    if (memory?.isConnected && memory !== best && boxes.has(memory) && can(memory) && ranked(boxes.get(cur)!, [memory], (el) => boxes.get(el)!, dir).length) best = memory
  }
  if (!best) {
    // Leaving the player's controls upwards hands the arrows back to the player.
    if (cur.closest(FULL_PLAYER) && (dir === 'up' || dir === 'down')) { (cur.closest(FULL_PLAYER) as HTMLElement).focus(); return true }
    return false
  }
  focus(best)
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
    if (target.tagName === 'SELECT') { e.preventDefault(); openList(target as HTMLSelectElement); return }
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
    if (target?.closest('.picker-grid')) return // the tile picker has its own grid navigation
    // In a book, arrows turn pages and scroll unless focus is on its toolbars.
    const reader = document.querySelector('.reader')
    if (reader) {
      const bar = target?.closest('.reader-bar, .reader-foot, [role=dialog], .rd-panel')
      if (!bar) return
      if (move(dir)) { e.preventDefault(); e.stopImmediatePropagation(); return } // or the page would turn too
      if (dir === 'up' || dir === 'down') { (target as HTMLElement).blur(); e.preventDefault(); e.stopImmediatePropagation() }
      return
    }
    const left = move(dir)
    if (left || target?.tagName === 'SELECT') e.preventDefault()
    return
  }
  if (isBackKey(e) || (e.key === 'Backspace' && !typing(target))) {
    if (document.querySelector(FULL_PLAYER) || document.querySelector('[role=dialog], [role=alertdialog], .modal')) return // handled there
    e.preventDefault()
    window.history.back()
  }
}

// ---------------------------------------------------------------- controllers

const STD: Record<number, string> = {
  0: 'Enter', 1: 'Escape', 2: ' ', 3: 'm', // A/Cross, B/Circle, X/Square (play/pause), Y/Triangle (disc menu)
  4: 'PageUp', 5: 'PageDown', // bumpers: previous / next chapter
  6: 'MediaRewind', 7: 'MediaFastForward', // triggers
  8: 't', 9: 'ContextMenu', // View/Share/Select: title menu, Menu/Options/Start: disc menu
  12: 'ArrowUp', 13: 'ArrowDown', 14: 'ArrowLeft', 15: 'ArrowRight',
}

const isNintendo = (id: string) => /nintendo|pro controller|joy-?con|057e/i.test(id)

// A D-pad some pads report as one axis: -1 at up, on round clockwise in eight steps, and
// out of that range at rest, which is what gives one away. Read as a stick it would
// look like a direction held down for good.
const hatPads = new Set<string>()
const HAT_KEYS: string[][] = [['ArrowUp'], ['ArrowUp', 'ArrowRight'], ['ArrowRight'], ['ArrowDown', 'ArrowRight'], ['ArrowDown'], ['ArrowDown', 'ArrowLeft'], ['ArrowLeft'], ['ArrowUp', 'ArrowLeft']]

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

// The keys the controllers are holding down at the moment.
function keysDown(pads: Gamepad[]) {
  const down = new Set<string>()
  for (const p of pads) {
    const swap = isNintendo(p.id)
    if (p.mapping === 'standard') {
      p.buttons.forEach((b, i) => {
        if (!b.pressed) return
        let k = STD[i]
        if (swap && (i === 0 || i === 1)) k = STD[1 - i]
        if (k) down.add(k)
      })
    } else {
      // Unknown layout: first two buttons confirm/back, last axes pair is often the D-pad hat.
      if (p.buttons[0]?.pressed) down.add(swap ? 'Escape' : 'Enter')
      if (p.buttons[1]?.pressed) down.add(swap ? 'Enter' : 'Escape')
    }
    // Left stick (and D-pad axes on non-standard pads) as arrows, with a dead zone.
    const last = p.axes[p.axes.length - 1] ?? 0
    if (p.mapping !== 'standard' && Math.abs(last) > 1.05) hatPads.add(p.id)
    const hat = hatPads.has(p.id)
    if (hat && Math.abs(last) <= 1.05) for (const k of HAT_KEYS[Math.round((last + 1) * 3.5) & 7]) down.add(k)
    const axes = p.mapping === 'standard' || hat ? [[0, 1]] : [[0, 1], [p.axes.length - 2, p.axes.length - 1]]
    for (const [x, y] of axes) {
      const ax = p.axes[x] ?? 0, ay = p.axes[y] ?? 0
      if (Math.abs(ax) > 0.6 && Math.abs(ax) >= Math.abs(ay)) down.add(ax > 0 ? 'ArrowRight' : 'ArrowLeft')
      else if (Math.abs(ay) > 0.6) down.add(ay > 0 ? 'ArrowDown' : 'ArrowUp')
    }
  }
  return down
}

function poll() {
  const pads = navigator.getGamepads ? Array.from(navigator.getGamepads()).filter(Boolean) as Gamepad[] : []
  if (!pads.length) { clearInterval(pollTimer); pollTimer = 0; held.clear(); return }
  if (document.hidden) { held.clear(); return } // nobody is looking, and browsers stop updating pads
  const down = keysDown(pads)
  if (inGame()) {
    // Whatever is down belongs to the game, including the buttons that open its menu: they
    // aren't presses in that menu until they've been let go and pressed again.
    held.clear()
    for (const k of down) held.set(k, Infinity)
    return
  }
  const now = performance.now()
  for (const k of down) {
    const next = held.get(k)
    if (next === undefined) { press(k); held.set(k, now + 420) } // first press, then repeat after a pause
    else if (now >= next && k.startsWith('Arrow')) { press(k); held.set(k, now + 110) }
  }
  for (const k of [...held.keys()]) if (!down.has(k)) held.delete(k)
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
  window.addEventListener('keydown', onKey)
  window.addEventListener('focusin', (e) => {
    const el = e.target as HTMLElement
    const group = groupOf(el)
    if (group) lastIn.set(group, el)
    lastFocus = { box: el.getBoundingClientRect(), scope: scopeOf(el) as Node, page: location.pathname }
  }, { passive: true })
  // A list is open from the moment it is pressed until a choice or leaving it.
  window.addEventListener('mousedown', (e) => { openSelect = (e.target as Element)?.tagName === 'SELECT' ? (e.target as Element) : null }, { passive: true })
  for (const ev of ['change', 'focusout'] as const) window.addEventListener(ev, () => { openSelect = null })
  window.addEventListener('gamepadconnected', startPolling)
  // Mouse use hides the controller focus styling again, unless this screen is set up for across the room.
  window.addEventListener('mousedown', () => { if (!document.documentElement.classList.contains('ten-foot')) document.documentElement.classList.remove('using-controller') }, { passive: true })
  if (navigator.getGamepads && Array.from(navigator.getGamepads()).some(Boolean)) startPolling()
}
