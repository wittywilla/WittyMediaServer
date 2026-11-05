import type { Programme } from './tvApi'
import { fmtClock } from './tvApi'

// "We'll be right back": the card over a channel's slate, when nothing is scheduled. The
// watch view and the guide's preview both show it, so neither looks broken while it's on.
export default function Slate({ name, next }: { name: string; next: Programme | null }) {
  return (
    <div className="ctv-slate" aria-live="polite">
      <div className="ctv-slate-card">
        <div className="ctv-slate-ch">{name}</div>
        <div className="ctv-slate-msg">We'll be right back</div>
        {next && <div className="ctv-slate-next">Coming up at {fmtClock(next.start)}: {next.title}</div>}
      </div>
    </div>
  )
}
