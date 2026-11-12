// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { HelpLink } from '../components/HelpLink'
import { api, type GameConsole, type GameDetailsInfo, type GameImport, type GameReview, type GameReviewItem, type GameVersions, type Item } from '../api'
import { Button, Dialog, EmptyState, Notice, Progress, Spinner, Tabs, toast } from '../ui'
import { isBackKey } from '../ui/spatial'
import SpecPage from '../spec/SpecPage'
import { fixGameSpec, versionLinksSpec } from '../spec/specs/fixgame'
import { reviewSpec } from '../spec/specs/gameReview'
import './gamereview.css'

// Putting a games library right. A scan lists what it can prove and asks about the rest:
// see docs/GAMING.md. Nothing here touches a file, only what the server believes about it,
// and everything can be taken back. Administrators only, as the routes behind it are.

type Tab = 'needs' | 'blocked' | 'left' | 'ignored'

const leftCount = (c: Record<string, number>) => Object.entries(c).reduce((n, [k, v]) => (k === 'needs' || k === 'blocked' || k === 'ignored' ? n : n + v), 0)

// How an import is going: what a scan is doing, then what's being looked up, in one line
// that stays out of the way. The numbers come from the server, and carry on after a
// restart or from another device. It asks often while something's moving, and rarely
// when not, as a lookup can be waiting on a source for hours.
export function GameImportBar({ libraryId, onChanged }: { libraryId: number; onChanged: () => void }) {
  const [st, setSt] = useState<GameImport | null>(null)
  const [said, setSaid] = useState('')
  const stage = useRef('')
  const changed = useRef(onChanged)
  changed.current = onChanged
  useEffect(() => {
    let stop = false
    let timer = 0
    let was = ''
    const tick = async () => {
      const r = await api.get<GameImport>(`/libraries/${libraryId}/games/import`).catch(() => null)
      if (stop || !r) return
      setSt(r)
      // Only what changes which files are listed: how a lookup is going is shown here, and
      // the page shouldn't reload for each picture.
      const mark = `${r.scanning}|${JSON.stringify(r.review)}`
      if (was && mark !== was) changed.current()
      was = mark
      const moving = r.scanning || r.lookup.running
      if (moving || r.lookup.pending + r.lookup.pictures > 0) timer = window.setTimeout(tick, moving ? 3000 : 30000)
    }
    void tick()
    return () => { stop = true; clearTimeout(timer) }
  }, [libraryId])
  // A screen reader hears when the stage changes. Numbers that change every few seconds
  // would never let it say anything else.
  const stageNow = !st ? '' : st.scanning ? 'scan' : st.lookup.pending > 0 ? 'details' : st.lookup.pictures > 0 ? 'pictures' : ''
  useEffect(() => {
    if (stageNow === stage.current) return
    const was = stage.current
    stage.current = stageNow
    if (stageNow === 'scan') setSaid('Going through the games folders.')
    else if (stageNow === 'details') setSaid('Finding out about the games.')
    else if (stageNow === 'pictures') setSaid('Fetching pictures of the games.')
    else if (was) setSaid('The games have finished importing.')
  }, [stageNow])
  const live = <p className="sr-only" role="status">{said}</p>
  if (!st) return live
  const { scan, lookup } = st
  let line = ''
  let value: number | undefined
  if (st.scanning && scan) {
    const total = scan.found
    line = scan.phase === 'indexing' ? `Adding games: ${scan.done.toLocaleString()} of ${total.toLocaleString()}` : total ? `Checking files: ${scan.done.toLocaleString()} of ${total.toLocaleString()}` : 'Looking through the folders…'
    value = total ? scan.done / total : undefined
  } else if (lookup.pending > 0) {
    const asked = lookup.total - lookup.pending
    line = lookup.matched > asked
      ? `${lookup.matched.toLocaleString()} of ${lookup.total.toLocaleString()} games recognised. Asking online sources for more about them: ${asked.toLocaleString()} done`
      : `Finding details: ${asked.toLocaleString()} of ${lookup.total.toLocaleString()} games`
    value = lookup.total ? asked / lookup.total : undefined
  } else if (lookup.pictures > 0) {
    line = `Fetching pictures: ${lookup.pictures.toLocaleString()} ${lookup.pictures === 1 ? 'game' : 'games'} to go`
  }
  if (!line) return live
  return (
    <div className="gr-import">
      {live}
      <span className="gr-import-line">{line}</span>
      {value !== undefined && <Progress value={value} max={1} label="How far the import has got" />}
      <span className="gr-import-note">It carries on by itself in the background, and picks up where it left off after a restart.</span>
    </div>
  )
}

