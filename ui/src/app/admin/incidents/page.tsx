'use client'
import { useState, useEffect, useCallback } from 'react'
import type { Incident, IncidentUpdate, Monitor } from '@/lib/types'
import {
  adminListIncidents, adminCreateIncident,
  adminUpdateIncidentStatus, adminAddIncidentUpdate,
  adminDeleteIncident, fetchIncidentUpdates, adminListMonitors
} from '@/lib/api'

const STATUSES: Incident['status'][] = ['detected', 'investigating', 'identified', 'implementing', 'monitoring', 'resolved']
const SEVERITIES: Incident['severity'][] = ['minor', 'major', 'critical']

const STATUS_LABELS: Record<string, string> = {
  detected: 'Detected',
  investigating: 'Investigating',
  identified: 'Identified',
  implementing: 'Implementing Fix',
  monitoring: 'Monitoring',
  resolved: 'Resolved',
}

const SEVERITY_DOT: Record<string, string> = {
  minor: 'var(--text-muted)',
  major: '#c0392b',
  critical: '#dc2626',
}

function mdToHtml(src: string): string {
  const esc = src.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return esc
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\n/g, '<br>')
}

function relTime(iso: string): string {
  const then = new Date(iso).getTime()
  if (isNaN(then)) return ''
  const diff = Date.now() - then
  const s = Math.round(diff / 1000)
  if (s < 60) return 'just now'
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.round(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

function parseMonitorIds(raw: string | undefined | null): number[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v.map(Number).filter(n => !isNaN(n)) : []
  } catch {
    return []
  }
}

function SeverityChip({ severity }: { severity: string }) {
  const styles: Record<string, React.CSSProperties> = {
    minor: { background: 'var(--bg3)', color: 'var(--text-muted)', border: '1px solid var(--border)' },
    major: { background: '#fdecec', color: '#c0392b', border: '1px solid #f5c6c6' },
    critical: { background: '#fbdddd', color: '#a31515', border: '1px solid #f0a8a8' },
  }
  return (
    <span style={{
      fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.5px',
      padding: '2px 8px', borderRadius: 100, ...styles[severity],
    }}>
      {severity}
    </span>
  )
}

function Stepper({ current }: { current: string }) {
  const idx = STATUSES.indexOf(current as Incident['status'])
  return (
    <div style={{ display: 'flex', alignItems: 'center', margin: '4px 0 8px' }}>
      {STATUSES.map((s, i) => {
        const isCurrent = i === idx
        const isDone = i < idx
        const dotColor = isCurrent ? 'var(--accent)' : isDone ? 'var(--text-muted)' : 'var(--border)'
        const lineColor = i <= idx ? 'var(--text-muted)' : 'var(--border)'
        return (
          <div key={s} style={{ display: 'flex', alignItems: 'center', flex: i < STATUSES.length - 1 ? 1 : '0 0 auto' }}>
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4 }}>
              <span style={{
                width: isCurrent ? 12 : 9, height: isCurrent ? 12 : 9, borderRadius: '50%',
                background: dotColor, flexShrink: 0,
                boxShadow: isCurrent ? '0 0 0 3px rgba(79,70,229,0.15)' : 'none',
              }} />
              <span style={{
                fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.3px', whiteSpace: 'nowrap',
                fontWeight: isCurrent ? 700 : 500,
                color: isCurrent ? 'var(--accent)' : isDone ? 'var(--text-muted)' : 'var(--border)',
              }}>
                {s}
              </span>
            </div>
            {i < STATUSES.length - 1 && (
              <span style={{ flex: 1, height: 1, background: lineColor, margin: '0 4px', marginBottom: 16, alignSelf: 'flex-start', marginTop: 5 }} />
            )}
          </div>
        )
      })}
    </div>
  )
}

