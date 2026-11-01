import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Button } from '../../ui'
import { ChoiceCards, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'

// Belongs to the disc modules, so it's only here when one of them is.
export function Discs({ info, next, back }: StepProps) {
  const [start, setStart] = useState<'menu' | 'title'>('menu')
  const [fx, setFx] = useState(true)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { setStart(s.dvdStart === 'title' ? 'title' : 'menu'); setFx(s.insertFx !== '0') }).catch(() => {}) }, [])
  const save = async () => {
    setBusy(true)
    try { await api.put('/settings', { dvdStart: start, insertFx: fx ? '1' : '0' }) } catch { /* can be changed later */ }
    setBusy(false)
    next()
  }
  return (
    <section>
      <StepHead title="DVDs and Blu-rays" lead="Copies of your discs play the way they do in a disc player. When someone presses play on a disc, what should happen?" />
      <ChoiceCards label="When a disc starts" value={start} onChange={setStart} cards={[
        { value: 'menu', title: 'Show the disc’s menu', text: 'Studio intros, then the menu with its buttons, chapters and extras.', pic: <span className="oobe-choice-pic menu" aria-hidden="true"><i /><i /><i /></span> },
        { value: 'title', title: 'Play the movie straight away', text: 'Skips the intros and menus. The menu is still one button away.', pic: <span className="oobe-choice-pic film" aria-hidden="true"><i /></span> },
      ]} />
      <TickRow kind="checkbox" checked={fx} onChange={setFx} title="Disc animation" text="A tray slides in when a disc starts, with drive sounds." />
      <p className="muted oobe-hint su-gap">Discs with copy protection can’t be played. They’re listed with a plain reason, and nothing is ever unlocked.</p>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={save}>Continue</Button></StepActions>
    </section>
  )
}
