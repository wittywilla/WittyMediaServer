import { useEffect, useState } from 'react'
import { api, mediaUrlQ } from '../api'
import './brandlogo.css'

// The logo service, for every screen (docs/METADATA.md#the-logo-service): a studio,
// network, console, game studio or streaming service, as its logo if this server has
// one and otherwise as its name. Logos only come from this server, and only exist
// once the owner has agreed to the download or uploaded one.
//
//   <BrandLogo kind="network" name="Cartoon Network" />
//
// Lookups made in the same moment are sent as one request, so a row of cards costs one.

export type LogoKind = 'studio' | 'network' | 'console' | 'gamestudio' | 'platform'
export type LogoInfo = {
  id?: number; kind: LogoKind; name: string; status: 'pending' | 'found' | 'none' | 'text'; has: boolean; version: number; opaque: boolean
  source?: string; sourceUrl?: string; license?: string; licenseUrl?: string; credit?: string
}

const known = new Map<string, Promise<LogoInfo>>()
let waiting: { key: string; kind: LogoKind; name: string; done: (l: LogoInfo) => void }[] = []
let timer = 0

const keyOf = (kind: string, name: string) => kind + '|' + name.trim().toLowerCase()

function send() {
  timer = 0
  const batch = waiting.splice(0, 200)
  if (waiting.length) timer = window.setTimeout(send, 0)
  api.post<{ logos: LogoInfo[] }>('/logos/lookup', { logos: batch.map((b) => ({ kind: b.kind, name: b.name })) })
    .then((r) => {
      const byKey = new Map(r.logos.map((l) => [keyOf(l.kind, l.name), l]))
      for (const b of batch) b.done(byKey.get(b.key) || { kind: b.kind, name: b.name, status: 'none', has: false, version: 0, opaque: false })
    })
    .catch(() => {
      for (const b of batch) {
        known.delete(b.key) // try again on the next screen
        b.done({ kind: b.kind, name: b.name, status: 'none', has: false, version: 0, opaque: false })
      }
    })
}

// lookupLogo: what's known about a name. Answers are kept until forgetLogos is called.
export function lookupLogo(kind: LogoKind, name: string): Promise<LogoInfo> {
  const key = keyOf(kind, name)
  let p = known.get(key)
  if (!p) {
    p = new Promise((done) => {
      waiting.push({ key, kind, name, done })
      if (!timer) timer = window.setTimeout(send, 20)
    })
    known.set(key, p)
  }
  return p
}

// forgetLogos: after changing one, or after agreeing to downloads. Whatever is on the screen
// asks again.
const CHANGED = 'wms-logos-changed'
export function forgetLogos() { known.clear(); window.dispatchEvent(new Event(CHANGED)) }

export function useLogo(kind: LogoKind, name: string): LogoInfo | null {
  const [info, setInfo] = useState<LogoInfo | null>(null)
  useEffect(() => {
    let on = true
    const ask = () => { if (name.trim()) lookupLogo(kind, name).then((l) => { if (on) setInfo(l) }) }
    setInfo(null)
    ask()
    window.addEventListener(CHANGED, ask)
    return () => { on = false; window.removeEventListener(CHANGED, ask) }
  }, [kind, name])
  return info
}

export const logoImageUrl = (l: Pick<LogoInfo, 'id' | 'version'>, variant: 'white' | 'dark' | 'original') =>
  mediaUrlQ(`/api/logos/${l.id}/image`, `variant=${variant}&v=${l.version}`)

// Which variant reads on this theme: the page's background decides, so a theme of
// any colours gets the right one. It's read once, and again when the theme changes.
export function logoVariant(): 'white' | 'dark' {
  const bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim()
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/i.exec(bg)
  if (!m) return 'white'
  const [r, g, b] = m.slice(1, 4).map((x) => parseInt(x, 16))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 140 ? 'dark' : 'white'
}

// on: what the logo sits on, when it isn't the page: 'dark' for a card or a plate that
// stays dark in every theme, such as over artwork.
export default function BrandLogo({ kind, name, on, className, label = true }: { kind: LogoKind; name: string; on?: 'dark' | 'light'; className?: string; label?: boolean }) {
  const info = useLogo(kind, name)
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [info?.id, info?.version])
  const cls = 'brand-logo' + (className ? ' ' + className : '')
  if (!info?.has || failed) return <span className={cls + ' brand-logo-text'}>{name}</span>
  const variant = info.opaque ? 'original' : on === 'dark' ? 'white' : on === 'light' ? 'dark' : logoVariant()
  return <img className={cls + (info.opaque ? ' brand-logo-plate' : '')} src={logoImageUrl(info, variant)} alt={label ? name : ''} loading="lazy" onError={() => setFailed(true)} />
}
