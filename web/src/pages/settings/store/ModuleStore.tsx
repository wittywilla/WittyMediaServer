// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import { HelpLink } from '../../../components/HelpLink'
import { Link, useSearchParams } from 'react-router-dom'
import { IconSearch, IconShield } from '../../../icons'
import { Button, EmptyState, LinkButton, Notice, Spinner } from '../../../ui'
import SpecPage from '../../../spec/SpecPage'
import { handledIds } from '../../../spec/model'
import { storeSearchSpec } from '../../../spec/specs/store'
import { ItemCard } from './ItemCard'
import { FailureNotice, type StoreViewState } from './parts'
import { itemModuleKind, MODULE_KINDS, moduleKindTitle, passesExtra, readFilters, STATUSES, statusTitle } from './overrides'
import { useFeatures } from '../../../sdui/useSdui'
import { t, tn } from '../../../i18n'
import { CATEGORIES, categoryOf, categoryTitle, matches, platformName, rank, type StoreItem } from './storeApi'
import { compat } from './compat'

// Browse, in the module store: find a module by searching or by what it's for. What's installed
// and what can be updated are the store's other tabs. The filters live in the address, so Back
// returns to the same results and a filtered view can be shared with another admin.

export default function ModuleStore({ store }: { store: StoreViewState }) {
  const { view, error, reload } = store
  const [params, setParams] = useSearchParams()
  const results = useRef<HTMLDivElement>(null)

  // What's typed is the page's own, and reaches the address when typing pauses: an address that
  // changes on every key would lose keys, and fill up history for nothing.
  const [q, setQ] = useState(params.get('q') || '')
  useEffect(() => {
    if ((params.get('q') || '') === q) return
    const t = setTimeout(() => setParams((p) => { const n = new URLSearchParams(p); if (q) n.set('q', q); else n.delete('q'); return n }, { replace: true }), 250)
    return () => clearTimeout(t)
  }, [q, params, setParams])
  const cat = params.get('cat') || ''
  const publisher = params.get('publisher') || ''
  const platform = params.get('platform') || ''
  const set = (k: string, v: string) => setParams((p) => { const n = new URLSearchParams(p); if (v) n.set(k, v); else n.delete(k); return n }, { replace: true })
  const extra = readFilters(params)
  const { moduleKind, status } = extra
  const filtered = !!(q || cat || publisher || platform || moduleKind || status)
  const features = useFeatures()

  const items = view?.items
  const server = view?.server || ''
  const shown = useMemo(() => (items || []).filter((i) =>
    matches(i, q) &&
    // A library part and an entry with nothing to install are found by searching, not shown on the shelves.
    (!!q || !!i.status?.installed || !compat(i, server).hidden) &&
    (!cat || categoryOf(i) === cat) &&
    (!publisher || i.publisher?.name === publisher) &&
    (!platform || i.platforms?.includes(platform) || i.platforms?.includes('all')) &&
    passesExtra(i, { moduleKind, status })
  ).sort((a, b) => rank(a, q) - rank(b, q) || a.name.localeCompare(b.name)), [items, server, q, cat, publisher, platform, moduleKind, status])
  // Kinds are offered when the server says what each module is; a status, whenever any isn't stable.
  const kindsHere = useMemo(() => MODULE_KINDS.filter((k) => (items || []).some((i) => itemModuleKind(i) === k)), [items])

  const publishers = useMemo(() => [...new Set((items || []).map((i) => i.publisher?.name).filter(Boolean) as string[])].sort(), [items])
  const platforms = useMemo(() => [...new Set((items || []).flatMap((i) => i.platforms || []).filter((p) => p !== 'all'))].sort(), [items])
  const categories = useMemo(() => {
    const n: Record<string, number> = {}
    for (const i of items || []) { if (!i.status?.installed && compat(i, server).hidden) continue; const c = categoryOf(i); n[c] = (n[c] || 0) + 1 }
    const list = CATEGORIES.filter((c) => n[c.id]).map((c) => ({ ...c, n: n[c.id] }))
    if (n.other) list.push({ id: 'other', title: t('Other'), n: n.other })
    return list
  }, [items, server])

  if (error && !view) return <Notice tone="danger" title={error.title} action={<Button size="sm" onClick={() => reload()}>{t('Try again')}</Button>}>{error.message}</Notice>
  if (!view) return <Spinner />

  const official = shown.filter((i) => i.official || i.kind === 'server')
  const others = shown.filter((i) => !i.official && i.kind !== 'server')
  const groups = categories.map((c) => ({ ...c, items: official.filter((i) => categoryOf(i) === c.id) })).filter((g) => g.items.length)
  const cards = (list: StoreItem[]) => <div className="st-grid">{list.map((i) => <ItemCard key={i.key} item={i} onChanged={reload} repairRoute={view.repairRoute} server={view.server} platform={platformName(view.platform)} />)}</div>
  const count = filtered ? tn('{count} module found', '{count} modules found', shown.length) : tn('{count} module', '{count} modules', shown.length)
  const clear = () => { setQ(''); setParams({}, { replace: true }) }
  const broken = view.stores.filter((s) => s.problem)
  const searchPage = storeSearchSpec({
    q, cats: categories.map((c) => ({ id: c.id, title: c.title, n: c.n })), cat,
    publishers, publisher, platforms: platforms.map((p) => ({ value: p, label: p === view.platform ? t('{platform} (this server)', { platform: platformName(p) }) : platformName(p) })), platform,
    kinds: features?.has('module-store-kinds') || kindsHere.length > 0 ? MODULE_KINDS.map((k) => ({ value: k, label: moduleKindTitle(k) })) : [],
    moduleKind, statuses: STATUSES.map((st) => ({ value: st, label: statusTitle(st) })), status,
  })
  const searchHandled = handledIds(searchPage)
  const onAction = (id: string) => { if (id.startsWith('cat:')) { const c = id.slice(4); set('cat', cat === c ? '' : c) } }
  const onField = (id: string, v: unknown) => { const x = String(v); if (id === 'q') setQ(x); else if (id === 'publisher') set('publisher', x); else if (id === 'platform') set('platform', x); else if (id === 'kind') set('moduleKind', x); else if (id === 'status') set('status', x) }

  return (
    <div className="st-browse">
      {/* Search and what it's for come first; the rest of the filters are folded away until wanted. */}
      <SpecPage embedded handled={searchHandled} page={{ ...searchPage, sections: searchPage.sections.slice(0, 2) }} onAction={onAction} onField={onField} />
      <details className="ms-more st-more-filters" open={!!(publisher || platform || moduleKind || status)}>
        <summary>{t('More filters')}{publisher || platform || moduleKind || status ? ' · ' + t('on') : ''}</summary>
        <SpecPage embedded handled={searchHandled} page={{ ...searchPage, sections: searchPage.sections.slice(2) }} onAction={onAction} onField={onField} />
      </details>

      {broken.map((s) => s.official
        ? <FailureNotice key={s.id} failure={s.problem!} name={t('The list of modules')} repairRoute={view.repairRoute} onRetry={() => void store.check()} />
        : <Notice key={s.id} tone={s.problem!.kind === 'key-changed' ? 'danger' : 'warn'} title={`${s.name} (${s.host}): ${s.problem!.title}`} action={<LinkButton size="sm" to="/settings/modules/stores">{t('See this store')}</LinkButton>}>{s.problem!.message}</Notice>
      )}
      {view.checkError && !broken.some((s) => s.official) && <Notice tone="warn" title={t('The last check didn’t work')}>{view.checkError} {t('Everything keeps working as it is.')}</Notice>}

      <div ref={results} className="st-results" role="region" aria-label={t('Modules')}>
        <p className="st-count" role="status">{count}{filtered && <> · <button type="button" className="st-link" onClick={clear}>{t('Clear search and filters')}</button></>}</p>

        {shown.length === 0 && (
          <EmptyState icon={<IconSearch />} title={t('No modules match')} text={q ? t('Nothing in the store matches “{q}”.', { q }) : t('Nothing in the store matches these filters.')}
            action={<Button onClick={clear}>{t('Clear search and filters')}</Button>} />
        )}

        {/* Browsing goes by what things are for; once someone's narrowed it down, one list reads better. */}
        {official.length > 0 && (filtered && !cat ? cards(official) : groups.map((g) => (
          <section key={g.id} id={'cat-' + g.id} className="st-shelf" aria-labelledby={'cat-h-' + g.id}>
            <h2 id={'cat-h-' + g.id}>{categoryTitle(g.id === 'other' ? undefined : g.id)}</h2>
            {cards(g.items)}
          </section>
        )))}

        {others.length > 0 && (
          <section className="st-shelf st-others" aria-labelledby="st-others-h">
            <h2 id="st-others-h"><IconShield /> {t('From other stores')}</h2>
            <p className="ms-note">{t('Nobody at this project has reviewed these. Each is labelled with the store it comes from.')} <Link to="/settings/modules/stores">{t('Manage stores')}</Link> · <HelpLink to="module-store#other-stores">{t('What to know first')}</HelpLink></p>
            {cards(others)}
          </section>
        )}
      </div>

    </div>
  )
}
