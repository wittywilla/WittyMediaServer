import { useEffect, useState } from 'react'
import type { GameConsole } from '../../api'
import { logoImageUrl, logoVariant, lookupLogo, useLogo } from '../../components/BrandLogo'

// A console's mark: its transparent logo if the owner has agreed to download logos and one
// has come, and otherwise its short name, set large. Words aren't a lesser version: a
// text tile is meant to look finished on its own.

export function ConsoleMark({ console: c, heading }: { console: GameConsole; heading?: boolean }) {
  const logo = useLogo('console', c.name)
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [logo?.id, logo?.version])
  if (logo?.has && !broken) {
    const variant = logo.opaque ? 'original' : logoVariant()
    return (
      <span className={'gm-mark' + (heading ? ' heading' : '') + (logo.opaque ? ' plate' : '')}>
        <img src={logoImageUrl(logo, variant)} alt="" loading="lazy" onError={() => setBroken(true)} />
        {heading ? <span>{c.name}</span> : <span className="sr-only">{c.name}</span>}
      </span>
    )
  }
  return heading ? <span className="gm-mark heading">{c.name}</span> : <span className="gm-mark text">{c.short}</span>
}

// Whether any of these consoles shows a logo, so the page can carry the notice that goes
// with logos.
export function useConsoleLogos(list: GameConsole[]) {
  const [any, setAny] = useState(false)
  const names = list.map((c) => c.name).join('|')
  useEffect(() => {
    let on = true
    if (!names) { setAny(false); return }
    Promise.all(names.split('|').map((n) => lookupLogo('console', n))).then((ls) => { if (on) setAny(ls.some((l) => l.has)) })
    return () => { on = false }
  }, [names])
  return any
}
