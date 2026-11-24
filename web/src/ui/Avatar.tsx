// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { avatarColor } from './avatarColor'
import { AvatarShape, parseAvatar } from './avatars'

// Avatar: the profile's initial on a colour picked from its name, so each
// person keeps the same colour on every device (see ui/avatarColor).
// A profile that chose a picture shows that instead (ui/avatars).
// Its own file so that the shell can show it without the sign-in page's code (#643).
export function Avatar({ name, big, avatar }: { name: string; big?: boolean; avatar?: string }) {
  const chosen = parseAvatar(avatar)
  if (chosen) return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: chosen.background }} aria-hidden="true"><AvatarShape shape={chosen.shape} /></span>
  return <span className={'avatar-tile' + (big ? ' big' : '')} style={{ background: avatarColor(name) }} aria-hidden="true">{name.trim()[0]?.toUpperCase() || '?'}</span>
}
