// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// keepDataChoice is the "Keep its data" box of a module's Remove dialog: ticked unless someone
// unticks it, and then the module is told to delete what it keeps (POST .../remove?keepData=0).
export function keepDataChoice(name: string) {
  const box = { keep: true }
  const node = (
    <label className="keep-data">
      <input type="checkbox" defaultChecked onChange={(e) => { box.keep = e.target.checked }} />{' '}
      Keep what {name} saved here (its own settings, saves and lists), so installing it again carries on where it left off
    </label>
  )
  return { node, query: () => (box.keep ? '' : '?keepData=0') }
}
