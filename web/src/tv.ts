// TV, remote-control and game-controller input for the whole app.
//
// - Arrow keys move focus to the nearest control in that direction (spatial
//   navigation), so every page works with a TV remote's D-pad. Inside the
//   player, arrows belong to the player (DVD menus, seeking) unless focus is
//   on the player's own controls.
// - Back (TV remotes send GoBack/BrowserBack, Backspace on some boxes) goes
//   back a page.
// - Game controllers are read with the Gamepad API and turned into the same
//   key presses. The browser's "standard" mapping covers XInput/Xbox,
//   DualShock, DualSense and most generic pads; Nintendo layouts get A/B
//   swapped so the right-hand button confirms; non-standard pads fall back
//   to their first axes and buttons. The controller is only polled while one
//   is connected, so this costs nothing otherwise.

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
// Inside the player only these areas take part in spatial navigation.
const PLAYER_CONTROLS = '.bottom, .top, .view-settings, .remote, .remote-bar, .mini-bar, .overlay-msg, .watch-next'
// The player that fills the screen. A player inside a page (.inline) is just a
// part of that page.
const FULL_PLAYER = '.player:not(.mini):not(.inline)'

type Dir = 'up' | 'down' | 'left' | 'right'
const DIRS: Record<string, Dir> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }

function typing(el: Element | null): boolean {
  if (!el) return false
  const tag = el.tagName
  if (tag === 'TEXTAREA' || tag === 'SELECT' || (el as HTMLElement).isContentEditable) return true
  if (tag === 'INPUT') return !['button', 'checkbox', 'radio', 'submit', 'reset', 'range'].includes((el as HTMLInputElement).type)
  return false
}

function visible(el: HTMLElement, r: DOMRect): boolean {
  if (r.width < 2 || r.height < 2) return false
  if (el.closest('[aria-hidden="true"], [inert]')) return false
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

function onKey(e: KeyboardEvent) {
  if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return
  const target = e.target as Element | null
  const dir = DIRS[e.key]
  if (dir) {
    if (typing(target) || target?.closest('[role=slider], input[type=range]')) return
    // In the player, arrows drive DVD menus and seeking unless a control has focus.
    if (target?.closest(FULL_PLAYER) && !target.closest(PLAYER_CONTROLS) && !target.closest('[role=dialog]')) return
    if (target?.closest('.picker-grid')) return // the tile picker has its own grid navigation
    if (move(dir)) e.preventDefault()
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
    const axes = p.mapping === 'standard' ? [[0, 1]] : [[0, 1], [p.axes.length - 2, p.axes.length - 1]]
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
  window.addEventListener('gamepadconnected', startPolling)
  // Mouse use hides the controller focus styling again.
  window.addEventListener('mousedown', () => document.documentElement.classList.remove('using-controller'), { passive: true })
  if (navigator.getGamepads && Array.from(navigator.getGamepads()).some(Boolean)) startPolling()
}
