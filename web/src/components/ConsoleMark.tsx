// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { logoImageUrl, logoVariant, useLogo } from './BrandLogo'
import './consolemark.css'

// A console, or a game studio, as a mark: its transparent logo if the owner has agreed to
// logos and one was found, and otherwise a small typed label that's a finished look of its
// own, the same height and weight wherever it goes. See docs/METADATA.md#the-logo-service.
// Show <LegalNote kind="logos" /> once on any screen that uses this.
//
//   <ConsoleMark console={c} />                      a console from GET /games/consoles
//   <ConsoleMark console={c} size="lg" on="dark" />  bigger, on a plate that's always dark
//   <StudioMark name={it.studio} />                  a game studio

type Size = 'sm' | 'md' | 'lg'

function Mark({ kind, lookup, text, title, size, on, className }: { kind: 'console' | 'gamestudio'; lookup: string; text: string; title: string; size: Size; on?: 'dark' | 'light'; className?: string }) {
  const info = useLogo(kind, lookup)
  const cls = `gc-mark ${size}` + (className ? ' ' + className : '')
  if (!info?.has) return <span className={cls + ' gc-mark-text'} title={title}>{text}</span>
  const variant = info.opaque ? 'original' : on === 'dark' ? 'white' : on === 'light' ? 'dark' : logoVariant()
  return <img className={cls + (info.opaque ? ' gc-mark-plate' : '')} src={logoImageUrl(info, variant)} alt={title} title={title} loading="lazy" />
}

export function ConsoleMark({ console: c, size = 'md', on, className }: { console: { id: string; name: string; short: string; logoName?: string }; size?: Size; on?: 'dark' | 'light'; className?: string }) {
  return <Mark kind="console" lookup={c.logoName || c.name} text={c.short} title={c.name} size={size} on={on} className={className} />
}

export function StudioMark({ name, size = 'sm', on, className }: { name: string; size?: Size; on?: 'dark' | 'light'; className?: string }) {
  return <Mark kind="gamestudio" lookup={name} text={name} title={name} size={size} on={on} className={className} />
}
