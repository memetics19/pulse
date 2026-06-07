'use client'
import { useState, useEffect, useCallback } from 'react'
import type { Monitor } from '@/lib/types'
import {
  listMaintenance, createMaintenance, updateMaintenanceStatus, deleteMaintenance,
  adminListMonitors, type Maintenance,
} from '@/lib/api'

// ── Status chip ──────────────────────────────────────────────────────────────

function StatusChip({ status }: { status: string }) {
  const styles: Record<string, React.CSSProperties> = {
    scheduled: { background: '#eef2ff', color: '#4338ca', border: '1px solid #c7d2fe' },
    in_progress: { background: '#fffbeb', color: '#b45309', border: '1px solid #fde68a' },
    completed: { background: 'var(--bg3)', color: 'var(--text-muted)', border: '1px solid var(--border)' },
    cancelled: { background: 'var(--bg3)', color: 'var(--text-muted)', border: '1px solid var(--border)' },
  }
  const labels: Record<string, string> = {
    scheduled: 'Scheduled',
    in_progress: 'In Progress',
    completed: 'Completed',
    cancelled: 'Cancelled',
  }
  const style = styles[status] ?? styles.scheduled
  return (
    <span style={{
      fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.5px',
      padding: '2px 8px', borderRadius: 100, ...style,
    }}>
      {labels[status] ?? status}
    </span>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtRange(starts_at: string, ends_at: string): string {
  const s = new Date(starts_at)
  const e = new Date(ends_at)
  if (isNaN(s.getTime()) || isNaN(e.getTime())) return '—'
  return `${s.toLocaleString()} – ${e.toLocaleString()}`
}

function isTerminal(status: string): boolean {
  return status === 'completed' || status === 'cancelled'
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function MaintenancePage() {
  const [windows, setWindows] = useState<Maintenance[]>([])
  const [monitors, setMonitors] = useState<Monitor[]>([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null)
  const [error, setError] = useState('')

  // form state
  const [form, setForm] = useState({
    title: '',
    description: '',
    monitorIds: [] as number[],
    startNow: false,
    startsAt: '',
    endsAt: '',
  })
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState('')

  const load = useCallback(async () => {
    try {
      const data = await listMaintenance()
      setWindows(data)
    } catch {
      // silently keep existing list
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])
  useEffect(() => { adminListMonitors().then(setMonitors).catch(() => {}) }, [])

  function openModal() {
    setForm({ title: '', description: '', monitorIds: [], startNow: false, startsAt: '', endsAt: '' })
    setFormError('')
    setShowModal(true)
  }

  function closeModal() {
    setShowModal(false)
    setFormError('')
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    setFormError('')
    if (!form.title.trim()) { setFormError('Title is required.'); return }
    if (!form.endsAt) { setFormError('End time is required.'); return }
    if (!form.startNow && !form.startsAt) { setFormError('Start time is required when "Start now" is unchecked.'); return }

    setSubmitting(true)
    try {
      const startsAt = form.startNow ? new Date().toISOString() : new Date(form.startsAt).toISOString()
      const endsAt = new Date(form.endsAt).toISOString()
      await createMaintenance({
        title: form.title.trim(),
        description: form.description.trim(),
        affected_monitor_ids: form.monitorIds,
        starts_at: startsAt,
        ends_at: endsAt,
        start_now: form.startNow,
      })
      closeModal()
      load()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleCancel(id: number) {
    setError('')
    try {
      await updateMaintenanceStatus(id, 'cancelled')
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  async function handleDelete(id: number) {
    setError('')
    try {
      await deleteMaintenance(id)
      setWindows(prev => prev.filter(w => w.id !== id))
      setConfirmDelete(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  function toggleMonitor(id: number) {
    setForm(prev => ({
      ...prev,
      monitorIds: prev.monitorIds.includes(id)
        ? prev.monitorIds.filter(m => m !== id)
        : [...prev.monitorIds, id],
    }))
  }

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 18 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>Maintenance</h1>
        <button className="btn primary" onClick={openModal}>New maintenance</button>
      </div>

      {error && <div className="error-msg" style={{ marginBottom: 14 }}>{error}</div>}

      {loading ? (
        <div className="card" style={{ textAlign: 'center', color: 'var(--text-muted)', fontSize: 13, padding: '48px 20px' }}>
          Loading…
        </div>
      ) : windows.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', color: 'var(--text-muted)', fontSize: 13, padding: '48px 20px' }}>
          No maintenance windows.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {windows.map(w => (
            <div key={w.id} className="card" style={{ display: 'flex', alignItems: 'flex-start', gap: 14 }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 4 }}>
                  <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>{w.title}</span>
                  <StatusChip status={w.status} />
                </div>
                {w.description && (
                  <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 6, lineHeight: 1.5 }}>
                    {w.description}
                  </div>
                )}
                <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 4 }}>
                  {fmtRange(w.starts_at, w.ends_at)}
                </div>
                {w.affected_monitor_ids.length > 0 && (
                  <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', marginTop: 4 }}>
                    {w.affected_monitor_ids.map(id => {
                      const mon = monitors.find(m => m.id === id)
                      return (
                        <span key={id} style={{
                          fontSize: 11, padding: '1px 8px', borderRadius: 100,
                          background: 'var(--bg2)', border: '1px solid var(--border)', color: 'var(--text-muted)',
                        }}>
                          {mon?.name ?? `#${id}`}
                        </span>
                      )
                    })}
                    <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                      {w.affected_monitor_ids.length} monitor{w.affected_monitor_ids.length !== 1 ? 's' : ''}
                    </span>
                  </div>
                )}
              </div>

              {/* Actions */}
              <div style={{ display: 'flex', gap: 6, flexShrink: 0, alignItems: 'center' }}>
                {!isTerminal(w.status) && (
                  <button
                    className="btn"
                    style={{ fontSize: 12, padding: '5px 10px' }}
                    onClick={() => handleCancel(w.id)}
                  >
                    Cancel
                  </button>
                )}
                {confirmDelete === w.id ? (
                  <>
                    <button
                      className="btn danger"
                      style={{ fontSize: 12, padding: '5px 10px' }}
                      onClick={() => handleDelete(w.id)}
                    >
                      Confirm delete
                    </button>
                    <button
                      className="btn"
                      style={{ fontSize: 12, padding: '5px 10px' }}
                      onClick={() => setConfirmDelete(null)}
                    >
                      Keep
                    </button>
                  </>
                ) : (
                  <button
                    className="btn danger"
                    style={{ fontSize: 12, padding: '5px 10px' }}
                    onClick={() => setConfirmDelete(w.id)}
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* ── New maintenance modal ── */}
      {showModal && (
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) closeModal() }}
        >
          <div className="modal" style={{ maxHeight: '90vh', overflowY: 'auto' }}>
            <h2 className="modal-title">New maintenance window</h2>

            <form onSubmit={handleCreate}>
              <div className="form-group">
                <label className="form-label">Title</label>
                <input
                  className="form-input"
                  autoFocus
                  placeholder="Brief description of the maintenance"
                  value={form.title}
                  onChange={e => setForm(p => ({ ...p, title: e.target.value }))}
                />
              </div>

              <div className="form-group">
                <label className="form-label">Description <span style={{ color: 'var(--text-muted)', textTransform: 'none', letterSpacing: 0, fontWeight: 400 }}>(optional)</span></label>
                <textarea
                  className="form-textarea"
                  placeholder="Additional details about the maintenance…"
                  value={form.description}
                  onChange={e => setForm(p => ({ ...p, description: e.target.value }))}
                />
              </div>

              <div className="form-group">
                <label className="form-label">Affected monitors</label>
                <div style={{
                  maxHeight: 150, overflowY: 'auto',
                  border: '1px solid var(--border)', borderRadius: 8, padding: '6px 4px',
                }}>
                  {monitors.length === 0 ? (
                    <div style={{ fontSize: 12, color: 'var(--text-muted)', padding: '6px 10px' }}>No monitors available.</div>
                  ) : (
                    monitors.map(m => (
                      <label key={m.id} style={{
                        display: 'flex', alignItems: 'center', gap: 8,
                        padding: '5px 10px', fontSize: 13, cursor: 'pointer', borderRadius: 6,
                      }}>
                        <input
                          type="checkbox"
                          checked={form.monitorIds.includes(m.id)}
                          onChange={() => toggleMonitor(m.id)}
                        />
                        {m.name}
                      </label>
                    ))
                  )}
                </div>
              </div>

              <div className="form-group" style={{ marginBottom: 10 }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
                  <input
                    type="checkbox"
                    checked={form.startNow}
                    onChange={e => setForm(p => ({ ...p, startNow: e.target.checked, startsAt: e.target.checked ? '' : p.startsAt }))}
                  />
                  <span style={{ fontWeight: 500 }}>Start now</span>
                </label>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <div className="form-group">
                  <label className="form-label">Start time</label>
                  <input
                    className="form-input"
                    type="datetime-local"
                    disabled={form.startNow}
                    value={form.startsAt}
                    onChange={e => setForm(p => ({ ...p, startsAt: e.target.value }))}
                    style={{ opacity: form.startNow ? 0.45 : 1 }}
                  />
                </div>
                <div className="form-group">
                  <label className="form-label">End time</label>
                  <input
                    className="form-input"
                    type="datetime-local"
                    value={form.endsAt}
                    onChange={e => setForm(p => ({ ...p, endsAt: e.target.value }))}
                  />
                </div>
              </div>

              {formError && <div className="error-msg" style={{ marginBottom: 12 }}>{formError}</div>}

              <div className="modal-footer">
                <button type="button" className="btn" onClick={closeModal} disabled={submitting}>Cancel</button>
                <button type="submit" className="btn primary" disabled={submitting}>
                  {submitting ? 'Scheduling…' : 'Schedule maintenance'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
