// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The server keeps where first run is (GET and PUT /setup/state) in the twelve numbered steps, by the same names as
// plan.ts's DRAWN: language, welcome, account, server, modules, libraries, modulesetup, looks, people, remote, extras,
// ready. A page is saved as the numbered step it is part of.
import { DRAWN, drawnId, type StepId } from './plan'

// The numbered steps behind this page's, in order, for `done`.
export const doneBefore = (id: StepId): string[] => {
  const i = DRAWN.findIndex((d) => d.id === drawnId(id))
  return i > 0 ? DRAWN.slice(0, i).map((d) => d.id) : []
}
