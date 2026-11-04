import { api } from '../../api'
import { toast } from '../../state'
import { ask } from '../../ui'
import { forgetLineup } from './tvApi'
import './reminders.css'

// Reset channel: asks first, in plain words, then makes the channel's schedule
// again from its settings and the files that match them now. Shared by the
// channel's editor and its details in the guide. Resolves true once it's done.

type Reminders = { kept: number; moved: number; dropped: number; droppedTitles?: string[] }

export async function resetChannel(id: number, name: string): Promise<boolean> {
  let startOver = false
  const yes = await ask({
    title: `Reset “${name}”?`,
    // The dialog's description is a paragraph already, so these are spans laid out as paragraphs.
    body: (
      <>
        <span className="tv-reset-para">What’s on now stops, and the guide is rebuilt from the channel’s current settings and the files that match them now. Anyone watching moves straight to the new schedule.</span>
        <span className="tv-reset-para">Reminders follow their programme to its new time. A reminder for something that no longer airs is removed, and you’ll be told which.</span>
        <label className="check-row"><input type="checkbox" onChange={(e) => { startOver = e.target.checked }} /> Also start every show from its first episode, and shuffle again</label>
      </>
    ),
    confirm: 'Reset channel',
    danger: true,
  })
  if (!yes) return false
  try {
    const r = await api.post<{ watching: boolean; reminders: Reminders }>(`/channels/${id}/reset`, { startOver })
    forgetLineup()
    toast(resetNote(name, r.reminders, r.watching))
    return true
  } catch (e) {
    toast((e as Error).message)
    return false
  }
}

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

export function resetNote(name: string, r: Reminders, watching: boolean) {
  const parts = [`${name} has a new schedule${watching ? ', and everyone watching is on it' : ''}.`]
  const said = [
    r.kept ? `${count(r.kept, 'reminder', 'reminders')} kept` : '',
    r.moved ? `${count(r.moved, 'reminder', 'reminders')} moved to a new time` : '',
  ].filter(Boolean)
  if (said.length) parts.push(said.join(', ') + '.')
  if (r.dropped) {
    const titles = r.droppedTitles?.length ? `: ${r.droppedTitles.join(', ')}${r.dropped > r.droppedTitles.length ? ' and more' : ''}` : ''
    parts.push(`${count(r.dropped, 'reminder was', 'reminders were')} removed, as ${r.dropped === 1 ? 'that programme no longer airs' : 'those programmes no longer air'}${titles}.`)
  }
  return parts.join(' ')
}
