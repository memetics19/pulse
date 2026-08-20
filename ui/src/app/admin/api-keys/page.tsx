'use client'
import { useState, useEffect, useCallback } from 'react'
import { listApiKeys, createApiKey, revokeApiKey, type ApiKey } from '@/lib/api'

const ALL_SCOPES = [
  'monitors:read',
  'monitors:write',
  'incidents:read',
  'incidents:write',
  'notifications:read',
  'notifications:write',
  'agents:read',
  'agents:write',
  'diagnostics:read',
  'theme:read',
  'theme:write',
  'status:read',
]

function fmtDate(iso: string | null): string {
  if (!iso) return 'never'
  const d = new Date(iso)
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState(false)
  // New-key form state
  const [name, setName] = useState('')
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(new Set())
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')
  // Revealed key (shown once)
  const [revealedKey, setRevealedKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  // Inline revoke confirm: stores the id pending confirmation
  const [revokeConfirm, setRevokeConfirm] = useState<number | null>(null)

  const load = useCallback(() => {
    listApiKeys()
      .then(setKeys)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  function openModal() {
    setName('')
    setSelectedScopes(new Set())
    setCreateError('')
    setModal(true)
  }

  function closeModal() {
    setModal(false)
    setName('')
    setSelectedScopes(new Set())
    setCreateError('')
  }

  function toggleScope(scope: string) {
    setSelectedScopes(prev => {
      const next = new Set(prev)
      if (next.has(scope)) { next.delete(scope) } else { next.add(scope) }
      return next
    })
  }

  async function handleCreate() {
    setCreateError('')
    if (!name.trim()) { setCreateError('Name is required.'); return }
    if (selectedScopes.size === 0) { setCreateError('Select at least one scope.'); return }
    setCreating(true)
    try {
      const result = await createApiKey(name.trim(), Array.from(selectedScopes))
      closeModal()
      setRevealedKey(result.key)
      load()
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : 'Failed to create key')
    } finally {
      setCreating(false)
    }
  }

  function dismissReveal() {
    setRevealedKey(null)
    setCopied(false)
  }

  async function copyKey() {
    if (!revealedKey) return
    try {
      await navigator.clipboard.writeText(revealedKey)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // fallback: select the text
    }
  }

  async function handleRevoke(id: number) {
    await revokeApiKey(id)
    setRevokeConfirm(null)
    setKeys(prev => prev.filter(k => k.id !== id))
  }

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>API Keys</h1>
        <button className="btn primary" onClick={openModal}>+ New API key</button>
      </div>

      {/* One-time key reveal banner */}
      {revealedKey && (
        <div className="card" style={{ marginBottom: 20, borderColor: '#a5b4fc', background: '#eef2ff' }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: '#3730a3', marginBottom: 8 }}>
            Copy this key now — you will not be able to see it again.
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <code style={{
              flex: 1,
              fontFamily: 'var(--font-mono)',
              fontSize: 13,
              background: '#fff',
              border: '1px solid #c7d2fe',
              borderRadius: 6,
              padding: '8px 12px',
              userSelect: 'all',
              wordBreak: 'break-all',
              color: '#1e1b4b',
            }}>
              {revealedKey}
            </code>
            <button className="btn" onClick={copyKey} style={{ flexShrink: 0 }}>
              {copied ? 'Copied!' : 'Copy'}
            </button>
            <button className="btn" onClick={dismissReveal} style={{ flexShrink: 0 }}>
              Dismiss
            </button>
          </div>
        </div>
      )}

      <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '28px 18px', color: 'var(--text-muted)', fontSize: 13 }}>Loading…</div>
        ) : keys.length === 0 ? (
          <div style={{ padding: '28px 18px', color: 'var(--text-muted)', fontSize: 13 }}>No API keys yet.</div>
        ) : (
          <table className="admin-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Prefix</th>
                <th>Scopes</th>
                <th>Last used</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {keys.map(k => (
                <tr key={k.id}>
                  <td style={{ fontWeight: 500 }}>{k.name}</td>
                  <td>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, background: 'var(--bg2)', border: '1px solid var(--border)', borderRadius: 4, padding: '2px 6px' }}>
                      {k.prefix}
                    </span>
                  </td>
                  <td>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {(k.scopes ?? []).map(s => (
                        <span key={s} style={{
                          fontSize: 11,
                          padding: '1px 7px',
                          borderRadius: 4,
                          border: '1px solid #c7d2fe',
                          background: '#eef2ff',
                          color: '#4338ca',
                          fontFamily: 'var(--font-mono)',
                        }}>
                          {s}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td style={{ color: 'var(--text-muted)', fontSize: 12 }}>{fmtDate(k.last_used_at)}</td>
                  <td style={{ color: 'var(--text-muted)', fontSize: 12 }}>{fmtDate(k.created_at)}</td>
                  <td>
                    {revokeConfirm === k.id ? (
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ fontSize: 12, color: 'var(--down)', fontWeight: 500 }}>Revoke?</span>
                        <button
                          className="btn danger"
                          style={{ padding: '4px 10px', fontSize: 12 }}
                          onClick={() => handleRevoke(k.id)}
                        >
                          Yes
                        </button>
                        <button
                          className="btn"
                          style={{ padding: '4px 10px', fontSize: 12 }}
                          onClick={() => setRevokeConfirm(null)}
                        >
                          Cancel
                        </button>
                      </span>
                    ) : (
                      <button
                        className="btn danger"
                        style={{ padding: '4px 10px', fontSize: 12 }}
                        onClick={() => setRevokeConfirm(k.id)}
                      >
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* New API key modal */}
      {modal && (
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) closeModal() }}
        >
          <div className="modal" style={{ width: 540 }}>
            <h2 className="modal-title">New API key</h2>

            <div className="form-group">
              <label className="form-label" htmlFor="apikey-name">Key name</label>
              <input
                id="apikey-name"
                className="form-input"
                placeholder="e.g. CI deploy token"
                value={name}
                onChange={e => setName(e.target.value)}
                autoFocus
              />
            </div>

            <div className="form-group">
              <label className="form-label">Scopes</label>
              <div style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(2, 1fr)',
                gap: '8px 16px',
                padding: '12px 14px',
                border: '1px solid var(--border)',
                borderRadius: 8,
                background: 'var(--bg2)',
              }}>
                {ALL_SCOPES.map(scope => (
                  <label key={scope} style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
                    <input
                      type="checkbox"
                      checked={selectedScopes.has(scope)}
                      onChange={() => toggleScope(scope)}
                      style={{ accentColor: 'var(--accent)', width: 14, height: 14, cursor: 'pointer' }}
                    />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>{scope}</span>
                  </label>
                ))}
              </div>
            </div>

            {createError && <div className="error-msg">{createError}</div>}

            <div className="modal-footer">
              <button className="btn" onClick={closeModal} disabled={creating}>Cancel</button>
              <button className="btn primary" onClick={handleCreate} disabled={creating}>
                {creating ? 'Creating…' : 'Create key'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
