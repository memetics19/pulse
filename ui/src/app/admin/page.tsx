'use client'
import { useState, useEffect } from 'react'
import { fetchOverview, type Overview } from '@/lib/api'

const BANNER: Record<Overview['overall'], { text: string; color: string }> = {
  operational: { text: 'All systems operational', color: 'var(--up)' },
  degraded: { text: 'Partial degradation', color: 'var(--warn)' },
  outage: { text: 'Service disruption', color: 'var(--down)' },
}

function Stat({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div className="stat-card">
      <div style={{ fontSize: 24, fontWeight: 700, color: color ?? 'var(--text)' }}>{value}</div>
      <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>{label}</div>
    </div>
  )
}

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { fetchOverview().then(setO).catch(() => setErr('Could not load overview.')) }, [])

  if (err) return (<><h1 className="admin-title">Overview</h1><div className="card">{err}</div></>)
  if (!o) return <h1 className="admin-title">Overview</h1>

  const b = BANNER[o.overall]
  return (
    <>
      <h1 className="admin-title">Overview</h1>
      <div style={{ display: 'grid', gap: 16 }}>
        <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ width: 9, height: 9, borderRadius: '50%', background: b.color }} />
          <span style={{ fontWeight: 600 }}>{b.text}</span>
          <span style={{ marginLeft: 'auto', fontSize: 13, color: 'var(--text-muted)' }}>{o.total_monitors} monitors · {o.agent_count} agents</span>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12 }}>
          <Stat label="Operational" value={o.counts.up} color="var(--up)" />
          <Stat label="Degraded" value={o.counts.degraded} color="var(--warn)" />
          <Stat label="Down" value={o.counts.down} color="var(--down)" />
          <Stat label="Monitors" value={o.total_monitors} />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr', gap: 14 }}>
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 10 }}>Active incidents</div>
            {o.active_incidents.length === 0
              ? <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>No active incidents.</div>
              : o.active_incidents.map(i => (
                  <div key={i.id} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderTop: '1px solid var(--border)' }}>
                    <span style={{ fontSize: 13 }}>{i.title}</span>
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{i.status} · {i.severity}</span>
                  </div>
                ))}
          </div>
          <div className="card">
            <div style={{ fontWeight: 600, marginBottom: 10 }}>Attention</div>
            {o.attention.length === 0
              ? <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>Nothing needs attention.</div>
              : o.attention.map((a, idx) => <div key={idx} style={{ fontSize: 13, padding: '4px 0' }}>{a.message}</div>)}
          </div>
        </div>
      </div>
    </>
  )
}
