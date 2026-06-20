'use client'
import { useState, useEffect, useCallback } from 'react'
import type { Monitor, MonitorGroup } from '@/lib/types'
import {
  adminListMonitors, adminListGroups, adminCreateMonitor,
  adminUpdateMonitor, adminDeleteMonitor, adminCreateGroup
} from '@/lib/api'

const TYPES = ['http', 'tcp', 'ping', 'dns', 'ssl', 'infra']
const EMPTY: Partial<Monitor> = {
  name: '', url: '', type: 'http', interval_seconds: 60, timeout_seconds: 10,
  degraded_threshold_ms: 500, down_threshold_ms: 2000, is_active: true,
  keyword_check: '', source: 'internal', external_id: '',
}

export default function MonitorsPage() {
  const [monitors,      setMonitors]      = useState<Monitor[]>([])
  const [groups,        setGroups]        = useState<MonitorGroup[]>([])
  const [modal,         setModal]         = useState<'create' | 'edit' | null>(null)
  const [form,          setForm]          = useState<Partial<Monitor>>(EMPTY)
  const [error,         setError]         = useState('')
  const [pendingDelete, setPendingDelete] = useState<number | null>(null)
  const [groupModal,    setGroupModal]    = useState(false)
  const [groupName,     setGroupName]     = useState('')

  const load = useCallback(() => {
    adminListMonitors().then(setMonitors).catch(() => {})
    adminListGroups().then(setGroups).catch(() => {})
  }, [])

  useEffect(() => { load() }, [load])

  async function save() {
    setError('')
    try {
      if (modal === 'create') {
        const m = await adminCreateMonitor(form)
        setMonitors(p => [...p, m])
      } else if (modal === 'edit' && form.id) {
        const m = await adminUpdateMonitor(form.id, form)
        setMonitors(p => p.map(x => x.id === m.id ? m : x))
      }
      setModal(null); setForm(EMPTY)
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  async function del(id: number) {
    await adminDeleteMonitor(id)
    setMonitors(p => p.filter(m => m.id !== id))
    setPendingDelete(null)
  }

  async function createGroup() {
    if (!groupName.trim()) return
    const g = await adminCreateGroup({ name: groupName.trim(), display_order: groups.length, description: '' })
    setGroups(p => [...p, g])
    setForm(p => ({ ...p, group_id: g.id }))
    setGroupModal(false)
    setGroupName('')
  }

  function field(key: keyof Monitor, label: string, inputType = 'text') {
    return (
      <div className="form-group">
        <label className="form-label">{label}</label>
        <input
          className="form-input"
          type={inputType}
          value={String(form[key] ?? '')}
          onChange={e => setForm(p => ({
            ...p,
            [key]: inputType === 'number' ? Number(e.target.value) : e.target.value
          }))}
        />
      </div>
    )
  }

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>Monitors</h1>
        <button className="btn primary" onClick={() => { setForm(EMPTY); setModal('create') }}>+ Add Monitor</button>
      </div>

      <table className="admin-table">
        <thead>
          <tr><th>Name</th><th>Type</th><th>URL</th><th>Interval</th><th>Active</th><th></th></tr>
        </thead>
        <tbody>
          {monitors.map(m => (
            <tr key={m.id}>
              <td>{m.name}</td>
              <td><code>{m.type}</code></td>
              <td style={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.url}</td>
              <td>{m.interval_seconds}s</td>
              <td>{m.is_active ? '✓' : '—'}</td>
              <td style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <button className="btn" onClick={() => { setForm(m); setModal('edit') }}>Edit</button>
                {pendingDelete === m.id ? (
                  <>
                    <button className="btn danger" onClick={() => del(m.id)}>Yes, delete</button>
                    <button className="btn" onClick={() => setPendingDelete(null)}>Cancel</button>
                  </>
                ) : (
                  <button className="btn danger" onClick={() => setPendingDelete(m.id)}>Delete</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {modal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setModal(null) }}>
          <div className="modal">
            <h2 className="modal-title">{modal === 'create' ? 'Add Monitor' : 'Edit Monitor'}</h2>
            {field('name', 'Name')}
            <div className="form-group">
              <label className="form-label">Type</label>
              <select className="form-select" value={form.type ?? 'http'} onChange={e => setForm(p => ({ ...p, type: e.target.value as Monitor['type'] }))}>
                {TYPES.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            </div>
            {field('url', 'URL / Host')}
            {field('interval_seconds', 'Interval (s)', 'number')}
            {field('timeout_seconds', 'Timeout (s)', 'number')}
            {field('degraded_threshold_ms', 'Degraded threshold (ms)', 'number')}
            {field('down_threshold_ms', 'Down threshold (ms)', 'number')}
            {field('keyword_check', 'Keyword check (optional)')}
            <div className="form-group">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 6 }}>
                <label className="form-label" style={{ marginBottom: 0 }}>Group</label>
                <button
                  className="btn"
                  style={{ fontSize: 11, padding: '2px 8px' }}
                  type="button"
                  onClick={() => setGroupModal(true)}
                >
                  + New group
                </button>
              </div>
              <p style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 6 }}>
                Groups organize monitors into sections on the status page (e.g. &ldquo;Core API&rdquo;, &ldquo;Database&rdquo;).
              </p>
              <select className="form-select" value={form.group_id ?? ''} onChange={e => setForm(p => ({ ...p, group_id: e.target.value ? Number(e.target.value) : null }))}>
                <option value="">No group</option>
                {groups.map(g => <option key={g.id} value={g.id}>{g.name}</option>)}
              </select>
            </div>
            {error && <div className="error-msg">{error}</div>}
            <div className="modal-footer">
              <button className="btn" onClick={() => { setModal(null) }}>Cancel</button>
              <button className="btn primary" onClick={save}>Save</button>
            </div>
          </div>
        </div>
      )}

      {groupModal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) { setGroupModal(false); setGroupName('') } }}>
          <div className="modal">
            <h2 className="modal-title">New group</h2>
            <div className="form-group">
              <label className="form-label">Name</label>
              <input
                className="form-input"
                autoFocus
                value={groupName}
                onChange={e => setGroupName(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') createGroup() }}
                placeholder="e.g. Core API"
              />
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => { setGroupModal(false); setGroupName('') }}>Cancel</button>
              <button className="btn primary" onClick={createGroup} disabled={!groupName.trim()}>Create</button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
