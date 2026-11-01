// What the main menu holds and in what order. Plain data and arithmetic, so the
// same rules can be tested here and ported to the native clients.

export type NavItem = { id: string; label: string; to: string; kind?: string; end?: boolean }

export type NavPrefs = {
  order?: string[]
  hidden?: string[]
  grouped?: boolean
  groups?: Record<string, string> // item id -> a name from GROUPS; '' keeps it out of any group
  overflow?: 'more' | 'scroll'
  collapse?: 'rail' | 'hide'
  style?: 'side' | 'top' | 'both'
}

export const GROUPS = ['Watch', 'Listen', 'Read', 'Play']

const KIND_GROUP: Record<string, string> = {
  movies: 'Watch', shows: 'Watch', anime: 'Watch', videos: 'Watch', shorts: 'Watch', channels: 'Watch', musicvideos: 'Watch', photos: 'Watch', commercials: 'Watch',
  music: 'Listen', podcasts: 'Listen', audiobooks: 'Listen', radio: 'Listen',
  books: 'Read', comics: 'Read',
  games: 'Play',
}

export function groupOf(item: NavItem, prefs: NavPrefs): string {
  const chosen = prefs.groups?.[item.id]
  if (chosen !== undefined) return chosen
  if (item.id === 'couch') return 'Play'
  return (item.id.startsWith('lib:') && (KIND_GROUP[item.kind || ''] || 'Watch')) || ''
}

// arrange: the items someone will see, in their order. A library added since they
// arranged things joins at the end instead of staying out of sight.
export function arrange(items: NavItem[], prefs: NavPrefs, includeHidden = false): NavItem[] {
  const order = prefs.order || []
  const at = new Map(order.map((id, i) => [id, i]))
  const hidden = new Set(prefs.hidden || [])
  return items
    .map((item, i) => ({ item, rank: at.get(item.id) ?? order.length + i }))
    .sort((a, b) => a.rank - b.rank)
    .map((x) => x.item)
    .filter((it) => includeHidden || !hidden.has(it.id))
}

export type NavEntry = { key: string; item: NavItem } | { key: string; group: string; items: NavItem[] }

// entries: what the bar shows. A group sits where its first member is, and a group
// of one is just a link: a menu with one thing in it only adds a click.
export function entries(visible: NavItem[], prefs: NavPrefs): NavEntry[] {
  if (!prefs.grouped) return visible.map((item) => ({ key: item.id, item }))
  const members = new Map<string, NavItem[]>()
  for (const it of visible) {
    const g = groupOf(it, prefs)
    if (g) members.set(g, [...(members.get(g) || []), it])
  }
  const out: NavEntry[] = []
  const placed = new Set<string>()
  for (const it of visible) {
    const g = groupOf(it, prefs)
    const list = g ? members.get(g)! : []
    if (list.length < 2) out.push({ key: it.id, item: it })
    else if (!placed.has(g)) { placed.add(g); out.push({ key: 'group:' + g, group: g, items: list }) }
  }
  return out
}

export const isActive = (item: NavItem, path: string) => item.end ? path === item.to : path === item.to || path.startsWith(item.to + '/')

export const entryActive = (e: NavEntry, path: string) => 'item' in e ? isActive(e.item, path) : e.items.some((it) => isActive(it, path))

// fitCount: how many entries fit along the bar with room left for "More". While the
// page you're on is under More, its name is on that button, which then needs the room
// `activeWidth` asks for.
export function fitCount(widths: number[], gap: number, room: number, moreWidth: number, activeIndex = -1, activeWidth = 0): number {
  const used = (n: number) => { let w = 0; for (let i = 0; i < n; i++) w += widths[i] + gap; return w }
  if (used(widths.length) - gap <= room) return widths.length
  const fit = (more: number) => { let n = widths.length - 1; while (n > 0 && used(n) + more > room) n--; return n }
  const n = fit(moreWidth)
  return activeIndex >= n && activeWidth > moreWidth ? fit(activeWidth) : n
}

export const moved = <T,>(list: T[], from: number, to: number): T[] => {
  const next = [...list]
  next.splice(to, 0, ...next.splice(from, 1))
  return next
}
