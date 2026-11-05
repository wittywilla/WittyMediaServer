// Going to one part of a settings page: from a search result, a table of contents or a link
// with #section in it. The page scrolls to it and the focus moves to its heading, so a screen
// reader reads where you are and a remote carries on from there. It glows for a moment, to
// show the eye the same.

const reduced = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

export function jumpTo(id: string): boolean {
  const el = document.getElementById(id)
  if (!el) return false
  el.scrollIntoView({ block: 'start', behavior: reduced() || document.hidden ? 'auto' : 'smooth' })
  const head = el.matches('h1, h2, h3') ? el : el.querySelector<HTMLElement>('h2, h3') || el
  if (!head.hasAttribute('tabindex') && !head.matches('a[href], button, input, select, textarea')) head.setAttribute('tabindex', '-1')
  head.focus({ preventScroll: true })
  el.classList.remove('set-flash')
  void el.offsetWidth // starts the glow again when it's the same part twice
  el.classList.add('set-flash')
  window.setTimeout(() => el.classList.remove('set-flash'), 1600)
  return true
}

// A page that's still loading has nothing to jump to yet: try for a few seconds. What's above it
// may still be arriving after that and push it down, so it's kept in place until the page
// settles, or the person scrolls or presses something. Returns a way to stop, for when the page
// changes again first.
const MOVES = ['wheel', 'touchstart', 'pointerdown', 'keydown'] as const

export function jumpWhenReady(id: string): () => void {
  let tries = 0, timer = 0, watch = 0
  const stop = () => {
    clearTimeout(timer); clearInterval(watch)
    MOVES.forEach((ev) => removeEventListener(ev, stop))
  }
  const hold = () => {
    let last = -1, steady = 0, ticks = 0
    watch = window.setInterval(() => {
      const el = document.getElementById(id)
      if (!el || ++ticks > 40) { stop(); return }
      const at = Math.round(el.getBoundingClientRect().top + scrollY) // its place in the page, whatever the scroll
      if (at === last) { if (++steady >= 5) stop(); return }
      if (last >= 0) el.scrollIntoView({ block: 'start' })
      last = at; steady = 0
    }, 100)
    MOVES.forEach((ev) => addEventListener(ev, stop, { passive: true }))
  }
  const go = () => {
    if (jumpTo(id)) hold()
    else if (++tries < 40) timer = window.setTimeout(go, 100)
  }
  timer = window.setTimeout(go, 0)
  return stop
}
