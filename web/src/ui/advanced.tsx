// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getTvMode } from '../tvmode/mode'
import { Button, Dialog } from '.'

// Advanced settings (the Canvas studio and its loop editor, and the like) are made on a computer. On a phone, a
// tablet or a television, where the screen or the pointer can't do them justice, they say so and where to go,
// instead of opening half-usable. The apps do the same (docs/plans/PLAYER-UI-PORTING.md, "Walled-off settings").
export const advancedHere = (): boolean =>
  !getTvMode() && !(typeof matchMedia === 'function' && matchMedia('(max-width: 700px), (pointer: coarse) and (max-width: 1100px)').matches)

export function UseAComputer({ what, onClose }: { what: string; onClose: () => void }) {
  return (
    <Dialog title={`${what} is made on a computer`} onClose={onClose} size="sm" actions={<Button variant="primary" onClick={onClose}>OK</Button>}>
      <p>{what} needs a bigger screen and a mouse or trackpad. Open WittyMediaServer in a web browser on a computer, or in the desktop app, to make changes. What you choose there shows here too.</p>
    </Dialog>
  )
}
