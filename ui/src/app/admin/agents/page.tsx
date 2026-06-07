'use client'
import { useState, useEffect } from 'react'
import type { InfraAgent } from '@/lib/types'
import { adminListAgents, adminCreateAgent, adminDeleteAgent } from '@/lib/api'

export default function AgentsPage() {
  const [agents,        setAgents]        = useState<InfraAgent[]>([])
  const [modal,         setModal]         = useState(false)
  const [form,          setForm]          = useState({ name: '', host_label: '' })
  const [newAgent,      setNewAgent]      = useState<InfraAgent | null>(null)
  const [error,         setError]         = useState('')
  const [pendingDelete, setPendingDelete] = useState<number | null>(null)

  useEffect(() => { adminListAgents().then(setAgents).catch(() => {}) }, [])

  async function create() {
    setError('')
    try {
      const a = await adminCreateAgent(form)
      setAgents(p => [...p, a])
      setNewAgent(a)
      setModal(false)
      setForm({ name: '', host_label: '' })
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  async function del(id: number) {
    await adminDeleteAgent(id)
    setAgents(p => p.filter(a => a.id !== id))
    setPendingDelete(null)
  }

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>Infra Agents</h1>
        <button className="btn primary" onClick={() => { setModal(true); setNewAgent(null) }}>+ Register Agent</button>
      </div>

      {newAgent && (
        <div style={{ background: 'var(--up-bg)', border: '1px solid var(--up-border)', borderRadius: 8, padding: 16, marginBottom: 20 }}>
          <div style={{ fontWeight: 600, marginBottom: 8, color: 'var(--up)' }}>Agent registered! Copy the token &mdash; it won&apos;t be shown again.</div>
          <code style={{ fontFamily: 'var(--font-mono)', fontSize: 12, wordBreak: 'break-all', display: 'block' }}>{newAgent.token}</code>
          <div style={{ marginTop: 10, fontSize: 12, color: 'var(--text-muted)' }}>
            Run: <code>pulse-agent --server https://your-domain --token {newAgent.token}</code>
          </div>
        </div>
      )}

      <table className="admin-table">
        <thead><tr><th>Name</th><th>Host</th><th>Last Seen</th><th>Active</th><th></th></tr></thead>
        <tbody>
          {agents.map(a => (
            <tr key={a.id}>
              <td>{a.name}</td>
              <td><code>{a.host_label}</code></td>
              <td>{a.last_seen_at ? new Date(a.last_seen_at).toLocaleString() : 'Never'}</td>
              <td>{a.is_active ? '✓' : '—'}</td>
              <td style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                {pendingDelete === a.id ? (
                  <>
                    <button className="btn danger" onClick={() => del(a.id)}>Yes, delete</button>
                    <button className="btn" onClick={() => setPendingDelete(null)}>Cancel</button>
                  </>
                ) : (
                  <button className="btn danger" onClick={() => setPendingDelete(a.id)}>Delete</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {modal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setModal(false) }}>
          <div className="modal">
            <h2 className="modal-title">Register Agent</h2>
            <div className="form-group">
              <label className="form-label">Name</label>
              <input className="form-input" value={form.name} onChange={e => setForm(p => ({ ...p, name: e.target.value }))} placeholder="prod-node-01" />
            </div>
            <div className="form-group">
              <label className="form-label">Host Label</label>
              <input className="form-input" value={form.host_label} onChange={e => setForm(p => ({ ...p, host_label: e.target.value }))} placeholder="Production Node 1" />
            </div>
            {error && <div className="error-msg">{error}</div>}
            <div className="modal-footer">
              <button className="btn" onClick={() => setModal(false)}>Cancel</button>
              <button className="btn primary" onClick={create}>Register</button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
