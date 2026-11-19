// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getTvMode } from '../tvmode/mode'
import { OnAComputerDialog } from '../tvmode/OnAComputer'
import { Button, Dialog } from '.'
import { t, tk, tx } from '../i18n'

// Advanced settings (the Canvas studio and its loop editor, and the like) are made on a computer. On a phone, a
// tablet or a television, where the screen or the pointer can't do them justice, they say so and where to go,
// instead of opening half-usable. The apps do the same (docs/plans/PLAYER-UI-PORTING.md, "Walled-off settings").
export const advancedHere = (): boolean =>
  !getTvMode() && !(typeof matchMedia === 'function' && matchMedia('(max-width: 700px), (pointer: coarse) and (max-width: 1100px)').matches)

// The things walled off this way, each with its whole sentence (so each language can say it its own way).
const WALLED: Record<string, { name: string; title: string; body: string }> = {
  'Voice settings': { name: tk('Voice settings'), title: tk('Voice settings: on a computer'), body: tk('Voice settings need a bigger screen and a mouse or trackpad.') },
  'Visualizer effects': { name: tk('Visualizer effects'), title: tk('Visualizer effects: on a computer'), body: tk('Visualizer effects need a bigger screen and a mouse or trackpad.') },
  'A Canvas video': { name: tk('A Canvas video'), title: tk('A Canvas video: on a computer'), body: tk('A Canvas video needs a bigger screen and a mouse or trackpad.') },
  'Advanced quality settings': { name: tk('Advanced quality settings'), title: tk('Advanced quality settings: on a computer'), body: tk('Advanced quality settings need a bigger screen and a mouse or trackpad.') },
}

// what: one of WALLED's names, in English.
export function UseAComputer({ what, onClose }: { what: string; onClose: () => void }) {
  // In TV mode the same wall as TV mode's other editors (docs/TV-MODE.md): a TV device is sent to a computer with a
  // QR code, a computer in TV mode is offered Leave TV mode.
  if (getTvMode()) return <OnAComputerDialog title={WALLED[what] ? tx(WALLED[what].name) : t('On a computer')} onClose={onClose} />
  const w = WALLED[what]
  return (
    <Dialog title={w ? tx(w.title) : t('On a computer')} onClose={onClose} size="sm" actions={<Button variant="primary" onClick={onClose}>{t('OK')}</Button>}>
      <p>{w ? tx(w.body) : t('This needs a bigger screen and a mouse or trackpad.')} {t('Open it in the WittyMediaClient app on a computer, or in a web browser, to make changes. What you choose there shows here too.')}</p>
    </Dialog>
  )
}
