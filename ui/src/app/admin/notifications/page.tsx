'use client'
import { useState, useEffect } from 'react'
import type { Notification } from '@/lib/types'
import { adminListNotifications, adminCreateNotification, adminDeleteNotification } from '@/lib/api'
import { useConfirm } from '@/components/ConfirmDialog'

export default function NotificationsPage() {
  const confirm = useConfirm()
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [modal,   setModal]   = useState(false)
  const [channel, setChannel] = useState<'email' | 'slack'>('email')
  const [config,  setConfig]  = useState('{"to":"user@example.com"}')
  const [error,   setError]   = useState('')

  useEffect(() => {
    adminListNotifications().then(setNotifications).catch(() => {})
  }, [])

  async function save() {
    setError('')
    try {
      JSON.parse(config)
      const n = await adminCreateNotification({ channel, config_json: config, monitor_id: null })
      setNotifications(p => [...p, n])
      setModal(false)
    } catch (e) { setError(e instanceof Error ? e.message : 'Invalid JSON') }
  }

  async function del(id: number) {
    const ok = await confirm({
      title: 'Delete notification?',
      message: 'This channel will stop receiving alerts. This cannot be undone.',
    })
    if (!ok) return
    await adminDeleteNotification(id)
    setNotifications(p => p.filter(n => n.id !== id))
  }

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>Notifications</h1>
        <button className="btn primary" onClick={() => setModal(true)}>+ Add</button>
      </div>

      <table className="admin-table">
        <thead><tr><th>Channel</th><th>Config</th><th>Monitor</th><th></th></tr></thead>
        <tbody>
          {notifications.map(n => (
            <tr key={n.id}>
              <td><code>{n.channel}</code></td>
              <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{n.config_json}</td>
              <td>{n.monitor_id ?? 'global'}</td>
              <td><button className="btn danger" onClick={() => del(n.id)}>Delete</button></td>
            </tr>
          ))}
        </tbody>
      </table>

      {modal && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setModal(false) }}>
          <div className="modal">
            <h2 className="modal-title">Add Notification</h2>
            <div className="form-group">
              <label className="form-label">Channel</label>
              <select className="form-select" value={channel} onChange={e => {
                const c = e.target.value as 'email' | 'slack'
                setChannel(c)
                setConfig(c === 'email' ? '{"to":"user@example.com"}' : '{"webhook_url":"https://hooks.slack.com/..."}')
              }}>
                <option value="email">Email (Resend)</option>
                <option value="slack">Slack webhook</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Config JSON</label>
              <textarea className="form-textarea" value={config} onChange={e => setConfig(e.target.value)} />
            </div>
            {error && <div className="error-msg">{error}</div>}
            <div className="modal-footer">
              <button className="btn" onClick={() => setModal(false)}>Cancel</button>
              <button className="btn primary" onClick={save}>Save</button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
