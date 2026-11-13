// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type GameConsole, type GameCore } from '../../api'
import { LoadError, Spinner } from '../../components/common'
import { IconCheck, IconGamepad, IconInfo } from '../../icons'
import { toast } from '../../state'
import { Dialog } from '../../ui'
import { getConsoles } from '../games/consoles'
import CustomCoresSection, { licenseText } from './GamingCustom'
import RegionOrder from './RegionOrder'
import { PageHead, Section } from './ui'
import LegalNote from '../../components/LegalNote'
import '../games/games-ui.css'
import { CorePlatforms, type CoreAlternative } from '../games/platforms'
import type { CorePickerScope } from '../games/CorePicker'

const CorePicker = lazy(() => import('../games/CorePicker'))

// Settings, Consoles & cores. What most people need is one screen: each console they have
// games for, whether it's ready to play, and the one thing to do if it isn't. Everything
// else is there, a click further in.
//
// A core is someone else's program, so nothing is installed without being asked for, with
// its licence in view first.

type BiosFile = { file: string; from?: string; region?: string; using?: boolean }
type ConsoleRow = GameConsole & { biosFound?: string[]; biosFiles?: BiosFile[]; blocked?: string; totalGames?: number }
type Status = {
  runner: string; encoders: string[]; consoles: ConsoleRow[]; cores: GameCore[]; canDownload: boolean
  coresDir: string; systemDir: string; running: number; unpackedBytes?: number; biosFolder: string; gpu?: { name?: string; error?: string; checking?: boolean }
}

type Standing = 'ready' | 'core' | 'bios' | 'gpu' | 'none'

// The core to suggest for a console: the first on offer that's published for this server and can
// draw here, as the server says (coreSupport), or the first on offer when it doesn't say.
const recommended = (c: GameConsole) => c.cores.find((x) => {
  const h = c.coreSupport?.[x.id]?.here
  return !h || (h.build && h.draws !== 'none')
}) || c.cores[0]

const regionWords: Record<string, string> = { usa: 'USA', europe: 'Europe', japan: 'Japan', asia: 'Asia', china: 'China', taiwan: 'Taiwan', russia: 'Russia' }

function standingOf(c: ConsoleRow, gpuOk: boolean): Standing {
  if (c.unavailable) return 'none'
  if (c.reason === 'gpu') return 'gpu'
  if (c.blocked) return 'bios'
  if (!c.ready) return 'core'
  if (c.needsGpu && !gpuOk && !c.software) return 'gpu'
  return 'ready'
}

const STANDING_WORDS: Record<Standing, string> = { ready: 'Ready to play', core: 'Needs a core', bios: 'Needs a BIOS file', gpu: 'Needs a graphics card', none: 'Not playable yet' }

function StandingPill({ s }: { s: Standing }) {
  return <span className={'gm-pill ' + (s === 'ready' ? 'ok' : s === 'none' ? 'off' : 'warn')}>{s === 'ready' ? <IconCheck size={14} /> : <IconInfo size={14} />} {STANDING_WORDS[s]}</span>
}

