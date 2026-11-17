// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { fitWidth, sized } from '../../api'
import { useArtColour } from '../../audio/art'
import { Placeholder } from '../../components/common'

// The top of an album or playlist page: the cover, and behind it a wash of a colour taken
// from the cover that fades into the page. The colour comes from a 32-pixel copy, read once
// and kept (audio/art.ts), and the wash is a plain gradient: no blur to pay for.
export function ListHeader({ art, title, kind, children, tile }: { art: string; title: string; kind: string; children: ReactNode; tile?: ReactNode }) {
  const rgb = useArtColour(art ? sized(art, 32) : null)
  return (
    <header className="mus-head" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mus-head-cover">{art ? <img src={sized(art, fitWidth(232))} alt="" /> : tile ?? <Placeholder title={title || 'Unknown'} />}</div>
      <div className="mus-head-words">
        <span className="mus-kind">{kind}</span>
        <h1 className={'mus-title' + (title.length > 40 ? ' long' : '')} title={title}>{title}</h1>
        {children}
      </div>
    </header>
  )
}

// The bar under a header with the big play button. It stays at the top of the window once
// the header has scrolled away, with the name beside it.
export function ActionBar({ title, children }: { title: string; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [stuck, setStuck] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    // It sticks under the app's own bar, so it counts as stuck once it reaches that.
    const bar = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--topbar-h')) || 0
    const io = new IntersectionObserver(([e]) => setStuck(e.intersectionRatio < 1 && e.boundingClientRect.top < bar + 8), { threshold: [1], rootMargin: `-${bar + 1}px 0px 0px 0px` })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  return (
    <div ref={ref} className={'mus-actions' + (stuck ? ' stuck' : '')}>
      {children}
      <b className="mus-actions-title" aria-hidden="true">{title}</b>
    </div>
  )
}
