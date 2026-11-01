import { useEffect, useState } from 'react'
import { api, mediaUrl } from '../../api'
import BrandLogo, { forgetLogos, lookupLogo, useLogo, type LogoInfo, type LogoKind } from '../../components/BrandLogo'
import { LOGO_KIND_NAMES, LogoConsentHint, LogoPanel } from '../../components/edit/LogoChooser'
import { toast } from '../../state'
import { Button, Dialog } from '../../ui'
import { forgetLineup, type TunerChannel } from './tvApi'

// A tuner or playlist channel's logo, in its row of the channel table. What the channel wears is
// worked out on the server: a person's choice, then what its channel list names, then a match on
// its name. Here a wrong match can be put right, and it stays through a new channel list.

const KINDS: LogoKind[] = ['network', 'studio', 'platform']

function Mark({ c }: { c: TunerChannel }) {
  const found = useLogo(c.brand?.kind || 'network', !c.logoUrl && c.brand ? c.brand.name : '')
  if (c.logoUrl) return <img className="tvs-logo" src={mediaUrl(c.logoUrl)} alt="" loading="lazy" />
  if (c.brand && found?.has) return <span className="tvs-logo"><BrandLogo kind={c.brand.kind} name={c.brand.name} label={false} /></span>
  return <span className="tvs-dim">{c.logoSet === 'none' ? 'None' : 'Its name'}</span>
}

function why(c: TunerChannel): string {
  if (c.logoSet === 'none') return 'You chose no logo, so it shows its name.'
  if (c.logoSet) return `You chose the logo for “${c.brand?.name}”.`
  if (c.logoUrl) return 'This logo is named by the channel list, and fetched by this server so that its site never sees your viewers. The list says nothing about who owns it.'
  if (c.brand) return `Matched by name, as “${c.brand.name}”. Only an exact name takes a logo by itself.`
  return 'Its name is too general to look up by itself, such as a number or a word like “Movies”.'
}

export default function TunerLogo({ channel, onChange }: { channel: TunerChannel; onChange: (c: TunerChannel) => void }) {
  const [open, setOpen] = useState(false)
  const [c, setC] = useState(channel)
  const [row, setRow] = useState<LogoInfo | null>(null)
  const [kind, setKind] = useState<LogoKind>(channel.brand?.kind || 'network')
  const [name, setName] = useState(channel.brand?.name || channel.name)
  const [busy, setBusy] = useState(false)
  useEffect(() => setC(channel), [channel])
  const brandKey = c.brand ? `${c.brand.kind}|${c.brand.name}` : ''
  useEffect(() => {
    setRow(null)
    if (!open || !c.brand) return
    let on = true
    forgetLogos()
    void lookupLogo(c.brand.kind, c.brand.name).then((l) => { if (on) setRow(l) })
    return () => { on = false }
  }, [open, brandKey]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (logo: string) => {
    setBusy(true)
    try {
      const n = await api.put<TunerChannel>(`/tuner-channels/${c.id}`, { logo })
      setC(n); onChange(n); forgetLineup(); forgetLogos()
      if (n.brand) { setKind(n.brand.kind); setName(n.brand.name) }
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return (
    <div className="tvs-logo-cell">
      <Mark c={c} />
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)} aria-label={`Change the logo of ${c.name}`}>Change</Button>
      {open && (
        <Dialog title={`The logo of ${c.name}`} size="wide" className="ed-logo-dialog" onClose={() => setOpen(false)} actions={<Button variant="ghost" onClick={() => setOpen(false)}>Done</Button>}>
          <p role="status">{why(c)}</p>
          <div className="tvs-logo-forms">
            <form className="tvs-toolbar" onSubmit={(e) => { e.preventDefault(); void save(`${kind}|${name.trim()}`) }}>
              <label className="tvs-logo-field">Look it up as
                <input className="input" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />
              </label>
              <label className="tvs-logo-field">Which is a
                <select className="select" value={kind} onChange={(e) => setKind(e.target.value as LogoKind)}>
                  {KINDS.map((k) => <option key={k} value={k}>{LOGO_KIND_NAMES[k]}</option>)}
                </select>
              </label>
              <Button type="submit" disabled={busy || !name.trim()}>Use this name</Button>
            </form>
            <div className="tvs-toolbar">
              {c.logoSet !== 'none' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => void save('none')}>No logo for this channel</Button>}
              {c.logoSet && <Button size="sm" variant="ghost" disabled={busy} onClick={() => void save('')}>Back to automatic</Button>}
            </div>
          </div>
          {row?.id ? <LogoPanel key={row.id} logo={row} onChanged={() => { forgetLogos(); forgetLineup() }} onLeave={() => setOpen(false)} /> : !c.brand && <LogoConsentHint onLeave={() => setOpen(false)} />}
        </Dialog>
      )}
    </div>
  )
}
