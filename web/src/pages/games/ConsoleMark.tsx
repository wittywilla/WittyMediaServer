import { useEffect, useState } from 'react'
import type { GameConsole } from '../../api'
import { lookupLogo } from '../../components/BrandLogo'

export { ConsoleMark } from '../../components/ConsoleMark'

// Whether any of these consoles shows a logo, so the page can carry the notice that goes
// with logos.
export function useConsoleLogos(list: GameConsole[]) {
  const [any, setAny] = useState(false)
  const names = list.map((c) => c.logoName || c.name).join('|')
  useEffect(() => {
    let on = true
    if (!names) { setAny(false); return }
    Promise.all(names.split('|').map((n) => lookupLogo('console', n))).then((ls) => { if (on) setAny(ls.some((l) => l.has)) })
    return () => { on = false }
  }, [names])
  return any
}
