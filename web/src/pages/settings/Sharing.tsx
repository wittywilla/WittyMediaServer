import { useEffect, useState } from 'react'
import { api } from '../../api'
import { type ShareLink } from '../../gallery'
import { toast, useApp } from '../../state'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'
import LegalNote from '../../components/LegalNote'

// Uploads, links for people without an account, the online map, and the place
// kept for the local AI.

function ServerSharing() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return st.failed ? <LoadError what="This page" onRetry={st.retry} /> : null
  return (
    <>
      <Section title="Uploads" desc="Adults can add pictures and videos to Photos and Vertical videos libraries from their devices. Files are saved in an Uploads folder inside the library's first folder and never replace anything.">
        <Row label="Allow uploads">
          <Switch label="Allow uploads" checked={on('uploads')} onChange={(v) => set('uploads', v ? '1' : '0')} />
        </Row>
        <Row label="Largest upload" desc="For one file." dim={!on('uploads')}>
          <select className="select" value={s.uploadMaxMb || '4096'} disabled={!on('uploads')} onChange={(e) => set('uploadMaxMb', e.target.value)}>
            <option value="512">512 MB</option><option value="2048">2 GB</option><option value="4096">4 GB</option><option value="16384">16 GB</option><option value="65536">64 GB</option>
          </select>
        </Row>
      </Section>
      <Section title="Links for people without an account" desc="Adults can make a link to one picture, video or album. Whoever has the link sees that and nothing else, without signing in. It only works where this server can be reached: on your home network until you set up remote access.">
        <Row label="Allow links" desc="Turning this off stops every existing link too.">
          <Switch label="Allow links" checked={on('shareLinks')} onChange={(v) => set('shareLinks', v ? '1' : '0')} />
        </Row>
      </Section>
      <Section title="Photo map" desc="The map works without the internet, with a plain outline of the world. An online map shows streets and places, but it means the viewer's device asks that provider for the parts of the map being looked at, so the provider learns roughly what area is being viewed and from where. Off by default.">
        <Row label="Online map address" desc={<>Optional. An https address containing {'{z}'}, {'{x}'} and {'{y}'}, from a provider whose terms allow it. Leave empty to stay offline.</>} stack>
          <input className="input" value={s.mapTiles || ''} onChange={(e) => set('mapTiles', e.target.value)} placeholder="https://…/{z}/{x}/{y}.png" spellCheck={false} />
        </Row>
      </Section>
      <Section title="Local AI" desc="Planned, not available yet.">
        <Row label="People and places in photos" desc="Will recognise faces and places on this machine, with nothing sent anywhere. It doesn't exist yet, so this does nothing and nothing looks at your pictures." dim>
          <Switch label="People and places in photos" checked={false} disabled onChange={() => {}} />
        </Row>
      </Section>
      <SaveBar {...st} />
    </>
  )
}

function Links() {
  const { user } = useApp()
  const [links, setLinks] = useState<ShareLink[] | null>(null)
  const [all, setAll] = useState(false)
  const load = () => api.get<ShareLink[]>(`/shares${all ? '?all=1' : ''}`).then(setLinks).catch(() => setLinks([]))
  useEffect(() => { load() }, [all]) // eslint-disable-line react-hooks/exhaustive-deps
  const end = async (l: ShareLink) => {
    if (!confirm(`End the link to “${l.title}”? Anyone who has it will no longer be able to open it.`)) return
    try { await api.del(`/shares/${l.id}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Your links" desc="Links stay valid until their end date, or until you end them here. A link itself can't be shown again: the server only keeps a fingerprint of it."
      actions={user?.isAdmin ? <label className="set-check"><input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} /> Everyone's</label> : undefined}>
      {links && !links.length && <p className="muted" style={{ padding: '4px 0' }}>No links yet. Make one with Share on a picture, video or album.</p>}
      {links?.map((l) => (
        <Row key={l.id} label={l.title} desc={[l.kind === 'album' ? 'Album' : 'Single item', l.owner && all ? `by ${l.owner}` : '', `opened ${l.views} ${l.views === 1 ? 'time' : 'times'}`, l.expiresAt ? `until ${new Date(l.expiresAt * 1000).toLocaleDateString()}` : 'no end date', l.allowDownload ? 'downloads allowed' : ''].filter(Boolean).join(' · ')}>
          <button className="btn sm ghost" onClick={() => end(l)}>End link</button>
        </Row>
      ))}
    </Section>
  )
}

export default function SharingSettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Sharing & photos" desc="Who can add to this server, and what can be seen from outside it." />
      {user?.isAdmin && <ServerSharing />}
      <Links />
      <LegalNote kind="sharing" className="set-legal" />
    </>
  )
}
