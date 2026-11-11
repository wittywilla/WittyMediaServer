// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { setNavStyle, useNavStyle, type NavStyle } from '../../navPrefs'
import { useApp } from '../../state'
import { applyTheme, type ThemeMeta } from '../../theme'
import { Button } from '../../ui'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { ChoiceCards, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'

export function Look({ info, next, back }: StepProps) {
  const { serverTheme, setServerTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [pick, setPick] = useState(serverTheme)
  const tv = useTvMode()
  const nav = useNavStyle()
  useEffect(() => { api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {}) }, [])
  const choose = (id: string) => { setPick(id); applyTheme(id) }
  const save = async () => {
    try { await api.put('/settings', { theme: pick }); setServerTheme(pick) } catch { /* can be changed later */ }
    next()
  }
  const shown = themes.filter((t) => t.builtin)
  return (
    <section>
      <StepHead title="Make it yours" lead="Pick a look for everyone. It changes right away. People can pick their own later." />
      {shown.length > 0 && (
        <ChoiceCards label="Theme" value={pick} onChange={choose} small cards={shown.map((t) => ({
          value: t.id, title: t.name.replace(/^WittyMediaServer /, ''),
          pic: (
            <span className="oobe-theme-pic" style={{ background: t.preview.bg }} aria-hidden="true">
              <i className="bar" style={{ background: t.preview.surface }} />
              <i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} />
              <i className="btn" style={{ background: t.preview.accent }} />
            </span>
          ),
        }))} />
      )}
      <h2 className="oobe-sub">Menus on this device</h2>
      <ChoiceCards<NavStyle> label="Menus on this device" value={nav} onChange={setNavStyle} small cards={[
        { value: 'top', title: 'Top bar', text: 'Libraries along the top', pic: <span className="layout-pic pic-top" aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span> },
        { value: 'side', title: 'Side menu', text: 'Libraries in a menu on the left', pic: <span className="layout-pic pic-side" aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span> },
        { value: 'both', title: 'Both', text: 'The top bar and the side menu', pic: <span className="layout-pic pic-both" aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span> },
      ]} />
      <div className="su-gap">
        <TickRow kind="switch" checked={tv} onChange={setTvMode} title="TV mode" text="Laid out and driven like the Apple TV and Android TV apps, with a remote or a controller, for a screen across the room. Only on this device." />
      </div>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={save}>Continue</Button></StepActions>
    </section>
  )
}
