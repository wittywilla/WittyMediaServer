import { useEffect, useId, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type GameConsole, type GameDetailsInfo, type GameImport, type GameReview, type GameReviewItem, type GameVersions, type Item } from '../api'
import { IconCheck, IconClose, IconRefresh } from '../icons'
import { Button, Dialog, EmptyState, Notice, Progress, Spinner, Tabs, toast } from '../ui'
import { isBackKey } from '../ui/spatial'
import './gamereview.css'

// Putting a games library right. A scan lists what it can prove and asks about the rest:
// see docs/GAMING.md. Nothing here touches a file, only what the server believes about it,
// and everything can be taken back. Administrators only, as the routes behind it are.

const KIND_WORDS: Record<string, string> = {
  bios: 'BIOS', update: 'Updates', dlc: 'Extra content', utility: 'Utilities', audio: 'Music', data: 'Pieces of a game', other: 'Other files', duplicate: 'Copies', ignored: 'Not games',
}
const kindWord = (k: string) => KIND_WORDS[k] || k

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
    const rows = box.current?.querySelectorAll<HTMLElement>('.gr-row')
    const next = rows?.[Math.min(focusRow.at, rows.length - 1)]?.querySelector<HTMLElement>('button, a')
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
        <div id={panel} role="tabpanel" aria-label={tabs.find((t) => t.value === tab)?.label}>
          {data === null ? <Spinner /> : data.items.length === 0 ? <EmptyState title={tab === 'needs' ? 'Nothing needs a look' : 'Nothing here'} /> : (
            <>
              {data.total > data.items.length && <p className="muted">Showing the first {data.items.length.toLocaleString()} of {data.total.toLocaleString()}.</p>}
              {tab === 'needs' && <BulkBar items={data.items} busy={busy} onAct={act} />}
              <ul className="gr-list">
                {data.items.map((it) => <ReviewRow key={it.id} it={it} tab={tab} consoles={consoles} busy={busy} onAct={act} />)}
              </ul>
            </>
          )}
        </div>
      </div>
    </Dialog>
  )
}

// One click for many: a whole folder that's plainly one console's, but has no headers.
function BulkBar({ items, busy, onAct }: { items: GameReviewItem[]; busy: boolean; onAct: (ids: number[], action: 'add' | 'ignore' | 'forget', console?: string, words?: string) => void }) {
  const groups = new Map<string, { name: string; ids: number[] }>()
  for (const it of items) {
    const s = it.suggest[0]
    if (!s) continue
    const g = groups.get(s.console) || { name: s.short, ids: [] }
    g.ids.push(it.id)
    groups.set(s.console, g)
  }
  const worth = [...groups.entries()].filter(([, g]) => g.ids.length > 1)
  if (worth.length === 0) return null
  return (
    <div className="gr-bulk">
      {worth.map(([id, g]) => <Button key={id} size="sm" disabled={busy} onClick={() => onAct(g.ids, 'add', id, `Added ${g.ids.length} files as ${g.name} games.`)}>Add all {g.ids.length} as {g.name}</Button>)}
    </div>
  )
}