export default function IncidentsPage() {
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [monitors, setMonitors] = useState<Monitor[]>([])
  const [selected, setSelected] = useState<Incident | null>(null)
  const [updates, setUpdates] = useState<IncidentUpdate[]>([])
  const [filter, setFilter] = useState<'active' | 'resolved' | 'all'>('active')
  const [createModal, setCreate] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null)
  const [error, setError] = useState('')

  // create form
  const [form, setForm] = useState<{ title: string; severity: string; monitorIds: number[]; firstMessage: string }>({
    title: '', severity: 'major', monitorIds: [], firstMessage: '',
  })

  // composer
  const [mode, setMode] = useState<'status' | 'comment'>('status')
  const [composeStatus, setComposeStatus] = useState<string>('investigating')
  const [composeMsg, setComposeMsg] = useState('')
  const [composeRca, setComposeRca] = useState('')
  const [previewMode, setPreviewMode] = useState(false)

  const load = useCallback(() => {
    adminListIncidents().then(setIncidents).catch(() => {})
  }, [])

  useEffect(() => { load() }, [load])
  useEffect(() => { adminListMonitors().then(setMonitors).catch(() => {}) }, [])

  const monitorName = useCallback((id: number) => monitors.find(m => m.id === id)?.name ?? `#${id}`, [monitors])

  async function selectIncident(inc: Incident) {
    setSelected(inc)
    setError('')
    setMode('status')
    setComposeStatus(inc.status)
    setComposeMsg('')
    setComposeRca(inc.rca ?? '')
    setPreviewMode(false)
    const u = await fetchIncidentUpdates(inc.id).catch(() => [] as IncidentUpdate[])
    setUpdates(u)
  }

  async function refreshSelected(id: number) {
    const all = await adminListIncidents()
    setIncidents(all)
    const inc = all.find(i => i.id === id) ?? null
    setSelected(inc)
    if (inc) {
      setComposeStatus(inc.status)
      setComposeRca(inc.rca ?? '')
      const u = await fetchIncidentUpdates(inc.id).catch(() => [] as IncidentUpdate[])
      setUpdates(u)
    }
  }

  async function createIncident() {
    setError('')
    if (!form.title.trim()) { setError('Title is required'); return }
    try {
      const created = await adminCreateIncident({
        title: form.title.trim(),
        severity: form.severity,
        affected_monitor_ids: JSON.stringify(form.monitorIds),
      })
      if (form.firstMessage.trim()) {
        await adminAddIncidentUpdate(created.id, { status: 'detected', message: form.firstMessage.trim() })
      }
      const all = await adminListIncidents()
      setIncidents(all)
      setCreate(false)
      setForm({ title: '', severity: 'major', monitorIds: [], firstMessage: '' })
      const inc = all.find(i => i.id === created.id)
      if (inc) selectIncident(inc)
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  async function postUpdate() {
    if (!selected) return
    setError('')
    if (!composeMsg.trim() && mode === 'comment') { setError('Message is required'); return }
    try {
      if (mode === 'comment') {
        await adminAddIncidentUpdate(selected.id, { status: selected.status, message: composeMsg.trim() })
      } else {
        if (composeStatus === 'resolved' && !composeRca.trim()) {
          setError('RCA is required to resolve an incident'); return
        }
        await adminUpdateIncidentStatus(selected.id, composeStatus, composeStatus === 'resolved' ? composeRca.trim() : undefined)
        if (composeMsg.trim()) {
          await adminAddIncidentUpdate(selected.id, { status: composeStatus, message: composeMsg.trim() })
        }
      }
      setComposeMsg('')
      setPreviewMode(false)
      await refreshSelected(selected.id)
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  async function doDelete(id: number) {
    await adminDeleteIncident(id)
    setIncidents(p => p.filter(x => x.id !== id))
    if (selected?.id === id) { setSelected(null); setUpdates([]) }
    setConfirmDelete(null)
  }

  const activeCount = incidents.filter(i => i.status !== 'resolved').length
  const filtered = incidents.filter(i =>
    filter === 'all' ? true : filter === 'active' ? i.status !== 'resolved' : i.status === 'resolved'
  )

  return (
    <>
      <h1 className="admin-title">Incident Management</h1>
      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: 20, alignItems: 'start' }}>
        {/* ── LEFT: list ── */}
        <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 16px', borderBottom: '1px solid var(--border)' }}>
            <span style={{ fontWeight: 600, fontSize: 14 }}>Incidents</span>
            <button className="btn primary" style={{ fontSize: 12, padding: '5px 10px' }} onClick={() => { setError(''); setCreate(true) }}>+ New incident</button>
          </div>

          {/* filter tabs */}
          <div style={{ display: 'flex', gap: 4, padding: '10px 12px', borderBottom: '1px solid var(--border)' }}>
            {([['active', `Active${activeCount ? ` (${activeCount})` : ''}`], ['resolved', 'Resolved'], ['all', 'All']] as const).map(([key, label]) => (
              <button
                key={key}
                onClick={() => setFilter(key)}
                style={{
                  flex: 1, padding: '5px 8px', fontSize: 11, fontWeight: 600, cursor: 'pointer',
                  borderRadius: 6, border: '1px solid', textTransform: 'capitalize',
                  borderColor: filter === key ? 'var(--accent)' : 'var(--border)',
                  background: filter === key ? 'var(--accent)' : 'transparent',
                  color: filter === key ? '#fff' : 'var(--text-muted)',
                }}
              >
                {label}
              </button>
            ))}
          </div>

          <div style={{ maxHeight: 'calc(100vh - 220px)', overflowY: 'auto' }}>
            {filtered.length === 0 && (
              <div style={{ padding: '24px 16px', fontSize: 12, color: 'var(--text-muted)', textAlign: 'center' }}>
                No {filter === 'all' ? '' : filter} incidents.
              </div>
            )}
            {filtered.map(i => {
              const isSel = selected?.id === i.id
              return (
                <div
                  key={i.id}
                  onClick={() => selectIncident(i)}
                  style={{
                    cursor: 'pointer', padding: '12px 14px', borderBottom: '1px solid var(--border)',
                    background: isSel ? 'var(--bg2)' : 'transparent',
                    boxShadow: isSel ? 'inset 3px 0 0 var(--accent)' : 'none',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}>
                    <span style={{ width: 8, height: 8, borderRadius: '50%', background: SEVERITY_DOT[i.severity], flexShrink: 0, marginTop: 5 }} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4, flexWrap: 'wrap' }}>
                        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{i.title}</span>
                        <SeverityChip severity={i.severity} />
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 11, color: 'var(--text-muted)' }}>
                        <span style={{ textTransform: 'uppercase', letterSpacing: '0.3px', fontWeight: 600 }}>{STATUS_LABELS[i.status] ?? i.status}</span>
                        <span>·</span>
                        <span>{relTime(i.started_at)}</span>
                      </div>
                    </div>
                    {confirmDelete === i.id ? (
                      <div style={{ display: 'flex', gap: 4, flexShrink: 0 }} onClick={e => e.stopPropagation()}>
                        <button className="btn danger" style={{ fontSize: 10, padding: '2px 7px' }} onClick={() => doDelete(i.id)}>Confirm</button>
                        <button className="btn" style={{ fontSize: 10, padding: '2px 7px' }} onClick={() => setConfirmDelete(null)}>Cancel</button>
                      </div>
                    ) : (
                      <button
                        style={{ flexShrink: 0, border: 'none', background: 'transparent', color: 'var(--text-muted)', cursor: 'pointer', fontSize: 11, padding: '0 2px' }}
                        title="Delete incident"
                        onClick={e => { e.stopPropagation(); setConfirmDelete(i.id) }}
                      >
                        ✕
                      </button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </div>

        {/* ── RIGHT: detail ── */}
        <div>
          {!selected ? (
            <div className="card" style={{ textAlign: 'center', color: 'var(--text-muted)', fontSize: 13, padding: '60px 20px' }}>
              Select an incident to manage it.
            </div>
          ) : (
            <div style={{ display: 'grid', gap: 16 }}>
              {/* header */}
              <div className="card">
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12, flexWrap: 'wrap' }}>
                  <h2 style={{ fontSize: 17, fontWeight: 700, color: 'var(--text)' }}>{selected.title}</h2>
                  <SeverityChip severity={selected.severity} />
                  <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--text-muted)' }}>Started {relTime(selected.started_at)}</span>
                </div>

                {/* affected monitors */}
                {parseMonitorIds(selected.affected_monitor_ids).length > 0 && (
                  <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 14 }}>
                    {parseMonitorIds(selected.affected_monitor_ids).map(id => (
                      <span key={id} style={{
                        fontSize: 11, padding: '2px 9px', borderRadius: 100,
                        background: 'var(--bg2)', border: '1px solid var(--border)', color: 'var(--text-muted)',
                      }}>
                        {monitorName(id)}
                      </span>
                    ))}
                  </div>
                )}

                {/* stepper */}
                <Stepper current={selected.status} />

                {selected.rca && (
                  <div className="rca-block">
                    <div className="rca-label">Root cause analysis</div>
                    <div className="rca-text" dangerouslySetInnerHTML={{ __html: mdToHtml(selected.rca) }} />
                  </div>
                )}
              </div>

              {/* timeline */}
              <div className="card">
                <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 8 }}>Timeline</div>
                {updates.length === 0 ? (
                  <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>No updates yet.</div>
                ) : (
                  updates.map(u => (
                    <div key={u.id} className="tl-row">
                      <span className={`tl-dot ${u.status === 'resolved' ? 'done' : u.status === 'detected' ? 'start' : 'active'}`} />
                      <div className="tl-time">{relTime(u.created_at)}</div>
                      <div style={{ flex: 1 }}>
                        <div className={`tl-status ${u.status}`}>{STATUS_LABELS[u.status] ?? u.status}</div>
                        <div className="tl-msg" style={{ color: 'var(--text)' }} dangerouslySetInnerHTML={{ __html: mdToHtml(u.message) }} />
                      </div>
                    </div>
                  ))
                )}
              </div>

              {/* composer */}
              <div className="card">
                <div style={{ display: 'flex', gap: 4, marginBottom: 14 }}>
                  {(['status', 'comment'] as const).map(m => (
                    <button
                      key={m}
                      onClick={() => setMode(m)}
                      style={{
                        padding: '5px 12px', fontSize: 12, fontWeight: 600, cursor: 'pointer', borderRadius: 6,
                        border: '1px solid', textTransform: 'capitalize',
                        borderColor: mode === m ? 'var(--accent)' : 'var(--border)',
                        background: mode === m ? 'var(--accent)' : 'transparent',
                        color: mode === m ? '#fff' : 'var(--text-muted)',
                      }}
                    >
                      {m === 'status' ? 'Status update' : 'Comment only'}
                    </button>
                  ))}
                </div>

                {mode === 'status' && (
                  <div className="form-group">
                    <label className="form-label">New status</label>
                    <select className="form-select" value={composeStatus} onChange={e => setComposeStatus(e.target.value)}>
                      {STATUSES.map(s => <option key={s} value={s}>{STATUS_LABELS[s]}</option>)}
                    </select>
                  </div>
                )}

                <div className="form-group">
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                    <label className="form-label" style={{ marginBottom: 0 }}>{mode === 'comment' ? 'Comment' : 'Update message'}</label>
                    <div style={{ display: 'flex', gap: 4 }}>
                      {(['Write', 'Preview'] as const).map(t => {
                        const isPreview = t === 'Preview'
                        const on = previewMode === isPreview
                        return (
                          <button
                            key={t}
                            onClick={() => setPreviewMode(isPreview)}
                            style={{
                              padding: '2px 9px', fontSize: 11, fontWeight: 600, cursor: 'pointer', borderRadius: 5,
                              border: '1px solid', borderColor: on ? 'var(--accent)' : 'var(--border)',
                              background: on ? 'var(--bg2)' : 'transparent',
                              color: on ? 'var(--accent)' : 'var(--text-muted)',
                            }}
                          >
                            {t}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                  {previewMode ? (
                    <div
                      style={{ minHeight: 80, padding: '8px 12px', border: '1px solid var(--border)', borderRadius: 8, fontSize: 13, color: 'var(--text)', background: 'var(--bg2)' }}
                      dangerouslySetInnerHTML={{ __html: composeMsg.trim() ? mdToHtml(composeMsg) : '<span style="color:var(--text-muted)">Nothing to preview</span>' }}
                    />
                  ) : (
                    <textarea
                      className="form-textarea"
                      value={composeMsg}
                      onChange={e => setComposeMsg(e.target.value)}
                      placeholder="Markdown supported — **bold**, *italic*, `code`, [link](https://…)"
                    />
                  )}
                </div>

                {mode === 'status' && composeStatus === 'resolved' && (
                  <div className="form-group">
                    <label className="form-label">Root cause analysis <span style={{ color: 'var(--down)' }}>*required to resolve</span></label>
                    <textarea
                      className="form-textarea"
                      style={{ minHeight: 90 }}
                      value={composeRca}
                      onChange={e => setComposeRca(e.target.value)}
                      placeholder="What caused this and how was it fixed?"
                    />
                  </div>
                )}

                {error && <div className="error-msg">{error}</div>}
                <button className="btn primary" onClick={postUpdate}>Post update</button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ── New incident modal ── */}
      {createModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setCreate(false) }}>
          <div className="modal">
            <h2 className="modal-title">New incident</h2>
            <div className="form-group">
              <label className="form-label">Title</label>
              <input className="form-input" value={form.title} autoFocus onChange={e => setForm(p => ({ ...p, title: e.target.value }))} placeholder="Brief summary of the issue" />
            </div>
            <div className="form-group">
              <label className="form-label">Severity</label>
              <select className="form-select" value={form.severity} onChange={e => setForm(p => ({ ...p, severity: e.target.value }))}>
                {SEVERITIES.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Affected monitors</label>
              <div style={{ maxHeight: 160, overflowY: 'auto', border: '1px solid var(--border)', borderRadius: 8, padding: '6px 4px' }}>
                {monitors.length === 0 ? (
                  <div style={{ fontSize: 12, color: 'var(--text-muted)', padding: '6px 10px' }}>No monitors available.</div>
                ) : (
                  monitors.map(m => {
                    const checked = form.monitorIds.includes(m.id)
                    return (
                      <label key={m.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '5px 10px', fontSize: 13, cursor: 'pointer', borderRadius: 6 }}>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => setForm(p => ({
                            ...p,
                            monitorIds: checked ? p.monitorIds.filter(id => id !== m.id) : [...p.monitorIds, m.id],
                          }))}
                        />
                        {m.name}
                      </label>
                    )
                  })
                )}
              </div>
            </div>
            <div className="form-group">
              <label className="form-label">First update <span style={{ color: 'var(--text-muted)', textTransform: 'none', letterSpacing: 0 }}>(optional)</span></label>
              <textarea
                className="form-textarea"
                value={form.firstMessage}
                onChange={e => setForm(p => ({ ...p, firstMessage: e.target.value }))}
                placeholder="Initial status note posted as the first timeline update…"
              />
            </div>
            {error && <div className="error-msg">{error}</div>}
            <div className="modal-footer">
              <button className="btn" onClick={() => setCreate(false)}>Cancel</button>
              <button className="btn primary" onClick={createIncident}>Create incident</button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
