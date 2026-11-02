import { useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { IconCheck, IconCopy } from '../icons'
import { guideByFile, settingsPages, slugOf } from './guides'
import imageSizes from './imageSizes.json'
import { plain, target, type Block, type Inline } from './markdown'

// Draws a guide. Everything becomes React elements, so nothing in a guide can act as HTML, and
// only plain web addresses and other guides become links.

async function copyText(text: string): Promise<boolean> {
  // The clipboard API exists only on secure pages, and a server at home usually isn't one.
  try {
    if (navigator.clipboard) { await navigator.clipboard.writeText(text); return true }
  } catch { /* fall through */ }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  let ok = false
  try { ok = document.execCommand('copy') } catch { /* nothing more to try */ }
  ta.remove()
  return ok
}

function CodeBlock({ v }: { v: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    if (!await copyText(v)) return
    setCopied(true)
    setTimeout(() => setCopied(false), 1800)
  }
  return (
    <div className="help-code">
      <pre tabIndex={0} role="group" aria-label="Code"><code>{v}</code></pre>
      <button type="button" className="btn sm ghost help-copy" onClick={copy} aria-label={copied ? 'Copied' : 'Copy this'}>
        {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}<span>{copied ? 'Copied' : 'Copy'}</span>
      </button>
    </div>
  )
}

type Ctx = { signedIn: boolean; from: string }

function Inlines({ c, ctx }: { c: Inline[]; ctx: Ctx }): ReactNode {
  return c.map((n, i) => {
    switch (n.t) {
      case 'text': return n.v
      case 'code': return <code key={i}>{n.v}</code>
      case 'em': return <em key={i}><Inlines c={n.c} ctx={ctx} /></em>
      case 'strong': {
        // "Settings → Repair" leads to that page, for someone who can go there.
        const m = ctx.signedIn ? /^Settings → ([^→]+?)(?: →.*)?$/.exec(plain(n.c)) : null
        const page = m ? settingsPages[m[1].trim()] : undefined
        if (page) return <strong key={i}><Link to={`/settings/${page}`}><Inlines c={n.c} ctx={ctx} /></Link></strong>
        return <strong key={i}><Inlines c={n.c} ctx={ctx} /></strong>
      }
      case 'link': {
        const to = target(n.href)
        if (to?.kind === 'web') return <a key={i} href={to.href} target="_blank" rel="noreferrer noopener"><Inlines c={n.c} ctx={ctx} /><span className="sr-only"> (opens in a new tab)</span></a>
        if (to?.kind === 'here') return <Link key={i} to={`${ctx.from}#${to.hash}`}><Inlines c={n.c} ctx={ctx} /></Link>
        const g = to?.kind === 'guide' ? guideByFile(to.file) : undefined
        if (g && to?.kind === 'guide') return <Link key={i} to={`/help/${slugOf(g.file)}${to.hash ? '#' + to.hash : ''}`}><Inlines c={n.c} ctx={ctx} /></Link>
        return <span key={i}><Inlines c={n.c} ctx={ctx} /></span>
      }
    }
  })
}

function Blocks({ blocks, ctx }: { blocks: Block[]; ctx: Ctx }): ReactNode {
  return blocks.map((b, i) => {
    switch (b.t) {
      case 'heading': {
        const H = `h${Math.min(b.level, 5)}` as 'h1'
        return <H key={i} id={b.id} tabIndex={-1}><Inlines c={b.c} ctx={ctx} /></H>
      }
      case 'p': return <p key={i}><Inlines c={b.c} ctx={ctx} /></p>
      case 'hr': return <hr key={i} />
      case 'figure': {
        // Small copies, from this server, loaded only as they come into view. Their sizes keep
        // the page from jumping as they arrive, which would throw off an address with a heading.
        const size = (imageSizes as Record<string, number[]>)[b.name]
        return (
          <figure key={i} className="help-figure">
            <img src={`/help-images/${b.name}`} alt={b.alt} loading="lazy" decoding="async" width={size?.[0]} height={size?.[1]} />
            {b.caption.length > 0 && <figcaption><Inlines c={b.caption} ctx={ctx} /></figcaption>}
          </figure>
        )
      }
      case 'quote': return <blockquote key={i}><Blocks blocks={b.c} ctx={ctx} /></blockquote>
      case 'code': return <CodeBlock key={i} v={b.v} />
      case 'list': {
        const items = b.items.map((it, k) => <li key={k}><Blocks blocks={it} ctx={ctx} /></li>)
        return b.ordered ? <ol key={i} start={b.start}>{items}</ol> : <ul key={i}>{items}</ul>
      }
      case 'table': return (
        <div key={i} className="help-table" tabIndex={0} role="group" aria-label="Table">
          <table>
            <thead><tr>{b.head.map((c, k) => <th key={k} scope="col" style={{ textAlign: b.align[k] }}><Inlines c={c} ctx={ctx} /></th>)}</tr></thead>
            <tbody>{b.rows.map((r, k) => <tr key={k}>{r.map((c, n) => <td key={n} style={{ textAlign: b.align[n] }}><Inlines c={c} ctx={ctx} /></td>)}</tr>)}</tbody>
          </table>
        </div>
      )
    }
  })
}

export function GuideBody({ blocks, signedIn }: { blocks: Block[]; signedIn: boolean }) {
  const { pathname } = useLocation()
  return <><Blocks blocks={blocks} ctx={{ signedIn, from: pathname }} /></>
}
