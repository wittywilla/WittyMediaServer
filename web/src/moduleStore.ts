import { api } from './api'

// What the server says about modules, for the three places that show them: the
// store in Settings, Add Library and first-run setup. Nothing here goes online.

export type Withdrawn = { version: string; severity: string; reason: string }
export type Offer = { version: string; minServer: string; maxServer: string; notes?: string; url?: string; published?: number; prerelease: boolean; size?: number; reason?: string; blocked?: string }
export type ModuleEvent = { at: number; kind: string; version?: string; detail?: string }
export type ModuleSource = { repo: string; fingerprint: string; addedAt: number; addedBy?: string }
export type ModuleStatus = {
  id: string; name: string; version: string; builtinVersion: string; delivery: string; updatable: boolean
  minServer: string; maxServer: string; api: number; repo: string; stockRepo: string; state: 'builtin' | 'installed' | 'fallback'
  notice?: string; previous?: string; canRollback: boolean; latest?: Offer; tooNew?: Offer
  phase: 'idle' | 'downloading' | 'verifying' | 'installing' | 'failed'; done?: number; total?: number; error?: string
  group?: string; summary?: string; needs?: string[]; updates?: string; stays?: string; pin?: string
  withdrawn?: Withdrawn; lastFailure?: string; events?: ModuleEvent[]
  installed: boolean; core?: boolean; libraries?: string[]; requires?: string[]; requiredBy?: string[]
  source?: ModuleSource; noFork?: string; autoUpdate: boolean
}
export type LibraryKind = { type: string; available: boolean; modules: ModuleRef[]; label?: string; shared?: boolean }
export type ModuleRef = { id: string; name: string; installed: boolean }
export type ModuleReport = { server: string; api: number; minApi: number; checkedAt?: number; checkError?: string; holdUntil?: number; modules: ModuleStatus[]; libraries: LibraryKind[] }
export type VersionInfo = Offer & { builtin?: boolean; inUse?: boolean; installed?: boolean; older?: boolean; allowed: boolean; refused?: string; warning?: string; withdrawn?: Withdrawn }
export type VersionList = { id: string; name: string; repo: string; inUse: string; pin?: string; checkedAt?: number; versions: VersionInfo[] }

// The store's shelves, in order. Core modules sit apart from these.
const GROUPS: [string, string][] = [['watch', 'Watch'], ['discs', 'Discs'], ['listen', 'Listen'], ['read', 'Read'], ['photos', 'Photos'], ['tv', 'Live TV'], ['games', 'Games'], ['system', 'System']]

// shelves sorts modules by what they do. One whose group this build has no name
// for still shows, on a shelf of its own at the end.
export function shelves(mods: ModuleStatus[]): { id: string; title: string; modules: ModuleStatus[] }[] {
  const rest = mods.filter((m) => !GROUPS.some(([id]) => id === m.group))
  // On a shelf, the module that brings the most kinds of library comes first.
  const bySize = (a: ModuleStatus, b: ModuleStatus) => (b.libraries?.length || 0) - (a.libraries?.length || 0)
  return [
    ...GROUPS.map(([id, title]) => ({ id, title, modules: mods.filter((m) => m.group === id).sort(bySize) })),
    { id: 'other', title: 'Other', modules: rest },
  ].filter((s) => s.modules.length > 0)
}

// libraryKinds asks what kinds of library can be made. A build without modules, or
// an answer that doesn't come, gives nothing, which means everything is available:
// the server still has the last word when a library is made.
export const libraryKinds = () => api.get<{ libraries: LibraryKind[] }>('/modules/libraries').then((r) => r.libraries || []).catch(() => [] as LibraryKind[])

// What this server calls a kind of library, if it has chosen a name of its own.
export const kindTitle = (kinds: LibraryKind[] | null, type: string, usual: string) => kinds?.find((k) => k.type === type)?.label || usual

export const orList = (names: string[]) => names.length < 2 ? names[0] || '' : `${names.slice(0, -1).join(', ')} or ${names[names.length - 1]}`

// The kind's entry if its module is missing; undefined if it can be made.
export const missingKind = (kinds: LibraryKind[], type: string) => kinds.find((k) => k.type === type && !k.available)
