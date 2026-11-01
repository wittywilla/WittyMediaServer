import { lazy, Suspense, useState } from 'react'
import { buttonLabel, markProblemsSeen, setPanelOpen, useActivity } from '../activity'
import { IconActivity } from '../icons'
import { IconButton } from '../ui'
import './activity.css'

const ActivityPanel = lazy(() => import('./ActivityPanel'))

// The spot in the top bar for the activity button, just left of the account menu; on
// phones it sits at the right of the row of sections. A ring around an icon showing how
// far along everything is, with a count, and quiet when nothing is going on. Nothing at
// all for an account the owner hasn't given it to, or someone who has hidden it.
const R = 10, C = 2 * Math.PI * R

export default function ActivitySlot() {
  const a = useActivity()
  const [open, setOpen] = useState(false)
  if (!a.loaded || !a.access.visible || a.hidden) return null
  const { active, queued, percent } = a.summary
  const busy = active + queued > 0
  const known = percent >= 0
  const show = (on: boolean) => { setOpen(on); setPanelOpen(on); if (on) markProblemsSeen() }
  return (
    <>
      <IconButton label={buttonLabel(a)} aria-haspopup="dialog" aria-expanded={open} className={'act-btn' + (busy ? ' on' : '')} onClick={() => show(true)}>
        {busy && (
          <svg className={'ui-ring act-ring' + (known ? '' : ' busy')} viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r={R} className="track" />
            <circle cx="12" cy="12" r={R} className="fill" strokeDasharray={C} strokeDashoffset={known ? C * (1 - percent / 100) : C * 0.7} />
          </svg>
        )}
        <IconActivity size={18} />
        {active + queued > 0 && <span className="act-count" aria-hidden="true">{active + queued}</span>}
        {a.unseenProblems > 0 && active + queued === 0 && <span className="act-count bad" aria-hidden="true">!</span>}
      </IconButton>
      {open && <Suspense fallback={null}><ActivityPanel onClose={() => show(false)} /></Suspense>}
    </>
  )
}
