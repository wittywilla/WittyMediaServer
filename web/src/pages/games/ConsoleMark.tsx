import { useState } from 'react'
import type { GameConsole } from '../../api'
import { mediaUrl } from '../../api'

// A console's mark: its transparent icon if someone has agreed to download those, and
// otherwise its short name, set large. Words are never a lesser version: a text tile
// is meant to look finished on its own.

// The icon service tells the page what it has; until it's there, every console is words.
let iconList: Set<string> | null = null
export const setConsoleIcons = (ids: string[]) => { iconList = new Set(ids) }

export function ConsoleMark({ console: c, heading }: { console: GameConsole; heading?: boolean }) {
  const [broken, setBroken] = useState(false)
  const icon = !broken && iconList?.has(c.id)
  if (icon) {
    return (
      <span className={'gm-mark' + (heading ? ' heading' : '')}>
        <img src={mediaUrl(`/api/games/consoles/${encodeURIComponent(c.id)}/icon`)} alt="" onError={() => setBroken(true)} />
        {heading ? <span>{c.name}</span> : <span className="sr-only">{c.name}</span>}
      </span>
    )
  }
  return heading ? <span className="gm-mark heading">{c.name}</span> : <span className="gm-mark text">{c.short}</span>
}