function ReviewRow({ it, tab, consoles, busy, onAct }: { it: GameReviewItem; tab: Tab; consoles: GameConsole[]; busy: boolean; onAct: (ids: number[], action: 'add' | 'ignore' | 'forget', console?: string, words?: string) => void }) {
  const [choosing, setChoosing] = useState(false)
  const toggle = useRef<HTMLButtonElement>(null)
  const first = it.suggest[0]
  return (
    <li className="gr-row">
      <div className="gr-row-text">
        <b className="gr-name">{it.name}</b>
        {it.path !== it.name && <small className="gr-path">{it.path}</small>}
        <span className="gr-reason">{tab === 'left' && <span className="pill">{kindWord(it.kind)}</span>} {it.reason}</span>
      </div>
      <div className="gr-row-actions">
        {tab === 'blocked' && it.itemId ? <Link className="btn sm ghost" to={`/item/${it.itemId}`} aria-label={`Open ${it.name}`}>Open the game</Link> : null}
        {tab === 'ignored' && <Button size="sm" disabled={busy} aria-label={`Take back saying ${it.name} isn’t a game`} onClick={() => onAct([it.id], 'forget', undefined, 'Taken back. The library is being scanned again.')}>Take it back</Button>}
        {(tab === 'needs' || tab === 'left') && it.kind !== 'duplicate' && (
          <>
            {first && <Button size="sm" variant={tab === 'needs' ? 'primary' : 'ghost'} disabled={busy} aria-label={`${it.name} is a ${first.short} game`} onClick={() => onAct([it.id], 'add', first.console, `Added ${it.name} as a ${first.short} game.`)}><IconCheck /> It’s a {first.short} game</Button>}
            <Button ref={toggle} size="sm" variant="ghost" disabled={busy} aria-expanded={choosing} aria-label={`Another console for ${it.name}`} onClick={() => setChoosing(!choosing)}>Another console</Button>
            {tab === 'needs' && <Button size="sm" variant="ghost" disabled={busy} aria-label={`${it.name} is not a game`} onClick={() => onAct([it.id], 'ignore', undefined, `${it.name} won’t be listed.`)}><IconClose /> Not a game</Button>}
          </>
        )}
      </div>
      {choosing && <ConsolePicker consoles={consoles} disabled={busy} onPick={(c, name) => onAct([it.id], 'add', c, `Added ${it.name} as a ${name} game.`)} label={`Console for ${it.name}`} onClose={() => { setChoosing(false); toggle.current?.focus() }} />}
    </li>
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
  const toggle = useRef<HTMLButtonElement>(null)
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
  return (
    <section className="gr-fix" aria-label="Fix this game">
      <h2 className="shelf-head" ref={heading} tabIndex={-1}>Fix this game</h2>
      {how && <p className="gr-how"><strong>{how}{d.detectSure ? `, ${d.detectSure}% sure` : ''}.</strong> {d.detectWhy}</p>}
      {v?.discs && v.discs.length > 1 && (
        <p className="gr-how"><strong>{v.discs.length} discs.</strong> Change discs from the game’s menu while it’s running. <small className="muted">{v.discs.map((x) => x.name).join(', ')}</small></p>
      )}
      {others.length > 0 && (
        <div className="gr-versions">
          <h3>Other versions of this game</h3>
          <ul>
            {others.map((x) => (
              <li key={x.id}>
                <Link to={`/item/${x.id}`}>{x.label || 'Another version'}</Link>
                <Button size="sm" variant="ghost" disabled={busy} aria-label={`Make ${x.label || 'this version'} a game of its own`} onClick={() => run(() => api.post(`/games/${x.id}/split`), 'It’s a game of its own now.')}>Make it a game of its own</Button>
              </li>
            ))}
          </ul>
        </div>
      )}
      {v?.inside && (
        <div className="gr-versions">
          <h3>Which version in this archive to play</h3>
          <ul>
            {v.inside.map((x) => (
              <li key={x.entry} aria-current={x.current ? 'true' : undefined}>
                <span className="gr-version-name">{x.label}{x.altered ? ' (altered)' : ''}</span>
                {x.current
                  ? <span className="pill"><IconCheck /> Plays now</span>
                  : <Button size="sm" variant="ghost" disabled={busy} aria-label={`Play ${x.label} from now on`} onClick={() => run(() => api.put(`/games/${it.id}/version`, { entry: x.entry }), 'This version plays from now on.')}>Play this one</Button>}
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="gr-fix-actions">
        <Button ref={toggle} size="sm" variant="ghost" disabled={busy} aria-expanded={choosing} onClick={() => setChoosing(!choosing)}><IconRefresh /> Wrong console</Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => run(() => api.post(`/games/${it.id}/not-a-game`), 'It won’t be listed any more. You can bring it back from the library’s page.')}><IconClose /> It isn’t a game</Button>
      </div>
      {choosing && <ConsolePicker consoles={consoles} disabled={busy} label="Which console this game is for" onClose={() => { setChoosing(false); toggle.current?.focus() }} onPick={(c, name) => run(() => api.put(`/games/${it.id}/console`, { console: c }), `It’s a ${name} game now.`)} />}
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
      <div className="gr-versions">
        <h2 className="shelf-head">Other versions of this game</h2>
        <ul>{others.map((x) => <li key={x.id}><Link to={`/item/${x.id}`}>{x.label || 'Another version'}</Link></li>)}</ul>
      </div>
    </section>
  )
}

// Why a game can't play, before anyone presses Play. It's for everyone, as it's about
// their file and what would help.
export function GameProblem({ d }: { d: GameDetailsInfo }) {
  if (!d.problem) return null
  return <Notice tone="danger" title="This game can’t play as it is">{d.problemText}</Notice>
}
