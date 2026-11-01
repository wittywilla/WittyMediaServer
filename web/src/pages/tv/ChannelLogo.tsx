import { useEffect, useState } from 'react'
import { logoImageUrl, useLogo } from '../../components/BrandLogo'
import type { Look } from './look'
import { logoSrc, type GuideChannel } from './tvApi'

// A channel's picture: the owner's own upload first, then what the logo service has
// for its network or studio. The service gives nothing until the owner has agreed to
// the download or uploaded one, and then the channel simply goes by its name.

type Mark = Pick<GuideChannel, 'logo' | 'name' | 'brand'>

export function ChannelLogo({ ch, look, on, fallback = null }: { ch: Mark; look: Look; on: 'dark' | 'light'; fallback?: React.ReactNode }) {
  const own = look.logos ? logoSrc(ch.logo) : ''
  const info = useLogo(ch.brand?.kind || 'studio', look.logos && !own && ch.brand ? ch.brand.name : '')
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [own, info?.id, info?.version])
  if (broken) return <>{fallback}</>
  if (own) return <img className="ctv-logo" src={own} alt={ch.name} loading="lazy" onError={() => setBroken(true)} />
  if (!info?.has) return <>{fallback}</>
  const variant = info.opaque ? 'original' : on === 'dark' ? 'white' : 'dark'
  return <img className={'ctv-logo' + (info.opaque ? ' plate' : '')} src={logoImageUrl(info, variant)} alt={ch.name} loading="lazy" onError={() => setBroken(true)} />
}

// In the guide's rows the logo stands for the name.
export function ChannelMark({ ch, look, on }: { ch: Mark; look: Look; on: 'dark' | 'light' }) {
  return <ChannelLogo ch={ch} look={look} on={on} fallback={<span className="ctv-name">{ch.name}</span>} />
}
