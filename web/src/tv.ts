// TV, remote-control and game-controller input for the whole app.
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

const FOCUSABLE = 'a[href], button:not([disabled]), summary, input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
// Inside the player only these areas take part in spatial navigation.
const PLAYER_CONTROLS = '.bottom, .top, .view-settings, .remote, .remote-bar, .mini-bar, .overlay-msg, .watch-next, .big-play, .skip-intro'
// The player that fills the screen. A player inside a page (.inline) is just a
// part of that page.
const FULL_PLAYER = '.player:not(.mini):not(.inline)'
// What OK reaches first when the picture has the focus: whatever is asking
// for an answer, then the controls.
const PLAYER_FIRST = ['.big-play button', '.overlay-msg button', '.skip-intro', '.watch-next button', '.bottom .big', '.bottom button:not([disabled])', '.top button:not([disabled])']

type Dir = 'up' | 'down' | 'left' | 'right'
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

// The area focus may move within: the innermost open dialog, the player's
// controls, or the whole page.
function scopeOf(el: Element | null): ParentNode {
  const dialog = el?.closest('[role=dialog], .picker-backdrop, .modal')
  if (dialog) return dialog
  const openDialog = document.querySelector('.picker-backdrop, .modal-backdrop .modal, .view-settings')
  if (openDialog) return openDialog
  const player = document.querySelector(FULL_PLAYER)
  if (player) return player
  return document
}

function move(dir: Dir): boolean {
  const cur = document.activeElement as HTMLElement | null
  const scope = scopeOf(cur)
  const all = Array.from(scope.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => !(el.closest(FULL_PLAYER) && !el.closest(PLAYER_CONTROLS)))
  const cands = all.map((el) => ({ el, r: el.getBoundingClientRect() })).filter((c) => visible(c.el, c.r))
  if (!cands.length) return false
  if (!cur || cur === document.body || !cands.some((c) => c.el === cur)) {
    // Nothing focused yet: start at the first control of the main content.
    const first = cands.find((c) => c.el.closest('.content, .player, [role=dialog]')) || cands[0]
    focus(first.el)
    return true
  }
  const a = cur.getBoundingClientRect()
  const ax = a.left + a.width / 2, ay = a.top + a.height / 2
  let best: HTMLElement | null = null, bestScore = Infinity
  for (const { el, r } of cands) {
    if (el === cur || el.contains(cur) || cur.contains(el)) continue
    const bx = r.left + r.width / 2, by = r.top + r.height / 2
    let primary: number, orth: number
    switch (dir) {
      case 'right': primary = r.left - a.right; orth = Math.abs(by - ay); if (bx <= ax + 1) continue; break
      case 'left': primary = a.left - r.right; orth = Math.abs(by - ay); if (bx >= ax - 1) continue; break
      case 'down': primary = r.top - a.bottom; orth = Math.abs(bx - ax); if (by <= ay + 1) continue; break
      default: primary = a.top - r.bottom; orth = Math.abs(bx - ax); if (by >= ay - 1) continue
    }
    // Prefer controls in line with the current one; overlap counts as 0 distance.
    const score = Math.max(0, primary) + orth * (dir === 'up' || dir === 'down' ? 0.6 : 2.2)
    if (score < bestScore) { bestScore = score; best = el }
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
  if (e.key === 'GoBack' || e.key === 'BrowserBack' || (e.key === 'Backspace' && !typing(target))) {
    if (document.querySelector(FULL_PLAYER) || document.querySelector('[role=dialog], .modal')) return // handled there
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

let polling = false
const held = new Map<string, number>() // key -> next repeat time

function poll() {
  const pads = navigator.getGamepads ? Array.from(navigator.getGamepads()).filter(Boolean) as Gamepad[] : []
  if (!pads.length) { polling = false; held.clear(); return }
  if (inGame()) { held.clear(); requestAnimationFrame(poll); return }
  const now = performance.now()
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
  for (const k of down) {
    const next = held.get(k)
    if (next === undefined) { press(k); held.set(k, now + 420) } // first press, then repeat after a pause
    else if (now >= next && k.startsWith('Arrow')) { press(k); held.set(k, now + 110) }
  }
  for (const k of [...held.keys()]) if (!down.has(k)) held.delete(k)
  requestAnimationFrame(poll)
}

function startPolling() {
  if (polling) return
  polling = true
  document.documentElement.classList.add('using-controller')
  requestAnimationFrame(poll)
}

let installed = false

export function installTVInput() {
  if (installed) return
  installed = true
  window.addEventListener('keydown', onKey)
  // A list is open from the moment it is pressed until a choice or leaving it.
  window.addEventListener('mousedown', (e) => { openSelect = (e.target as Element)?.tagName === 'SELECT' ? (e.target as Element) : null }, { passive: true })
  for (const ev of ['change', 'focusout'] as const) window.addEventListener(ev, () => { openSelect = null })
  window.addEventListener('gamepadconnected', startPolling)
  // Mouse use hides the controller focus styling again.
  window.addEventListener('mousedown', () => document.documentElement.classList.remove('using-controller'), { passive: true })
  if (navigator.getGamepads && Array.from(navigator.getGamepads()).some(Boolean)) startPolling()
}
