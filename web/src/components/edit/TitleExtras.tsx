import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrlQ, type Item } from '../../api'
import type { Target } from '../../metadata'
import BrandLogo from '../BrandLogo'
import { CardRow, ItemCard } from '../common'
import LegalNote from '../LegalNote'
import './titlepage.css'

// What a title's page shows beyond its row: who made it, its other versions, its bonus
// features, and where it's listed if the owner turned that on.

type Attached = {
  extras: { item: Item; link: { extraType?: string } }[]; versions: { item: Item; link: { label?: string } }[]
  studios: string[]; director: string; edition: string[]; tags: string[]; hasBanner: boolean; hasLogo: boolean
}
type TitleListings = { platforms: { key: string; name: string }[]; asOf: number; region: string; attribution: string }

const memo = new Map<string, Promise<Attached>>()
const attached = (target: Target, id: number, version: number) => {
  const k = `${target}${id}.${version}`
  let p = memo.get(k)
  if (!p) {
    if (memo.size > 50) memo.clear()
    p = api.get<Attached>(`/meta/${target}/${id}/attached`)
    memo.set(k, p)
  }
  return p
}

// The title as its logo, if it has one; the words are still the page's heading.
export function TitleLogo({ target, id, name, version }: { target: Target; id: number; name: string; version: number }) {
  const [has, setHas] = useState(false)
  const [broken, setBroken] = useState(false)
  useEffect(() => { let on = true; setBroken(false); attached(target, id, version).then((a) => { if (on) setHas(a.hasLogo) }).catch(() => {}); return () => { on = false } }, [target, id, version])
  if (!has || broken) return <h1>{name}</h1>
  return (
    <h1 className="title-logo" aria-label={name}>
      <img src={mediaUrlQ(`/api/meta/${target}/${id}/art/logo`, `v=${version}`)} alt="" onError={() => setBroken(true)} />
    </h1>
  )
}

const extraName = (t?: string) => (t || 'extra').replace(/-/g, ' ').replace(/^./, (c) => c.toUpperCase())

export default function TitleExtras({ target, id, libraryId }: { target: Target; id: number; libraryId: number }) {
  const [a, setA] = useState<Attached | null>(null)
  const [l, setL] = useState<TitleListings | null>(null)
  useEffect(() => {
    let on = true
    api.get<Attached>(`/meta/${target}/${id}/attached`).then((r) => { if (on) setA(r) }).catch(() => {})
    api.get<TitleListings>(`/meta/${target}/${id}/listings`).then((r) => { if (on) setL(r) }).catch(() => {})
    return () => { on = false }
  }, [target, id])
  if (!a) return null
  const hasBrands = a.studios.length > 0
  return (
    <div className="title-extras">
      {(a.director || a.edition.length > 0 || a.tags.length > 0) && (
        <div className="title-facts">
          {a.director && <span><span className="muted">{target === 'show' ? 'Created by' : 'Directed by'}</span> {a.director}</span>}
          {a.edition.map((e) => <span key={e} className="pill">{e}</span>)}
          {a.tags.map((t) => <Link key={t} className="chip" to={`/library/${libraryId}?tag=${encodeURIComponent(t)}`}>{t}</Link>)}
        </div>
      )}
      {hasBrands && (
        <div className="title-brands" aria-label="Studios">
          {a.studios.map((s) => (
            <Link key={s} to={`/brand/studio/${encodeURIComponent(s)}?library=${libraryId}`} className="title-brand" title={s} aria-label={s}><BrandLogo kind="studio" name={s} label={false} /></Link>
          ))}
        </div>
      )}
      {a.versions.length > 0 && (
        <section aria-label="Other versions" className="title-versions">
          <h2>Other versions</h2>
          <div className="title-version-list">
            {a.versions.map((v) => (
              <Link key={v.item.id} className="btn" to={`/item/${v.item.id}`}>{v.link.label || v.item.title}{v.item.height ? ` · ${v.item.height}p` : ''}</Link>
            ))}
          </div>
        </section>
      )}
      {a.extras.length > 0 && (
        <section aria-label="Bonus features" className="title-bonus">
          <h2>Bonus features</h2>
          <CardRow>
            {a.extras.map((x) => (
              <div key={x.item.id} className="title-bonus-card">
                <ItemCard item={x.item} wide />
                <span className="muted">{extraName(x.link.extraType)}</span>
              </div>
            ))}
          </CardRow>
        </section>
      )}
      {l && l.platforms.length > 0 && (
        <section aria-label="Also listed on" className="title-listed">
          <h2>Also listed on</h2>
          <div className="title-brands">
            {l.platforms.map((p) => <Link key={p.key} className="title-brand" to={`/library/${libraryId}?platform=${encodeURIComponent(p.key)}`} title={p.name} aria-label={p.name}><BrandLogo kind="platform" name={p.name} label={false} /></Link>)}
          </div>
          <p className="muted title-listed-note"><b>Nothing plays from these services.</b> You're looking at your own copy, on your own server. Listed for {l.region}{l.asOf ? `, as of ${new Date(l.asOf * 1000).toLocaleDateString()}` : ''}. {l.attribution}</p>
          <LegalNote kind="listings" />
        </section>
      )}
    </div>
  )
}
