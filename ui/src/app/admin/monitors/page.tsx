'use client'
import { useState, useEffect, useCallback } from 'react'
import type { Monitor, MonitorAuthInput, MonitorGroup } from '@/lib/types'
import {
  adminListMonitors, adminListGroups, adminCreateMonitor,
  adminUpdateMonitor, adminDeleteMonitor, adminCreateGroup
} from '@/lib/api'

const TYPES = ['http', 'tcp', 'ping', 'dns', 'ssl', 'infra']
const EMPTY: Partial<Monitor> = {
  name: '', url: '', type: 'http', interval_seconds: 60, timeout_seconds: 10,
  degraded_threshold_ms: 500, down_threshold_ms: 2000, is_active: true,
  keyword_check: '', source: 'internal', external_id: '',
  method: 'GET', headers: {}, body: '', auth: null, tls_verify: true, ca_pem: '',
  follow_redirects: true, accepted_statuses: [], keyword_mode: 'contains',
}
const METHODS: Monitor['method'][] = ['GET', 'HEAD', 'POST']

// Headers are edited as one "Name: value" pair per line.
function headersToText(headers?: Record<string, string>): string {
  return Object.entries(headers ?? {}).map(([k, v]) => `${k}: ${v}`).join('\n')
}

function parseHeaders(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    const i = line.indexOf(':')
    if (i <= 0) throw new Error(`Header line "${line}" must look like Name: value`)
    out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return out
}

type AuthType = MonitorAuthInput['type']

