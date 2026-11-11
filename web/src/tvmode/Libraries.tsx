// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { inMainMenus } from '../api'
import { EmptyState } from '../components/common'
import { IconLibrary, libIcon } from '../icons'
import { countOf } from '../pages/HomePage'
import { useApp } from '../state'

// Libraries: every library the profile can see, as a grid of tiles. A games library opens Games' view of it.
export default function Libraries() {
  const { libraries } = useApp()
  const list = libraries.filter(inMainMenus)
  return (
    <div className="tvm-page">
      <h1 className="tvm-title">Libraries</h1>
      {list.length === 0
        ? <EmptyState icon={<IconLibrary />} title="No libraries here yet" text="An owner adds them in Settings on the server’s website." />
        : (
          <div className="tvm-lib-grid" data-nav-group>
            {list.map((l) => (
              <Link key={l.id} to={l.type === 'games' && l.console !== 'rig' ? `/games/library/${l.id}` : `/library/${l.id}`} className="tvm-lib">
                <span className="tvm-lib-ic" aria-hidden="true">{libIcon(l.type)}</span>
                <span className="tvm-lib-txt"><b>{l.name}</b><span>{countOf(l)}</span></span>
              </Link>
            ))}
          </div>
        )}
    </div>
  )
}
