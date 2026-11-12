// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A page shown over another (docs/SDUI.md, navigate with as "layer"): the quick menu, a sheet. It is the page itself, drawn in a sheet
// with Back closing it, so a controller and a remote use it like any page. An address that isn't a page of the server's is gone to.
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Sheet } from '../../ui/Overlay'
import SduiPage from '../SduiPage'
import { resolve } from '../routes'

export default function LayerHost({ to, onClose }: { to: string; onClose: () => void }) {
  const nav = useNavigate()
  const [path, search = ''] = to.split('?')
  const at = resolve(path, search ? '?' + search : '')
  const page = at?.kind === 'page' ? at : null
  useEffect(() => { if (!page) { onClose(); nav(to) } }, [page, to]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!page) return null
  return <Sheet title="Menu" onClose={onClose} className="sdui-layer"><SduiPage page={page.page} params={page.params} /></Sheet>
}
