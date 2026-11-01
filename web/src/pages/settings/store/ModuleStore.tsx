import { useEffect, useMemo, useRef, useState } from 'react'
import { HelpLink } from '../../../components/HelpLink'
import { Link, useSearchParams } from 'react-router-dom'
import { IconRefresh, IconSearch, IconShield } from '../../../icons'
import { toast } from '../../../state'
import { Button, Chip, Chips, EmptyState, Input, LinkButton, Notice, Select, Spinner } from '../../../ui'
import { PageHead } from '../ui'
import { ItemCard } from './ItemCard'
import { FailureDialog, FailureNotice, UpdateAllPanel, useStoreView } from './parts'
import { CATEGORIES, categoryOf, categoryTitle, checkAll, failureOf, matches, platformName, rank, showsRun, type Failure, type StoreItem } from './storeApi'

// The store: find a module by searching or by what it's for, and see at a glance what's installed
// and what can be updated. The filters live in the address, so Back returns to the same results
// and a filtered view can be shared with another admin.

export default function ModuleStore() {
  const { view, setView, error, reload } = useStoreView()
  const [params, setParams] = useSearchParams()
  const [checking, setChecking] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
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
  const show = params.get('show') || ''
  const publisher = params.get('publisher') || ''
  const platform = params.get('platform') || ''
  const set = (k: string, v: string) => setParams((p) => { const n = new URLSearchParams(p); if (v) n.set(k, v); else n.delete(k); return n }, { replace: true })
  const filtered = !!(q || cat || show || publisher || platform)

  const items = view?.items
  const shown = useMemo(() => (items || []).filter((i) =>
    matches(i, q) &&
    (!cat || categoryOf(i) === cat) &&
    (show !== 'installed' || i.status?.installed) &&
    (show !== 'updates' || (i.status?.installed && i.status.latest)) &&
    (!publisher || i.publisher?.name === publisher) &&
    (!platform || i.platforms?.includes(platform) || i.platforms?.includes('all'))
  ).sort((a, b) => rank(a, q) - rank(b, q) || a.name.localeCompare(b.name)), [items, q, cat, show, publisher, platform])

  const publishers = useMemo(() => [...new Set((items || []).map((i) => i.publisher?.name).filter(Boolean) as string[])].sort(), [items])
  const platforms = useMemo(() => [...new Set((items || []).flatMap((i) => i.platforms || []).filter((p) => p !== 'all'))].sort(), [items])
  const categories = useMemo(() => {
    const n: Record<string, number> = {}
    for (const i of items || []) { const c = categoryOf(i); n[c] = (n[c] || 0) + 1 }
    const list = CATEGORIES.filter((c) => n[c.id]).map((c) => ({ ...c, n: n[c.id] }))
    if (n.other) list.push({ id: 'other', title: 'Other', n: n.other })
    return list
  }, [items])

  const check = async () => {
    if (checking) return
    setChecking(true)
    try {
      const v = await checkAll()
      setView(v)
      if (v.checkError || v.stores.some((s) => s.problem)) toast('The check didn’t all work. What happened is on the page.')
      else toast(v.updates ? `${v.updates} ${v.updates === 1 ? 'update' : 'updates'} found` : 'Every module is up to date')
    } catch (e) { setFailure(failureOf(e, 'The check')) } finally { setChecking(false) }
  }

  const head = (
    <PageHead title="Module store" desc="Add to what this server can do. Everything here is signed, and nothing runs until you choose it."
      actions={<>
        <LinkButton size="sm" variant="ghost" to="/settings/modules">Installed modules</LinkButton>
        <LinkButton size="sm" variant="ghost" to="/settings/modules/stores" icon={<IconShield />}>Other stores</LinkButton>
      </>} />
  )
  if (error && !view) return <div className="ms-page st-page">{head}<Notice tone="danger" title={error.title} action={<Button size="sm" onClick={() => reload()}>Try again</Button>}>{error.message}</Notice></div>
  if (!view) return <div className="ms-page st-page">{head}<Spinner /></div>

  const official = shown.filter((i) => i.official || i.kind === 'server')
  const others = shown.filter((i) => !i.official && i.kind !== 'server')
  const groups = categories.map((c) => ({ ...c, items: official.filter((i) => categoryOf(i) === c.id) })).filter((g) => g.items.length)
  const cards = (list: StoreItem[]) => <div className="st-grid">{list.map((i) => <ItemCard key={i.key} item={i} onChanged={reload} repairRoute={view.repairRoute} />)}</div>
  const count = `${shown.length} ${shown.length === 1 ? 'module' : 'modules'}`
  const clear = () => { setQ(''); setParams({}, { replace: true }) }
  const broken = view.stores.filter((s) => s.problem)

  return (
    <div className="ms-page st-page">
      {head}

      <section className="st-search" role="search" aria-label="Find a module">
        <div className="st-search-box">
          <IconSearch />
          <Input type="search" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search modules by name, description or category" placeholder="Search modules" autoComplete="off" spellCheck={false} />
        </div>
        <Chips label="Category" className="st-cats">
          <Chip selected={!cat} onClick={() => set('cat', '')}>All</Chip>
          {categories.map((c) => <Chip key={c.id} selected={cat === c.id} onClick={() => set('cat', cat === c.id ? '' : c.id)}>{c.title} <span className="st-chip-n">{c.n}</span></Chip>)}
        </Chips>
        <div className="st-filters">
          <Chips label="Show">
            <Chip selected={show === 'installed'} onClick={() => set('show', show === 'installed' ? '' : 'installed')}>Installed</Chip>
            <Chip selected={show === 'updates'} onClick={() => set('show', show === 'updates' ? '' : 'updates')}>Updates available{view.updates ? ` · ${view.updates}` : ''}</Chip>
          </Chips>
          <label className="st-filter">
            <span>Publisher</span>
            <Select inputSize="sm" value={publisher} onChange={(e) => set('publisher', e.target.value)} options={[{ value: '', label: 'Anyone' }, ...publishers.map((p) => ({ value: p, label: p }))]} />
          </label>
          <label className="st-filter">
            <span>Platform</span>
            <Select inputSize="sm" value={platform} onChange={(e) => set('platform', e.target.value)} options={[{ value: '', label: 'Any' }, ...platforms.map((p) => ({ value: p, label: p === view.platform ? `${platformName(p)} (this server)` : platformName(p) }))]} />
          </label>
          <Button size="sm" variant="ghost" className="st-check" icon={<IconRefresh />} aria-busy={checking} aria-label="Check for updates now" onClick={check}>{checking ? 'Checking…' : 'Check now'}</Button>
        </div>
      </section>

      {broken.map((s) => s.official
        ? <FailureNotice key={s.id} failure={s.problem!} name="The list of modules" repairRoute={view.repairRoute} onRetry={check} />
        : <Notice key={s.id} tone={s.problem!.kind === 'key-changed' ? 'danger' : 'warn'} title={`${s.name} (${s.host}): ${s.problem!.title}`} action={<LinkButton size="sm" to="/settings/modules/stores">See this store</LinkButton>}>{s.problem!.message}</Notice>
      )}
      {view.checkError && !broken.some((s) => s.official) && <Notice tone="warn" title="The last check didn’t work">{view.checkError} Everything keeps working as it is.</Notice>}

      {(view.updates > 0 || showsRun(view.run)) && <UpdateAllPanel view={view} onChanged={reload} />}

      <div ref={results} className="st-results" role="region" aria-label="Modules">
        <p className="st-count" role="status">{filtered ? `${count} found` : count}{filtered && <> · <button type="button" className="st-link" onClick={clear}>Clear search and filters</button></>}</p>

        {shown.length === 0 && (
          <EmptyState icon={<IconSearch />} title="No modules match" text={q ? `Nothing in the store matches “${q}”.` : 'Nothing in the store matches these filters.'}
            action={<Button onClick={clear}>Clear search and filters</Button>} />
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
            <h2 id="st-others-h"><IconShield /> From other stores</h2>
            <p className="ms-note">Nobody at this project has reviewed these. Each is labelled with the store it comes from. <Link to="/settings/modules/stores">Manage stores</Link> · <HelpLink to="module-store#other-stores">What to know first</HelpLink></p>
            {cards(others)}
          </section>
        )}
      </div>

      {failure && <FailureDialog failure={failure} name="The store" repairRoute={view.repairRoute} onClose={() => setFailure(null)} />}
    </div>
  )
}
