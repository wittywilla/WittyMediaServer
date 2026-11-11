// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../api'
import { getNavStyle } from '../navPrefs'
import { ArrangeList } from '../sdui/ArrangeList'
import { useApp } from '../state'
import { Button, LinkButton } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Select, Switch } from '../ui/Field'
import { ask } from '../ui/Overlay'
import { toast } from '../ui/Toast'
import { GROUPS, groupOf, type NavPrefs } from './navModel'
import { navIcon, setOwnerNavPrefs, useMenu, useNavPrefs } from './useNav'

// Arrange the main menu: drag a row by its handle, or use the arrow keys on it, or
// the Move buttons (which is what a remote has). Each change takes effect at once.
// Which entries, and in what order, is this profile's arrangement of the menu the
// server describes (docs/SDUI.md, "Navigation"), the same one every app of theirs
// follows; how the menu is drawn here is a choice of its own beside it.
export default function NavCustomize() {
  const { user } = useApp()
  const menu = useMenu()
  const { prefs, mine, owner, setMine, server } = useNavPrefs()
  const [rows, setRows] = useState(menu.rows)
  const [undo, setUndo] = useState<{ order: string[]; hidden: string[] }[]>([])
  const sig = menu.rows.map((r) => r.id + (r.hidden ? '!' : '')).join('|')
  useEffect(() => { setRows(menu.rows) }, [sig]) // eslint-disable-line react-hooks/exhaustive-deps
  // "Arrange this menu" in the bar's More menu comes straight here.
  useEffect(() => { if (location.hash === '#navigation') document.getElementById('navigation')?.scrollIntoView() }, [])

  const failed = () => toast('Could not save that', { tone: 'danger' })
  const saveLook = (patch: Partial<NavPrefs>) => setMine({ ...prefs, ...patch }).catch(failed)
  const state = (list: typeof rows) => ({ order: list.map((r) => r.id), hidden: list.filter((r) => r.hidden).map((r) => r.id) })
  const apply = (next: typeof rows, remember = true) => {
    if (remember) setUndo((u) => [...u.slice(-19), state(rows)])
    setRows(next)
    const s = state(next)
    menu.save(s.order, s.hidden).catch(() => { setRows(rows); failed() })
  }
  const move = (id: string, to: number) => {
    const from = rows.findIndex((r) => r.id === id)
    if (from < 0 || to < 0 || to >= rows.length) return
    const next = [...rows]
    next.splice(to, 0, ...next.splice(from, 1))
    apply(next)
  }
  const back = () => {
    const was = undo[undo.length - 1]
    if (!was) return
    setUndo((u) => u.slice(0, -1))
    const byId = new Map(rows.map((r) => [r.id, r]))
    apply(was.order.map((id) => byId.get(id)).filter((r): r is typeof rows[number] => !!r).map((r) => ({ ...r, hidden: was.hidden.includes(r.id) })), false)
  }
  const startAgain = async () => {
    try {
      await menu.reset()
      setUndo([])
      toast(server ? 'The menu is back as everyone has it' : 'Back to the server’s arrangement', { tone: 'ok' })
    } catch { failed() }
  }
  const shareLook = async () => {
    if (!await ask({ title: server ? 'Use this look for everyone?' : 'Use this arrangement for everyone?', body: server
      ? 'How the menu is drawn (grouping, what a full bar does, where it sits) becomes the default. People who have chosen their own keep theirs.'
      : 'People who have arranged their own menu keep theirs. Everyone else gets this one, and it can be changed again at any time.', confirm: 'Use for everyone' })) return
    const value: NavPrefs = server ? { ...prefs, style: getNavStyle() } : { ...prefs, ...state(rows), style: getNavStyle() }
    try {
      await api.put('/settings', { navDefault: JSON.stringify(value) })
      setOwnerNavPrefs(value)
      toast('This is now the default for everyone', { tone: 'ok' })
    } catch (err) { toast((err as Error).message, { tone: 'danger' }) }
  }
  const arranged = rows.some((r) => r.hidden) || rows.some((r, i) => r.id !== menu.rows[i]?.id) || undo.length > 0

  return (
    <div className="nav-custom" id="navigation">
      <h3>Arrange the menu</h3>
      <div className="nav-custom-opts">
        <div className="nav-custom-opt"><span>When the top bar is full</span>
          <Segmented label="When the top bar is full" value={prefs.overflow || 'more'} onChange={(v) => saveLook({ overflow: v })}
            options={[{ value: 'more', label: 'Use a More menu' }, { value: 'scroll', label: 'Scroll sideways' }]} />
        </div>
        <div className="nav-custom-opt"><span>When the side menu is folded</span>
          <Segmented label="When the side menu is folded" value={prefs.collapse || 'rail'} onChange={(v) => saveLook({ collapse: v })}
            options={[{ value: 'rail', label: 'Keep the icons' }, { value: 'hide', label: 'Hide it' }]} />
        </div>
        <div className="nav-custom-toggle"><span id="nav-grouped">Group into Watch, Listen, Read and Play</span>
          <Switch labelledBy="nav-grouped" checked={!!prefs.grouped} onChange={(on) => saveLook({ grouped: on })} />
        </div>
      </div>
      {server && !menu.editable
        ? <p className="muted">Whoever manages this profile has turned off rearranging, so the menu is as the server has it.</p>
        : (
          <ArrangeList label="Menu items, in order" canMove canHide
            rows={rows.map((r) => ({ id: r.id, title: r.title, hidden: r.hidden, icon: r.item ? navIcon(r.item) : undefined, hideFixed: r.fixed }))}
            onMove={move} onHidden={(id, hidden) => apply(rows.map((r) => (r.id === id && !r.fixed ? { ...r, hidden } : r)))}
            extra={(row) => {
              const it = rows.find((r) => r.id === row.id)?.item
              return prefs.grouped && it ? (
                <Select aria-label={`Group for ${row.title}`} value={groupOf(it, prefs)} onChange={(e) => saveLook({ groups: { ...prefs.groups, [it.id]: e.target.value } })}
                  options={[{ value: '', label: 'No group' }, ...GROUPS.map((g) => ({ value: g, label: g }))]} />
              ) : null
            }} />
        )}
      <div className="nav-custom-foot">
        {server && menu.editable && <Button variant="ghost" onClick={back} disabled={!undo.length}>Undo</Button>}
        {(server ? menu.editable && arranged : !!mine) && <Button variant="ghost" onClick={startAgain}>{server ? 'Reset the menu' : owner.order ? 'Use the server’s arrangement' : 'Start again'}</Button>}
        {user?.isAdmin && <Button onClick={shareLook}>{server ? 'Use this look for everyone' : 'Use for everyone'}</Button>}
        {user?.isAdmin && server && <LinkButton to="/settings/pages/navigation" variant="ghost">Change the menu for everyone</LinkButton>}
      </div>
    </div>
  )
}
