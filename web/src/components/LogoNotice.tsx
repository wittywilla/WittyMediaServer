import { useEffect, useState } from 'react'
import { lookupLogo, type LogoKind } from './BrandLogo'
import LegalNote from './LegalNote'

// The rights notice, for a screen with a few logos that may or may not exist: it's there only when
// one of them is on the screen, and a logo that turns up later brings it with it.
export default function LogoNotice({ logos, className }: { logos: { kind: LogoKind; name: string }[]; className?: string }) {
  const [shown, setShown] = useState(false)
  const key = logos.map((l) => l.kind + '|' + l.name).join('\n')
  useEffect(() => {
    let on = true
    if (!logos.length) { setShown(false); return }
    void Promise.all(logos.map((l) => lookupLogo(l.kind, l.name))).then((all) => { if (on) setShown(all.some((l) => l.has)) })
    return () => { on = false }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps
  return shown ? <LegalNote kind="logos" className={className} /> : null
}