export default function MonitorsPage() {
  const [monitors,      setMonitors]      = useState<Monitor[]>([])
  const [groups,        setGroups]        = useState<MonitorGroup[]>([])
  const [modal,         setModal]         = useState<'create' | 'edit' | null>(null)
  const [form,          setForm]          = useState<Partial<Monitor>>(EMPTY)
  const [error,         setError]         = useState('')
  const [pendingDelete, setPendingDelete] = useState<number | null>(null)
  const [groupModal,    setGroupModal]    = useState(false)
  const [groupName,     setGroupName]     = useState('')
  // HTTP option inputs that are not plain monitor fields.
  const [headersText,   setHeadersText]   = useState('')
  const [statusesText,  setStatusesText]  = useState('')
  const [authType,      setAuthType]      = useState<AuthType>('none')
  const [authUser,      setAuthUser]      = useState('')
  const [authSecret,    setAuthSecret]    = useState('')

  function openForm(m: Partial<Monitor>, mode: 'create' | 'edit') {
    setForm(m)
    setHeadersText(headersToText(m.headers))
    setStatusesText((m.accepted_statuses ?? []).join(', '))
    setAuthType(m.auth?.type ?? 'none')
    setAuthUser(m.auth?.username ?? '')
    setAuthSecret('')
    setError('')
    setModal(mode)
  }

  // The secret is write-only. An empty secret keeps the stored one.
  function authInput(): MonitorAuthInput {
    if (authType === 'none') return { type: 'none' }
    if (authType === 'bearer') return { type: 'bearer', ...(authSecret ? { token: authSecret } : {}) }
    return { type: 'basic', username: authUser, ...(authSecret ? { password: authSecret } : {}) }
  }

  const load = useCallback(() => {
    adminListMonitors().then(setMonitors).catch(() => {})
    adminListGroups().then(setGroups).catch(() => {})
  }, [])

  useEffect(() => { load() }, [load])

  async function save() {
    setError('')
    try {
      const payload = {
        ...form,
        headers: parseHeaders(headersText),
        accepted_statuses: statusesText.split(',').map(s => s.trim()).filter(Boolean),
        auth: authInput(),
      }
      if (modal === 'create') {
        const m = await adminCreateMonitor(payload)
        setMonitors(p => [...p, m])
      } else if (modal === 'edit' && form.id) {
        const m = await adminUpdateMonitor(form.id, payload)
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
        <button className="btn primary" onClick={() => openForm(EMPTY, 'create')}>+ Add Monitor</button>
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
                <button className="btn" onClick={() => openForm(m, 'edit')}>Edit</button>
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
          <div className="modal" style={{ maxHeight: '90vh', overflowY: 'auto' }}>
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
            {form.type === 'http' && (
              <details className="form-group">
                <summary className="form-label" style={{ cursor: 'pointer' }}>HTTP options</summary>
                <div className="form-group">
                  <label className="form-label">Method</label>
                  <select className="form-select" value={form.method ?? 'GET'} onChange={e => setForm(p => ({ ...p, method: e.target.value as Monitor['method'] }))}>
                    {METHODS.map(m => <option key={m} value={m}>{m}</option>)}
                  </select>
                </div>
                {form.method === 'POST' && (
                  <div className="form-group">
                    <label className="form-label">Request body</label>
                    <textarea className="form-textarea" value={form.body ?? ''} onChange={e => setForm(p => ({ ...p, body: e.target.value }))} />
                  </div>
                )}
                <div className="form-group">
                  <label className="form-label">Headers (one Name: value per line)</label>
                  <textarea className="form-textarea" value={headersText} onChange={e => setHeadersText(e.target.value)} placeholder="X-Api-Key: ..." />
                </div>
                <div className="form-group">
                  <label className="form-label">Auth</label>
                  <select className="form-select" value={authType} onChange={e => { setAuthType(e.target.value as AuthType); setAuthSecret('') }}>
                    <option value="none">None</option>
                    <option value="basic">Basic</option>
                    <option value="bearer">Bearer token</option>
                  </select>
                </div>
                {authType === 'basic' && (
                  <div className="form-group">
                    <label className="form-label">Username</label>
                    <input className="form-input" value={authUser} onChange={e => setAuthUser(e.target.value)} />
                  </div>
                )}
                {authType !== 'none' && (
                  <div className="form-group">
                    <label className="form-label">{authType === 'basic' ? 'Password' : 'Token'}</label>
                    <input
                      className="form-input"
                      type="password"
                      autoComplete="new-password"
                      value={authSecret}
                      onChange={e => setAuthSecret(e.target.value)}
                      placeholder={form.auth?.secret_set && form.auth.type === authType ? 'Saved. Leave empty to keep it.' : ''}
                    />
                  </div>
                )}
                <div className="form-group">
                  <label className="form-label">Accepted statuses (optional, e.g. 200-299, 301)</label>
                  <input className="form-input" value={statusesText} onChange={e => setStatusesText(e.target.value)} />
                </div>
                <div className="form-group">
                  <label className="form-label">Keyword mode</label>
                  <select className="form-select" value={form.keyword_mode ?? 'contains'} onChange={e => setForm(p => ({ ...p, keyword_mode: e.target.value as Monitor['keyword_mode'] }))}>
                    <option value="contains">Body must contain the keyword</option>
                    <option value="not_contains">Body must not contain the keyword</option>
                  </select>
                </div>
                <div className="form-group">
                  <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
                    <input type="checkbox" checked={form.follow_redirects ?? true} onChange={e => setForm(p => ({ ...p, follow_redirects: e.target.checked }))} />
                    <span>Follow redirects</span>
                  </label>
                </div>
                <div className="form-group">
                  <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
                    <input type="checkbox" checked={form.tls_verify ?? true} onChange={e => setForm(p => ({ ...p, tls_verify: e.target.checked }))} />
                    <span>Verify TLS certificate</span>
                  </label>
                </div>
                {(form.tls_verify ?? true) && (
                  <div className="form-group">
                    <label className="form-label">Private CA certificate (PEM, optional)</label>
                    <textarea className="form-textarea" value={form.ca_pem ?? ''} onChange={e => setForm(p => ({ ...p, ca_pem: e.target.value }))} placeholder="-----BEGIN CERTIFICATE-----" />
                  </div>
                )}
              </details>
            )}
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