// only: one console's card alone (its core, its other cores and its BIOS files), as WittyPlay's console page shows it.
export default function GamingSettings({ only }: { only?: string } = {}) {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState('')
  const [confirm, setConfirm] = useState<{ id: string; name: string; license: string }[] | null>(null)
  const [picker, setPicker] = useState<CorePickerScope | null>(null)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const [failed, setFailed] = useState(false)
  const load = () => api.get<Status>('/gaming').then(setSt).catch(() => setFailed(true))
  useEffect(() => { void load(); api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {}) }, [])
  const emptyUnpacked = async () => {
    setBusy('unpacked')
    try {
      const r = await api.del<{ unpackedBytes: number }>('/gaming/unpacked')
      setSt((cur) => (cur ? { ...cur, unpackedBytes: r.unpackedBytes } : cur))
      toast(r.unpackedBytes ? 'Emptied, apart from the games being played' : 'Emptied. Games unpack again when next played')
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  // The graphics card is checked in the background, and the answer comes in by itself.
  const checking = !!st?.gpu?.checking
  useEffect(() => {
    if (!checking) return
    const t = setTimeout(load, 2000)
    return () => clearTimeout(t)
  }, [checking, st])
  const refresh = async () => { await load(); await getConsoles(true) }
  const install = async () => {
    if (!confirm) return
    const list = confirm
    setConfirm(null)
    const instead: { id: string; name: string; license: string }[] = []
    for (const c of list) {
      setBusy(c.id)
      try {
        await api.post(`/gaming/cores/${c.id}/install`)
        toast(`${c.name} installed`)
      } catch (e: any) {
        // Not every core is built for every system (Beetle Supafaust has no build for a Mac with
        // Apple silicon): offer the console's next one, with its licence in view as always.
        const alt = e?.data?.alternative as CoreAlternative | undefined
        const next = alt ? st?.consoles.flatMap((x) => x.cores).find((x) => x.id === alt.id) : /404/.test(String(e.message)) ? nextCore(c.id) : undefined
        if (next) {
          toast(`${c.name} isn’t built for this server. ${next.name} is the next choice.`)
          if (!instead.some((x) => x.id === next.id)) instead.push(next)
        } else toast(e.message)
      }
    }
    setBusy('')
    await refresh()
    if (instead.length) setConfirm(instead)
  }
  // The core after this one on a console it's the first choice for, that this server offers.
  const nextCore = (id: string) => {
    for (const c of st?.consoles || []) {
      const i = c.cores.findIndex((x) => x.id === id)
      if (i !== 0) continue
      const n = c.cores.slice(1).find((x) => offeredIds.has(x.id) && !installed.has(x.id))
      if (n) return n
    }
    return undefined
  }
  const remove = async (id: string, name: string) => {
    setBusy(id)
    try { await api.del(`/gaming/cores/${id}`); toast(`${name} removed`); await refresh() } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const put = async (patch: Record<string, string>) => {
    try { setSettings(await api.put<Record<string, string>>('/settings', patch)); toast('Saved') } catch (e: any) { toast(e.message) }
  }
  const installed = useMemo(() => new Map((st?.cores || []).map((c) => [c.id, c])), [st])
  const offeredIds = useMemo(() => new Set((st?.consoles || []).filter((c) => !c.id.startsWith('custom-')).flatMap((c) => c.cores.map((x) => x.id))), [st])
  // Still being checked, or never checked, isn't the same as having none.
  const gpuOk = !st?.gpu?.error || !!st?.gpu?.name
  // The consoles someone actually has games for come first.
  const withGames = useMemo(() => (st?.consoles || []).filter((c) => (c.totalGames || 0) > 0).sort((a, b) => (b.totalGames || 0) - (a.totalGames || 0)), [st])
  const others = useMemo(() => (st?.consoles || []).filter((c) => !(c.totalGames || 0)), [st])
  if (failed && !st) return <LoadError what="This page" onRetry={() => { setFailed(false); void load() }} />
  if (!st) return <Spinner />
  const known = new Set(st.consoles.flatMap((c) => c.cores.map((x) => x.id)))
  const offered = new Set(st.consoles.filter((c) => !c.id.startsWith('custom-')).flatMap((c) => c.cores.map((x) => x.id)))
  const extra = st.cores.filter((c) => !known.has(c.id) && c.source !== 'custom')
  const readyCount = withGames.filter((c) => standingOf(c, gpuOk) === 'ready').length
  const missing = st.canDownload ? withGames.filter((c) => !c.unavailable && !c.ready && c.reason !== 'gpu' && recommended(c) && offered.has(recommended(c)!.id)).map((c) => recommended(c)!) : []
  const uniqueMissing = [...new Map(missing.map((c) => [c.id, c])).values()]

  const card = (c: ConsoleRow) => {
    const s = standingOf(c, gpuOk)
    const first = recommended(c)
    const needed = (c.bios || []).filter((b) => b.required)
    const found = new Set(c.biosFound || [])
    const biosList = c.bios || []
    return (
      <article key={c.id} className={'gm-setup ' + s}>
        <header className="gm-setup-head">
          <div className="gm-setup-name"><h3>{c.name}</h3>{(c.totalGames || 0) > 0 && <small>{(c.totalGames || 0).toLocaleString()} {c.totalGames === 1 ? 'game' : 'games'}</small>}</div>
          <StandingPill s={s} />
        </header>
        {s === 'none' && <p className="muted">{c.unavailable}</p>}
        {s === 'core' && first && (
          <div className="gm-setup-do">
            <p>Install <b>{first.name}</b>, the free program that plays these games. It’s the one we’d pick.</p>
            {st.canDownload && offered.has(first.id)
              ? <button type="button" className="btn primary" disabled={!!busy} onClick={() => setConfirm([first])}>{busy === first.id ? 'Installing…' : `Install ${first.name}`}</button>
              : <p className="muted">There are no prebuilt cores for this platform. Install them with your system’s packages, or place them in the cores folder.</p>}
          </div>
        )}
        {s === 'bios' && (
          <div className="gm-setup-do">
            <p>These games need your own copy of a BIOS file, such as <code>{c.blocked}</code>. Put it in a folder named <code>{st.biosFolder}</code> beside your {c.short} games, and it’s found the next time you press Play.</p>
          </div>
        )}
        {s === 'gpu' && <p className="muted">Its games are drawn with a graphics card, and this server doesn’t have one it can use{c.gpuMissing ? `: ${c.gpuMissing}` : ''}. Its games are listed, and can’t be started here.</p>}
        {s === 'ready' && c.needsGpu && !c.software && <p className="muted">Uses this server’s graphics card.</p>}
        {s === 'ready' && c.software && <p className="muted">This server has no graphics card games can use, so its processor draws these games. They can run slower than on a server with one, and keep several processor cores busy.</p>}
        {s !== 'none' && (c.cores.length > 0 || (c.customCores?.length ?? 0) > 0) && (() => {
          // What plays these games now: the default chosen here, or the first core installed.
          const now = (c.defaultCore && c.installed.find((x) => x.id === c.defaultCore)) || c.installed[0]
          return (
            <div className="gm-setup-core">
              <span><small className="muted">{c.defaultCore ? 'Plays with' : 'Plays with, automatically'}</small><b>{now ? now.name : 'No core yet'}</b>
                {now && c.coreAbout?.[now.id] && <small className="muted">{c.coreAbout[now.id].about}</small>}</span>
              <button type="button" className="btn sm" disabled={!!busy} onClick={() => setPicker({ kind: 'console', consoleId: c.id, consoleShort: c.short })}>{now ? 'Change core' : 'Choose a core'}</button>
            </div>
          )
        })()}
        {(c.cores.length > 1 || biosList.length > 0 || (c.customCores?.length ?? 0) > 0 || (s === 'ready' && c.cores.length > 0)) && (
          <details className="gm-setup-more">
            <summary>Cores and BIOS files</summary>
            <ul className="gm-setup-cores">
              {c.cores.map((core) => {
                const have = installed.get(core.id)
                return (
                  <li key={core.id}>
                    <span className="gaming-core"><span><b>{core.name}</b>{core.id === recommended(c)?.id && <span className="pill gaming-rec">Recommended</span>}</span><small className="muted">License: {core.license}{have && have.source !== 'installed' && have.source !== 'custom' ? ` · found in ${have.source === 'retroarch' ? 'your RetroArch folder' : 'a system folder'}` : have?.source === 'custom' ? ' · custom' : ''}</small>
                      <CorePlatforms support={c.coreSupport?.[core.id]} /></span>
                    {have
                      ? have.source === 'installed'
                        ? <button type="button" className="btn sm ghost" disabled={!!busy} onClick={() => remove(core.id, core.name)} aria-label={`Remove ${core.name}`}>{busy === core.id ? 'Removing…' : 'Remove'}</button>
                        : <span className="muted">Ready</span>
                      : c.coreSupport?.[core.id] && !c.coreSupport[core.id].here.build
                        ? <span className="muted">Not for this server</span>
                        : st.canDownload && offered.has(core.id) && <button type="button" className="btn sm" disabled={!!busy} onClick={() => setConfirm([core])} aria-label={`Install ${core.name}`}>{busy === core.id ? 'Installing…' : 'Install'}</button>}
                  </li>
                )
              })}
              {c.customCores?.filter((x) => !c.cores.some((y) => y.id === x.id)).map((x) => (
                <li key={x.id}>
                  <span className="gaming-core"><span><b>{x.name}</b><span className="pill gaming-rec">Custom</span></span><small className="muted">{licenseText(x)}. Third-party, unverified.</small></span>
                  <span className="muted">Ready</span>
                </li>
              ))}
            </ul>
            {biosList.length > 0 && (
              <>
                <p className="muted gm-setup-note">{needed.length ? 'BIOS files this console needs, which you supply:' : 'BIOS files some games use, which you supply:'}</p>
                <ul className="gm-checks">
                  {biosList.map((b) => (
                    <li key={b.file} className={found.has(b.file) ? 'ok' : b.required ? 'no' : 'info'}>
                      {found.has(b.file) ? <IconCheck size={16} /> : <IconInfo size={16} />}
                      <span><code>{b.file}</code><small>{found.has(b.file) ? 'Found. ' : 'Not found yet. '}{b.for}</small></span>
                    </li>
                  ))}
                </ul>
                {(c.biosFiles?.length ?? 0) > 0 && (
                  <>
                    <p className="muted gm-setup-note">In this server’s BIOS folder:</p>
                    <ul className="gm-checks">
                      {c.biosFiles!.map((f) => (
                        <li key={f.file} className="ok">
                          <IconCheck size={16} />
                          <span><code>{f.file.split('/').pop()}</code>
                            <small>{[f.using ? 'In use' : '', f.region ? regionWords[f.region] || f.region : '', f.from ? `Found by its checksum in ${f.from}` : 'Put there by hand, or found by its name'].filter(Boolean).join('. ')}.</small></span>
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </>
            )}
          </details>
        )}
      </article>
    )
  }

  return (
    <>
      {only ? (() => { const row = st.consoles.find((c) => c.id === only); return row ? <div className="gm-setups">{card(row)}</div> : <p className="muted">This server doesn’t know this console.</p> })() : <>
      <PageHead title="Consoles & cores" desc="Games run on this server and stream to whatever you’re playing on. To play a console’s games, the server needs a core for it: a small free program that plays them. Some consoles also need a BIOS file of your own." />
      <Section title="Your consoles" desc={withGames.length ? `${readyCount} of ${withGames.length} ${withGames.length === 1 ? 'console' : 'consoles'} with games ${readyCount === 1 ? 'is' : 'are'} ready to play.` : 'When a library has games in it, its consoles show up here first.'}
        actions={uniqueMissing.length > 1 ? <button type="button" className="btn primary" disabled={!!busy} onClick={() => setConfirm(uniqueMissing)}>{busy ? 'Installing…' : `Install all ${uniqueMissing.length} recommended cores`}</button> : undefined}>
        {withGames.length > 0
          ? <div className="gm-setups">{withGames.map(card)}</div>
          : <p className="muted"><IconGamepad size={16} /> Add a games library in <Link to="/settings/libraries">Libraries</Link>, and this page will show what it needs.</p>}
        <details className="gm-adv">
          <summary>Every other console ({others.length})</summary>
          <div className="gm-setups">{others.map(card)}</div>
        </details>
      </Section>

      <Section title="BIOS files" desc="Some consoles need a file copied from a console you own. They’re never downloaded for you.">
        <ol className="gm-bios-steps">
          <li>Make a folder named <code>{st.biosFolder}</code> inside a console’s games folder, such as <code>PS2/{st.biosFolder}</code>. Folders inside it are fine, and it’s never listed as a game.</li>
          <li>Put your BIOS files in it. Each console above lists the files it looks for.</li>
          <li>That’s all. The server recognises each file by its checksum, whatever it’s called, after a library scan or when you press Play, copies what a core needs to its own BIOS folder, and leaves yours alone. A file with the right name and the wrong contents is never used.</li>
        </ol>
        <p className="muted gm-long-path">You can also put files straight into <code>{st.systemDir}</code>. A PlayStation 2 BIOS goes in <code>pcsx2/bios</code> inside it.</p>
      </Section>

      {settings && <RegionOrder saved={settings.gamingRegionOrder} onSave={(order) => put({ gamingRegionOrder: order })} />}

      {settings && (
        <Section title="How games play" desc="The defaults suit most homes. Someone’s own picture, sound and controls are in My game settings, and a game’s own page can change anything for just that game.">
          <div className="set-form gaming-form">
            <label>Picture quality
              <select className="select" value={settings.gamingQuality || ''} onChange={(e) => put({ gamingQuality: e.target.value })}>
                <option value="">Automatic</option>
                <option value="low">Data saver</option>
                <option value="high">High</option>
              </select>
            </label>
            <label>Games at once
              <input className="input" type="number" min={1} max={16} defaultValue={settings.gamingMaxSessions || '2'} onBlur={(e) => put({ gamingMaxSessions: e.target.value })} />
              <small className="muted">Each game takes one CPU core or more. Two is a safe start.</small>
            </label>
          </div>
          <p><Link to="/settings/games">Open My game settings</Link>. To set a picture or shortcuts as the starting point for everyone, choose <b>Everyone</b> at the top of a panel there.</p>
          <details className="gm-adv">
            <summary>Advanced</summary>
            <div className="set-form gaming-form">
              <label>Games on the graphics card at once
                <input className="input" type="number" min={1} max={16} defaultValue={settings.gamingMaxGpuSessions || '2'} onBlur={(e) => put({ gamingMaxGpuSessions: e.target.value })} />
                <small className="muted">For consoles that draw with a graphics card. More games share it, so each one has less.</small>
              </label>
              <label>Internal resolution
                <select className="select" value={settings.gamingResolution || ''} onChange={(e) => put({ gamingResolution: e.target.value })}>
                  <option value="">Automatic (twice native on most consoles)</option>
                  <option value="1">Native: fastest, works on any graphics card</option>
                  <option value="2">2×</option>
                  <option value="3">3×</option>
                  <option value="4">4×: sharpest, needs a fast card</option>
                </select>
                <small className="muted">How sharply hardware-rendered games are drawn. Applies the next time a game starts.</small>
              </label>
              <label>Fastest fast-forward
                <input className="input" type="number" min={1} max={16} step={0.5} defaultValue={settings.gamingMaxSpeed || ''} placeholder="Automatic" onBlur={(e) => put({ gamingMaxSpeed: e.target.value })} />
                <small className="muted">Times normal speed, for everyone. Automatic is 4×, or 2× on a machine with few CPU threads. 1 turns it off.</small>
              </label>
              <label>Memory for rewind, per game
                <input className="input" type="number" min={0} max={512} defaultValue={settings.gamingRewindMB || ''} placeholder="Automatic" onBlur={(e) => put({ gamingRewindMB: e.target.value })} />
                <small className="muted">In MB. Automatic is 32, or 16 on a machine with few CPU threads. 0 turns rewind off for everyone.</small>
              </label>
              <label>Run-ahead
                <select className="select" value={settings.gamingRunAhead === '0' ? '0' : '1'} onChange={(e) => put({ gamingRunAhead: e.target.value })}>
                  <option value="1">Allowed</option>
                  <option value="0">Not allowed</option>
                </select>
                <small className="muted">Lowers delay, and uses several times the CPU of a normal game.</small>
              </label>
              <label>Game runner
                <select className="select" value={settings.gamingRunner || 'auto'} onChange={(e) => put({ gamingRunner: e.target.value })}>
                  <option value="auto">Automatic</option>
                  <option value="builtin">Built in</option>
                  <option value="retroarch">A separate RetroArch-based runner, when installed</option>
                </select>
              </label>
              <label>Space for unpacked games (GB)
                <input className="input" type="number" min={1} max={100000} defaultValue={settings.gamingUnpackGB || '16'} onBlur={(e) => put({ gamingUnpackGB: e.target.value })} />
                <small className="muted">Games in archives are unpacked here when played, and kept for next time. The longest unplayed go first when it fills up.{st.unpackedBytes ? ` In use now: ${(st.unpackedBytes / 1073741824).toFixed(1)} GB.` : ''}</small>
                {!!st.unpackedBytes && <button type="button" className="btn sm" disabled={busy === 'unpacked'} onClick={emptyUnpacked}>Empty it now</button>}
              </label>
              <label>UDP port for WebRTC
                <input className="input" type="number" min={0} max={65535} defaultValue={settings.gamingUdpPort || ''} placeholder="Any free port" onBlur={(e) => put({ gamingUdpPort: e.target.value })} />
                <small className="muted">Most homes need nothing here. Open a port when playing from outside your network, or from a container.</small>
              </label>
              <label>Public IP addresses
                <input className="input" defaultValue={settings.gamingPublicIp || ''} placeholder="Only when behind port forwarding" onBlur={(e) => put({ gamingPublicIp: e.target.value })} />
              </label>
              <label>STUN servers
                <input className="input" defaultValue={settings.gamingStun || ''} placeholder="stun:stun.example.org:3478" onBlur={(e) => put({ gamingStun: e.target.value })} />
              </label>
            </div>
            <dl className="gaming-facts">
              <dt>Game runner</dt><dd>{st.runner || <><b>Missing from this install</b><br /><span className="muted">Games can’t start without it. It ships with the release downloads; if you built this server yourself, build the runner too (see <Link to="/help/games#cores">the games guide</Link>).</span></>}</dd>
              <dt>Encoders</dt><dd>{st.encoders.join(', ') || 'None found'}</dd>
              <dt>Graphics card</dt><dd>{st.gpu?.name || (checking && !st.gpu?.error ? 'Checking…' : <><b>None this server can use</b><br /><span className="muted">{st.gpu?.error ? `${st.gpu.error}. ` : ''}Consoles that draw with a graphics card won’t start. Older consoles still play.</span></>)}</dd>
              <dt>Games running</dt><dd>{st.running}</dd>
              <dt>Cores folder</dt><dd><code>{st.coresDir}</code></dd>
              <dt>BIOS folder</dt><dd><code>{st.systemDir}</code></dd>
            </dl>
            {extra.length > 0 && (
              <>
                <h3>Other cores found</h3>
                <p className="muted">These aren’t in this server’s list, so nothing plays with them until you add one under Custom cores below, which tests it first.</p>
                <ul className="gaming-extra">
                  {extra.map((c) => <li key={c.id}><b>{c.name}</b> <small className="muted">{licenseText(c)}</small></li>)}
                </ul>
              </>
            )}
          </details>
        </Section>
      )}
      <CustomCoresSection onChanged={refresh} />
      <LegalNote kind="cores" className="set-legal" />
      </>}
      {picker && <Suspense fallback={null}><CorePicker scope={picker} onClose={() => { setPicker(null); void refresh() }} /></Suspense>}
      {confirm && (
        <Dialog title={confirm.length === 1 ? `Install ${confirm[0].name}?` : `Install ${confirm.length} cores?`} onClose={() => setConfirm(null)}
          actions={<><button type="button" className="btn ghost" onClick={() => setConfirm(null)}>Cancel</button><button type="button" className="btn primary" autoFocus onClick={install}>Install</button></>}>
          <p>This downloads {confirm.length === 1 ? 'a program' : 'these programs'} from <b>buildbot.libretro.com</b>, the libretro project’s build server, and runs {confirm.length === 1 ? 'it' : 'them'} on this server in a boxed-in process whenever someone plays.</p>
          <ul className="gm-confirm-list">{confirm.map((c) => <li key={c.id}><b>{c.name}</b>: {c.license}</li>)}</ul>
          <p>{confirm.length === 1 ? 'The core is its authors’ work and stays a separate program from this server.' : 'Each core is its authors’ work and stays a separate program from this server.'}</p>
          <LegalNote kind="cores" />
        </Dialog>
      )}
    </>
  )
}