// The way in, on a library's page. It says nothing when there's nothing to say.
export function GameReviewBanner({ libraryId, consoles, version, onChanged }: { libraryId: number; consoles: GameConsole[]; version: number; onChanged: () => void }) {
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [open, setOpen] = useState(false)
  const load = () => api.get<GameReview>(`/libraries/${libraryId}/games/review?kind=needs&limit=1`).then((r) => setCounts(r.counts)).catch(() => {})
  useEffect(() => { load() }, [libraryId, version]) // eslint-disable-line react-hooks/exhaustive-deps
  const needs = counts.needs || 0
  const blocked = counts.blocked || 0
  const left = leftCount(counts) + (counts.ignored || 0)
  // While the list is open, even when the last of it has just been dealt with: it says so.
  if (needs + blocked + left === 0 && !open) return null
  return (
    <>
      <div className={'gr-banner' + (needs + blocked > 0 ? ' attention' : '')} hidden={needs + blocked + left === 0}>
        <span>
          {needs > 0 ? <><strong>{needs.toLocaleString()} {needs === 1 ? 'file needs' : 'files need'} a look.</strong> They might be games, but nothing proves it.</>
            : blocked > 0 ? <><strong>{blocked.toLocaleString()} {blocked === 1 ? 'game' : 'games'} can’t play as {blocked === 1 ? 'it is' : 'they are'}.</strong></>
              : <>{left.toLocaleString()} {left === 1 ? 'file' : 'files'} in these folders {left === 1 ? 'isn’t a game' : 'aren’t games'}, and {left === 1 ? 'was' : 'were'} left out.</>}
        </span>
        <Button size="sm" variant={needs > 0 ? 'primary' : 'ghost'} onClick={() => setOpen(true)}>{needs + blocked > 0 ? 'Take a look' : 'See what'}</Button>
      </div>
      {open && <GameReviewDialog libraryId={libraryId} consoles={consoles} onClose={() => { setOpen(false); load() }} onChanged={() => { load(); onChanged() }} first={needs > 0 ? 'needs' : blocked > 0 ? 'blocked' : 'left'} />}
    </>
  )
}

const TabCount = ({ n }: { n?: number }) => (n ? <> <span className="gr-count">{n.toLocaleString()}</span></> : null)

