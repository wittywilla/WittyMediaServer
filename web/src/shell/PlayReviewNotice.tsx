// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// On WittyPlay's home, for an administrator: files a scan wasn't sure about ("6 files need a look"), one line for each games library that
// has some, leading to that library in WittyPlay where they are looked at. Nothing when there is nothing to say.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type GameReview } from '../api'
import { IconInfo } from '../icons'
import { useApp } from '../state'
import { PLAY_LIBRARY } from './playPaths'

export function PlayReviewNotice() {
  const { user, libraries } = useApp()
  const [needs, setNeeds] = useState<{ id: number; name: string; n: number; blocked: number }[]>([])
  const games = libraries.filter((l) => l.type === 'games')
  const key = games.map((l) => l.id).join(',')
  useEffect(() => {
    if (!user?.isAdmin || !games.length) return
    let on = true
    Promise.all(games.map((l) => api.get<GameReview>(`/libraries/${l.id}/games/review?kind=needs&limit=1`)
      .then((r) => ({ id: l.id, name: l.name, n: r.counts?.needs || 0, blocked: r.counts?.blocked || 0 })).catch(() => ({ id: l.id, name: l.name, n: 0, blocked: 0 }))))
      .then((all) => { if (on) setNeeds(all.filter((x) => x.n + x.blocked > 0)) })
    return () => { on = false }
  }, [user?.isAdmin, key]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!needs.length) return null
  return (
    <div className="wplay-review-notice" role="status">
      {needs.map((x) => (
        <p key={x.id}>
          <IconInfo />
          {x.n > 0
            ? <span><b>{x.n.toLocaleString()} {x.n === 1 ? 'file' : 'files'} in {x.name} need{x.n === 1 ? 's' : ''} a look.</b> They might be games, but nothing proves it.</span>
            : <span><b>{x.blocked.toLocaleString()} {x.blocked === 1 ? 'game' : 'games'} in {x.name} can’t play as {x.blocked === 1 ? 'it is' : 'they are'}.</b></span>}
          <Link className="wplay-review-go" to={`${PLAY_LIBRARY}/${x.id}`}>Take a look</Link>
        </p>
      ))}
    </div>
  )
}
