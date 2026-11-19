// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getTvMode } from '../tvmode/mode'
import { OnAComputerDialog } from '../tvmode/OnAComputer'
import { Button, Dialog } from '.'

// Advanced settings (the Canvas studio and its loop editor, and the like) are made on a computer. On a phone, a
// tablet or a television, where the screen or the pointer can't do them justice, they say so and where to go,
// instead of opening half-usable. The apps do the same (docs/plans/PLAYER-UI-PORTING.md, "Walled-off settings").
export const advancedHere = (): boolean =>
  !getTvMode() && !(typeof matchMedia === 'function' && matchMedia('(max-width: 700px), (pointer: coarse) and (max-width: 1100px)').matches)

export function UseAComputer({ what, onClose }: { what: string; onClose: () => void }) {
  // In TV mode the same wall as TV mode's other editors (docs/TV-MODE.md): a TV device is sent to a computer with a
  // QR code, a computer in TV mode is offered Leave TV mode.
  if (getTvMode()) return <OnAComputerDialog title={what} onClose={onClose} />
  const plural = /s$/.test(what) && !/^A /.test(what) // "Voice settings need", "A Canvas video needs"
  return (
    <Dialog title={`${what}: on a computer`} onClose={onClose} size="sm" actions={<Button variant="primary" onClick={onClose}>OK</Button>}>
      <p>{what} {plural ? "need" : "needs"} a bigger screen and a mouse or trackpad. Open it in the WittyMediaClient app on a computer, or in a web browser, to make changes. What you choose there shows here too.</p>
    </Dialog>
  )
}