function GameReviewDialog({ libraryId, consoles, first, onClose, onChanged }: { libraryId: number; consoles: GameConsole[]; first: Tab; onClose: () => void; onChanged: () => void }) {
  const [tab, setTab] = useState<Tab>(first)
  const [data, setData] = useState<GameReview | null>(null)
  const [busy, setBusy] = useState(false)
  const [said, setSaid] = useState('')
  const panel = useId()
  const box = useRef<HTMLDivElement>(null)
  // After something has been dealt with, which row focus goes to: what was pressed has gone.
  const [focusRow, setFocusRow] = useState<{ at: number } | null>(null)
  useEffect(() => {
    if (!focusRow || busy) return
    const rows = box.current?.querySelectorAll<HTMLElement>('.spec-row')
    const next = rows?.[Math.min(focusRow.at, rows.length - 1)]?.querySelector<HTMLElement>('button, a') || rows?.[Math.min(focusRow.at, rows.length - 1)]
    ;(next || box.current?.querySelector<HTMLElement>('[role=tab][aria-selected=true]'))?.focus()
    setFocusRow(null)
  }, [focusRow, busy])
  const load = () => api.get<GameReview>(`/libraries/${libraryId}/games/review?kind=${tab}&limit=500`).then(setData).catch((e) => toast(e.message))
  useEffect(() => { setData(null); load() }, [tab, libraryId]) // eslint-disable-line react-hooks/exhaustive-deps
  const act = async (ids: number[], action: 'add' | 'ignore' | 'forget', console_?: string, words?: string) => {
    setBusy(true)
    try {
      await api.post(`/libraries/${libraryId}/games/review/apply`, { ids, action, console: console_ })
      setSaid(words || 'Done. The library is being scanned again.')
      setFocusRow({ at: Math.max(0, (data?.items || []).findIndex((i) => ids.includes(i.id))) })
      setData((d) => d && { ...d, items: d.items.filter((i) => !ids.includes(i.id)), total: d.total - ids.length, counts: { ...d.counts, [tab]: Math.max(0, (d.counts[tab] || 0) - ids.length) } })
      onChanged()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const counts = data?.counts || {}
  const tabs = [
    { value: 'needs' as Tab, label: 'Needs a look', badge: <TabCount n={counts.needs} /> },
    { value: 'blocked' as Tab, label: 'Can’t play', badge: <TabCount n={counts.blocked} /> },
    { value: 'left' as Tab, label: 'Left out', badge: <TabCount n={leftCount(counts)} /> },
    { value: 'ignored' as Tab, label: 'You said no', badge: <TabCount n={counts.ignored} /> },
  ]
  return (
    <Dialog title="Files that didn’t become games" size="wide" onClose={onClose} className="gr-dialog"
      description="Files are never moved or changed. These choices are kept beside the library, and every later scan goes by them.">
      <div ref={box}>
        <Tabs value={tab} onChange={setTab} items={tabs} label="Which files" panelId={panel} />
        <p className="gr-live" role="status">{said}</p>
        <p className="muted"><HelpLink to="games#needs-a-look">How games are recognised, and what to do here</HelpLink></p>
        <div id={panel} role="tabpanel" aria-label={tabs.find((t) => t.value === tab)?.label}>
          {data === null ? <Spinner /> : data.items.length === 0 ? <EmptyState title={tab === 'needs' ? 'Nothing needs a look' : 'Nothing here'} /> : (
            <>
              {data.total > data.items.length && <p className="muted">Showing the first {data.items.length.toLocaleString()} of {data.total.toLocaleString()}.</p>}
              <ReviewList tab={tab} items={data.items} consoles={consoles} busy={busy} onAct={act} />
            </>
          )}
        </div>
      </div>
    </Dialog>
  )
}

// The list of files with their choices (reviewSpec); which row has its console choice open is this list's own.
function ReviewList({ tab, items, consoles, busy, onAct }: { tab: Tab; items: GameReviewItem[]; consoles: GameConsole[]; busy: boolean; onAct: (ids: number[], action: 'add' | 'ignore' | 'forget', console?: string, words?: string) => void }) {
  const [choosing, setChoosing] = useState<number | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const page = useMemo(() => reviewSpec(tab, items, { busy, consoles: consoles.map((c) => ({ id: c.id, name: c.name })), choosing }), [tab, items, busy, consoles, choosing])
  const ids = new Set<string>()
  for (const sec of page.sections) if (sec.kind !== 'note' && sec.kind !== 'fields') for (const it of sec.items) { if (it.id) ids.add(it.id); if ('actions' in it) for (const a of it.actions || []) if (a.id) ids.add(a.id) }
  // Opening the console choice puts focus on its first button; Back or Escape closes it.
  useEffect(() => { if (choosing != null) box.current?.querySelector<HTMLElement>('.spec-chips:last-of-type .spec-chip')?.focus() }, [choosing])
  const act = (id: string) => {
    const [verb, a, b] = id.split('.')
    const it = items.find((x) => String(x.id) === a)
    if (verb === 'bulk') { const g = items.filter((x) => x.suggest[0]?.console === a); onAct(g.map((x) => x.id), 'add', a, `Added ${g.length} files as ${g[0]?.suggest[0]?.short} games.`) }
    else if (verb === 'closepick') setChoosing(null)
    else if (!it) return
    else if (verb === 'add' && it.suggest[0]) onAct([it.id], 'add', it.suggest[0].console, `Added ${it.name} as a ${it.suggest[0].short} game.`)
    else if (verb === 'other') setChoosing(choosing === it.id ? null : it.id)
    else if (verb === 'ignore') onAct([it.id], 'ignore', undefined, `${it.name} won’t be listed.`)
    else if (verb === 'forget') onAct([it.id], 'forget', undefined, 'Taken back. The library is being scanned again.')
    else if (verb === 'pick') { const c = consoles.find((x) => x.id === b); setChoosing(null); if (c) onAct([it.id], 'add', c.id, `Added ${it.name} as a ${c.name} game.`) }
  }
  return (
    <div ref={box} onKeyDown={(e) => { if (choosing != null && (e.key === 'Escape' || isBackKey(e.nativeEvent))) { e.preventDefault(); e.stopPropagation(); setChoosing(null) } }}>
      <SpecPage bare page={page} handled={ids} onAction={act} />
    </div>
  )
}

// Buttons, not a drop-down: a drop-down that acts when its value changes acts on the first
// press of an arrow key, and a TV remote may not be able to open one at all. Back or Escape
// closes this without closing anything else.
function ConsolePicker({ consoles, disabled, onPick, onClose, label }: { consoles: GameConsole[]; disabled: boolean; onPick: (id: string, name: string) => void; onClose: () => void; label: string }) {
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => { box.current?.querySelector('button')?.focus() }, [])
  return (
    <div className="gr-picker" ref={box} role="group" aria-label={label}
      onKeyDown={(e) => { if (e.key === 'Escape' || isBackKey(e.nativeEvent)) { e.preventDefault(); e.stopPropagation(); onClose() } }}>
      {consoles.map((c) => <Button key={c.id} size="sm" variant="ghost" disabled={disabled} onClick={() => onPick(c.id, c.name)}>{c.name}</Button>)}
    </div>
  )
}

const howWords = (d: GameDetailsInfo) => {
  switch (d.detectedBy) {
    case 'bytes': return 'Recognised from the file itself'
    case 'extension': return 'Going by the kind of file it is'
    case 'folder': return 'Going by the folder it’s in'
    case 'chosen': return 'You said what it is'
  }
  return ''
}

// The game's own page: why it can't play, if it can't, and everything about it that a
// person can put right. Which game it is, by name, is GameMatchBar's job.
export function GameFixPanel({ it, d, consoles, onChanged }: { it: Item; d: GameDetailsInfo; consoles: GameConsole[]; onChanged: () => void }) {
  const [v, setV] = useState<GameVersions | null>(null)
  const [choosing, setChoosing] = useState(false)
  const [busy, setBusy] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  // What was pressed may have gone with the change, or gone out of use for a moment.
  const [ran, setRan] = useState(0)
  const held = useRef<HTMLElement | null>(null)
  useEffect(() => {
    if (!ran || busy) return
    if (held.current?.isConnected && !(held.current as HTMLButtonElement).disabled) held.current.focus()
    else heading.current?.focus()
  }, [ran, busy])
  const load = () => api.get<GameVersions>(`/games/${it.id}/versions`).then(setV).catch(() => setV({ versions: [] }))
  useEffect(() => { load() }, [it.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const run = async (fn: () => Promise<unknown>, words: string) => {
    held.current = document.activeElement as HTMLElement | null
    setBusy(true)
    try { await fn(); toast(words); await load(); onChanged() } catch (e: any) { toast(e.message) } finally { setBusy(false); setRan((n) => n + 1) }
  }
  const how = howWords(d)
  const others = (v?.versions || []).filter((x) => !x.current)
  const page = fixGameSpec({
    how, sure: d.detectSure, why: d.detectWhy, discs: v?.discs?.map((x) => x.name), others: others.map((x) => ({ id: x.id as number, label: x.label || '' })),
    inside: v?.inside?.map((x) => ({ entry: x.entry as string, label: x.label, altered: x.altered, current: x.current })), choosing, busy,
  })
  const act = (id: string) => {
    if (id === 'console') setChoosing(!choosing)
    else if (id === 'notgame') void run(() => api.post(`/games/${it.id}/not-a-game`), 'It won’t be listed any more. You can bring it back from the library’s page.')
    else if (id.startsWith('split.')) void run(() => api.post(`/games/${id.slice(6)}/split`), 'It’s a game of its own now.')
    else if (id.startsWith('play.')) void run(() => api.put(`/games/${it.id}/version`, { entry: id.slice(5) }), 'This version plays from now on.')
  }
  const handled = new Set(['console', 'notgame', ...others.map((x) => `split.${x.id}`), ...(v?.inside || []).map((x) => `play.${x.entry}`)])
  return (
    <section className="gr-fix" aria-label="Fix this game">
      <h2 className="shelf-head" ref={heading} tabIndex={-1}>Fix this game</h2>
      <SpecPage bare page={page} handled={handled} onAction={act}
        after={{ [page.sections.length - 1]: choosing ? <ConsolePicker consoles={consoles} disabled={busy} label="Which console this game is for" onClose={() => { setChoosing(false) }} onPick={(c, name) => run(() => api.put(`/games/${it.id}/console`, { console: c }), `It’s a ${name} game now.`)} /> : null }} />
    </section>
  )
}

// The other versions of a game, for anyone who isn't an administrator: each is a game with
// its own saves, so all there is to do is open it.
export function GameVersionLinks({ it }: { it: Item }) {
  const [v, setV] = useState<GameVersions | null>(null)
  useEffect(() => { api.get<GameVersions>(`/games/${it.id}/versions`).then(setV).catch(() => {}) }, [it.id])
  const others = (v?.versions || []).filter((x) => !x.current)
  if (others.length === 0) return null
  return (
    <section className="gr-fix" aria-label="Other versions of this game">
      <h2 className="shelf-head">Other versions of this game</h2>
      <SpecPage bare page={versionLinksSpec(others.map((x) => ({ id: x.id as number, label: x.label || '' })))} />
    </section>
  )
}

// Why a game can't play, before anyone presses Play. It's for everyone, as it's about
// their file and what would help.
export function GameProblem({ d }: { d: GameDetailsInfo }) {
  if (!d.problem) return null
  return <Notice tone="danger" title="This game can’t play as it is">{d.problemText}</Notice>
}
