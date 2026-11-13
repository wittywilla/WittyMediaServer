// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useId, useRef, useState } from 'react'
import { Button, Dialog } from '../../../ui'
import { confirmedIds, olderTitle, olderWords, startChoices, type OlderChoices, type OlderVersion } from './olderServer'
import './olderServer.css'

// The pop-up before a module version made for an older server is installed, wherever that can
// happen: the store, Settings, Modules, first-run setup and a setup step's "add the module". It
// says which version, which server it was made for and what may not work. "Don't install" is
// first, focused, and what Escape means; only "Install anyway" sends confirmOlder. Several at once
// (setup installs a set) are one pop-up, with a choice for each. The dialog keeps the focus inside
// it and gives it back when it closes (a11y.tsx).

export function OlderServerDialog({ list, onDone }: { list: OlderVersion[]; onDone: (installAnyway: string[]) => void }) {
  const [choices, setChoices] = useState<OlderChoices>(() => startChoices(list))
  const id = useId()
  const none = () => onDone([])
  if (list.length === 1) {
    const o = list[0]
    const [first, ...rest] = olderWords(o)
    return (
      <Dialog alert size="sm" title={olderTitle(list)} onClose={none} description={first}
        actions={<>
          <Button autoFocus onClick={none}>Don’t install</Button>
          <Button variant="danger" onClick={() => onDone([o.module])}>Install anyway</Button>
        </>}>
        {rest.map((p) => <p key={p} className="muted">{p}</p>)}
      </Dialog>
    )
  }
  return (
    <Dialog alert size="md" title={olderTitle(list)} onClose={none}
      description="No version made for this server is published yet for these. Each one may start, but its pages, settings and features from this server may be missing. Choose for each one; Don’t install is the safe choice."
      actions={<>
        <Button autoFocus onClick={none}>Don’t install any</Button>
        <Button variant="danger" onClick={() => onDone(confirmedIds(choices))}>Continue</Button>
      </>}>
      <ul className="st-older-list">
        {list.map((o) => {
          const name = `${id}-${o.module}`
          const [first] = olderWords(o)
          return (
            <li key={o.module}>
              <fieldset>
                <legend><b>{o.name} {o.version}</b></legend>
                <p id={name + '-why'} className="muted">{first}</p>
                <div role="radiogroup" aria-label={`${o.name} ${o.version}`} aria-describedby={name + '-why'}>
                  <label><input type="radio" name={name} checked={!choices[o.module]} onChange={() => setChoices((c) => ({ ...c, [o.module]: false }))} /> Don’t install</label>
                  <label><input type="radio" name={name} checked={!!choices[o.module]} onChange={() => setChoices((c) => ({ ...c, [o.module]: true }))} /> Install anyway</label>
                </div>
              </fieldset>
            </li>
          )
        })}
      </ul>
    </Dialog>
  )
}

// useOlderAsk gives a page the pop-up as a question to await: the module ids to install anyway,
// or none. The page renders dialog where its other dialogs are.
export function useOlderAsk() {
  const [list, setList] = useState<OlderVersion[] | null>(null)
  const answer = useRef<((ids: string[]) => void) | null>(null)
  const ask = (l: OlderVersion[]) => new Promise<string[]>((resolve) => {
    answer.current?.([])
    answer.current = resolve
    setList(l)
  })
  const dialog = list ? <OlderServerDialog key={list.map((o) => o.module + o.version).join()} list={list} onDone={(ids) => { setList(null); const a = answer.current; answer.current = null; a?.(ids) }} /> : null
  return { ask, dialog }
}
