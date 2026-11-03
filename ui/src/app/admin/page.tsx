'use client'
import { useState, useEffect } from 'react'
import Link from 'next/link'
import { fetchOverview, type Overview } from '@/lib/api'

const BANNER: Record<Overview['overall'], { text: string; color: string }> = {
  operational: { text: 'All systems operational', color: 'var(--up)' },
  degraded: { text: 'Partial degradation', color: 'var(--warn)' },
  outage: { text: 'Service disruption', color: 'var(--down)' },
}

const SEVERITY_COLOR: Record<string, string> = { minor: 'var(--warn)', major: 'var(--down)', critical: 'var(--down)' }

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    const load = () => fetchOverview().then(v => { setO(v); setErr('') }).catch(() => setErr('Could not load the overview. Check that Pulse is running and reload the page.'))
    load()
    // Keep the overview current while it is open.
    const id = setInterval(() => { if (document.visibilityState === 'visible') load() }, 30_000)
    return () => clearInterval(id)
  }, [])

  if (err && !o) return (<><h1 className="admin-title">Overview</h1><div className="callout" role="alert">{err}</div></>)
  if (!o) return (
    <>
      <h1 className="admin-title">Overview</h1>
      <div style={{ display: 'grid', gap: 12 }} aria-busy="true">
        <div className="skeleton" style={{ height: 64 }} />
        <div className="skeleton" style={{ height: 120 }} />
      </div>
    </>
  )

  const b = BANNER[o.overall]
  const parts = [
    { n: o.counts.up, label: 'up', color: 'var(--up)' },
    { n: o.counts.degraded, label: 'degraded', color: 'var(--warn)' },
    { n: o.counts.down, label: 'down', color: 'var(--down)' },
  ]
  return (
    <>
      <h1 className="admin-title">Overview</h1>
      <div style={{ display: 'grid', gap: 14 }}>
        <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          <span className={`state ${o.overall === 'operational' ? 'up' : o.overall === 'degraded' ? 'degraded' : 'down'}`} style={{ fontSize: 15, fontWeight: 600 }}>
            <span className="state-dot" aria-hidden="true" />{b.text}
          </span>
          <span className="num" style={{ fontSize: 13, color: 'var(--text-muted)' }}>
            {parts.map((p, i) => (
              <span key={p.label}>{i > 0 && ' · '}<span style={{ color: p.n ? p.color : undefined, fontWeight: p.n ? 600 : undefined }}>{p.n}</span> {p.label}</span>
            ))}
            {' '}of {o.total_monitors} {o.total_monitors === 1 ? 'monitor' : 'monitors'}
            {o.agent_count > 0 && ` · ${o.agent_count} ${o.agent_count === 1 ? 'agent' : 'agents'}`}
          </span>
          <span style={{ marginLeft: 'auto', display: 'flex', gap: 14 }}>
            <Link className="text-link" href="/admin/monitors">Monitors</Link>
            <a className="text-link" href="/" target="_blank" rel="noopener">Status page ↗</a>
          </span>
        </div>

        {o.total_monitors === 0 && (
          <div className="empty-state">
            <h2>Start by adding a monitor</h2>
            <p>Pulse checks your services on a schedule and shows the results on the public status page. Add a monitor to see it here.</p>
            <Link className="btn primary" href="/admin/monitors">Go to monitors</Link>
          </div>
        )}

        <div className="overview-grid">
          <div className="card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
              <span style={{ fontWeight: 600 }}>Active incidents</span>
              <Link className="text-link" style={{ fontSize: 12 }} href="/admin/incidents">All incidents</Link>
            </div>
            {o.active_incidents.length === 0
              ? <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>No active incidents. Incidents open on their own when a monitor keeps failing, or you can post one.</div>
              : o.active_incidents.map(i => (
                  <Link key={i.id} className="list-link" href="/admin/incidents">
                    <span>{i.title}</span>
                    <span style={{ fontSize: 12, color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                      {i.status} · <span style={{ color: SEVERITY_COLOR[i.severity] }}>{i.severity}</span>
                    </span>
                  </Link>
                ))}
          </div>
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 8 }}>Needs attention</div>
            {o.attention.length === 0
              ? <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>Nothing needs attention.</div>
              : o.attention.map((a, idx) => (
                  <Link key={idx} className="list-link" href={a.kind === 'agent_offline' ? '/admin/agents' : '/admin/monitors'}>
                    <span className={`state ${a.kind === 'down' ? 'down' : 'degraded'}`} style={{ fontWeight: 400, color: 'var(--text)' }}>
                      <span className="state-dot" aria-hidden="true" />{a.message}
                    </span>
                    <span aria-hidden="true" style={{ color: 'var(--text-muted)' }}>→</span>
                  </Link>
                ))}
          </div>
        </div>
      </div>
    </>
  )
}
