// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Posters that lean toward the pointer, with a soft light moving across them. One
// listener on the page and one light element, handed to whichever poster is under the
// pointer, so a library of thousands costs nothing extra. Only transform and opacity
// change, and the poster's place is read once when the pointer arrives. A remote gets
// the same feeling on focus: the poster settles in from the direction you moved.

const POSTER = '.card:not(.square) .poster'
const CARD = '.card:not(.square)'
const PERSPECTIVE = 'perspective(46rem)'

const motion = window.matchMedia('(prefers-reduced-motion: reduce)')

type Nav = Navigator & { deviceMemory?: number; connection?: { saveData?: boolean } }

// Why this device goes without it, in words for the Settings page. '' means it can have it.
export function tiltBlocked(): string {
  const n = navigator as Nav
  if (motion.matches) return 'This device is set to reduce motion.'
  if (n.connection?.saveData) return 'This device is set to save data.'
  if ((n.deviceMemory !== undefined && n.deviceMemory <= 2) || (n.hardwareConcurrency !== undefined && n.hardwareConcurrency <= 2)) return 'This device is too modest to keep it smooth.'
  return ''
}

let active: HTMLElement | null = null
let rect: DOMRect | null = null
let frame = 0
let px = 0, py = 0
let sheen: HTMLElement | null = null
let maxDeg = 7, scale = 1.05
let lastArrow = ''

function draw() {
  frame = 0
  if (!active || !rect || !sheen) return
  const x = Math.max(-0.5, Math.min(0.5, (px - rect.left) / rect.width - 0.5))
  const y = Math.max(-0.5, Math.min(0.5, (py - rect.top) / rect.height - 0.5))
  active.style.transform = `${PERSPECTIVE} rotateX(${(-y * 2 * maxDeg).toFixed(2)}deg) rotateY(${(x * 2 * maxDeg).toFixed(2)}deg) scale(${scale})`
  sheen.style.transform = `translate(${(x * 70).toFixed(1)}%, ${(y * 70).toFixed(1)}%)`
}

function leave() {
  if (frame) { cancelAnimationFrame(frame); frame = 0 }
  if (!active) return
  active.classList.remove('tilting')
  active.style.transform = ''
  sheen?.remove()
  active = null
  rect = null
}

function onMove(e: PointerEvent) {
  if (e.pointerType === 'touch') return
  const poster = (e.target as Element | null)?.closest?.<HTMLElement>(POSTER) || null
  if (poster !== active) {
    leave()
    if (poster) {
      active = poster
      rect = poster.getBoundingClientRect()
      poster.classList.add('tilting')
      sheen ||= Object.assign(document.createElement('div'), { className: 'tilt-sheen' })
      sheen.style.transform = ''
      poster.appendChild(sheen)
    }
  }
  if (active) {
    px = e.clientX; py = e.clientY
    frame ||= requestAnimationFrame(draw)
  }
}

function onKey(e: KeyboardEvent) { if (e.key.startsWith('Arrow')) lastArrow = e.key }

function onFocus(e: FocusEvent) {
  const el = e.target as HTMLElement | null
  if (!el?.matches?.(`${CARD}, ${CARD} *`) || !el.matches(':focus-visible')) return
  const poster = el.closest(CARD)?.querySelector<HTMLElement>('.poster')
  if (!poster || poster === active) return
  const lean = lastArrow === 'ArrowLeft' ? 'rotateY(-5deg)' : lastArrow === 'ArrowRight' ? 'rotateY(5deg)' : lastArrow === 'ArrowUp' ? 'rotateX(5deg)' : lastArrow === 'ArrowDown' ? 'rotateX(-5deg)' : 'rotateX(2deg)'
  const rest = getComputedStyle(document.documentElement).getPropertyValue('--focus-scale').trim() || '1'
  poster.animate([{ transform: `${PERSPECTIVE} ${lean} scale(${scale})` }, { transform: `${PERSPECTIVE} rotateX(0deg) rotateY(0deg) scale(${rest})` }], { duration: 280, easing: 'cubic-bezier(.2, .8, .2, 1)' })
}

let installed = false

function setInstalled(on: boolean) {
  if (on === installed) return
  installed = on
  document.documentElement.classList.toggle('tilt-on', on)
  const opts = { passive: true }
  if (on) {
    const css = getComputedStyle(document.documentElement)
    maxDeg = parseFloat(css.getPropertyValue('--tilt-max')) || maxDeg
    scale = parseFloat(css.getPropertyValue('--tilt-scale')) || scale
    document.addEventListener('pointermove', onMove, opts)
    document.documentElement.addEventListener('pointerleave', leave, opts)
    window.addEventListener('scroll', leave, { passive: true, capture: true })
    window.addEventListener('keydown', onKey, opts)
    window.addEventListener('focusin', onFocus, opts)
  } else {
    leave()
    document.removeEventListener('pointermove', onMove)
    document.documentElement.removeEventListener('pointerleave', leave)
    window.removeEventListener('scroll', leave, { capture: true })
    window.removeEventListener('keydown', onKey)
    window.removeEventListener('focusin', onFocus)
  }
}

let wanted = true

// syncTilt: the person's choice; the device has its say too.
export function syncTilt(on: boolean) {
  wanted = on
  setInstalled(wanted && !tiltBlocked())
}

motion.addEventListener('change', () => syncTilt(wanted))
