// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { EmptyState } from '../components/common'
import { IconMore } from '../icons'
import { sduiIcon } from '../sdui/render/icons'
import { useMenu } from '../shell/useNav'
import { moreTabs } from './nav'

// More: the entries of the server's menu that it places under More on a television (docs/SDUI.md,
// "Navigation"), as a grid of tiles like Libraries, so a long menu never runs off the bar.
export default function More() {
  const menu = useMenu()
  const list = useMemo(() => moreTabs(menu.all), [menu.all])
  return (
    <div className="tvm-page">
      <h1 className="tvm-title">More</h1>
      {list.length === 0
        ? <EmptyState icon={<IconMore />} title="Nothing more here" text="Everything in the menu is on the bar above." />
        : (
          <div className="tvm-lib-grid" data-nav-group>
            {list.map((t) => (
              <Link key={t.id} to={t.path} className="tvm-lib">
                <span className="tvm-lib-ic" aria-hidden="true">{sduiIcon(t.icon)}</span>
                <span className="tvm-lib-txt"><b>{t.label}</b></span>
              </Link>
            ))}
          </div>
        )}
    </div>
  )
}
