'use client'
import { useState, useEffect, useCallback } from 'react'
import type { MonitorGroup } from '@/lib/types'
import {
  listPages, createPage, updatePage, deletePage,
  adminListGroups, type StatusPage,
} from '@/lib/api'

type FormState = {
  title: string
  domain: string
  published: boolean
  group_ids: number[]
}

const EMPTY_FORM: FormState = { title: '', domain: '', published: true, group_ids: [] }

function toggleItem(arr: number[], id: number): number[] {
  return arr.includes(id) ? arr.filter(x => x !== id) : [...arr, id]
}

export default function PagesPage() {
  const [pages,         setPages]         = useState<StatusPage[]>([])
  const [groups,        setGroups]        = useState<MonitorGroup[]>([])
  const [modal,         setModal]         = useState<'create' | 'edit' | null>(null)
  const [editTarget,    setEditTarget]    = useState<StatusPage | null>(null)
  const [form,          setForm]          = useState<FormState>(EMPTY_FORM)
  const [error,         setError]         = useState('')
  const [pendingDelete, setPendingDelete] = useState<number | null>(null)
  const [busy,          setBusy]          = useState(false)

  const load = useCallback(() => {
    listPages().then(setPages).catch(() => {})
    adminListGroups().then(setGroups).catch(() => {})
  }, [])

  useEffect(() => { load() }, [load])

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setError('')
    setModal('create')
  }

  function openEdit(page: StatusPage) {
    setEditTarget(page)
    setForm({
      title:     page.title,
      domain:    page.domain,
      published: page.published,
      group_ids: page.group_ids ?? [],
    })
    setError('')
    setModal('edit')
  }

  function closeModal() {
    setModal(null)
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setError('')
  }

  async function save() {
    if (!form.title.trim()) { setError('Title is required.'); return }
    setBusy(true); setError('')
    try {
      const payload = {
        title:     form.title.trim(),
        domain:    form.domain.trim(),
        published: form.published,
        group_ids: form.group_ids,
      }
      if (modal === 'create') {
        await createPage(payload)
      } else if (modal === 'edit' && editTarget) {
        await updatePage(editTarget.id, payload)
      }
      closeModal()
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function del(id: number) {
    await deletePage(id)
    setPages(p => p.filter(pg => pg.id !== id))
    setPendingDelete(null)
  }

  const isDefaultEdit = modal === 'edit' && editTarget?.is_default

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 className="admin-title" style={{ marginBottom: 0 }}>Pages</h1>
        <button className="btn primary" onClick={openCreate}>+ New page</button>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
        {pages.length === 0 ? (
          <div style={{ padding: '32px 24px', textAlign: 'center', color: 'var(--text-muted, #6b7280)', fontSize: 14 }}>
            No pages yet. Create one to get started.
          </div>
        ) : (
          pages.map((pg, i) => (
            <div
              key={pg.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                padding: '14px 20px',
                borderTop: i === 0 ? 'none' : '1px solid var(--border, #e6e8eb)',
              }}
            >
              {/* Title */}
              <div style={{ flex: 1, minWidth: 0 }}>
                <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text, #18181b)' }}>{pg.title}</span>
              </div>

              {/* Domain chip */}
              <div style={{ flexShrink: 0 }}>
                {pg.is_default ? (
                  <span style={{
                    display: 'inline-block',
                    padding: '2px 8px',
                    borderRadius: 5,
                    fontSize: 11,
                    fontWeight: 600,
                    background: '#eef0ff',
                    color: '#4f46e5',
                    letterSpacing: '0.01em',
                  }}>
                    default
                  </span>
                ) : (
                  <code style={{
                    display: 'inline-block',
                    padding: '2px 8px',
                    borderRadius: 5,
                    fontSize: 11,
                    background: 'var(--bg, #fbfbfc)',
                    border: '1px solid var(--border, #e6e8eb)',
                    color: 'var(--text, #18181b)',
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
                  }}>
                    {pg.domain}
                  </code>
                )}
              </div>

              {/* Published indicator */}
              <div style={{ flexShrink: 0 }}>
                <span style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 5,
                  fontSize: 12,
                  color: pg.published ? '#16a34a' : 'var(--text-muted, #6b7280)',
                }}>
                  <span style={{
                    width: 7,
                    height: 7,
                    borderRadius: '50%',
                    background: pg.published ? '#16a34a' : 'var(--text-muted, #6b7280)',
                    display: 'inline-block',
                  }} />
                  {pg.published ? 'Published' : 'Unpublished'}
                </span>
              </div>

              {/* Group count */}
              <div style={{ flexShrink: 0, fontSize: 12, color: 'var(--text-muted, #6b7280)', minWidth: 80, textAlign: 'right' }}>
                {pg.is_default ? 'all groups' : `${(pg.group_ids ?? []).length} group${(pg.group_ids ?? []).length !== 1 ? 's' : ''}`}
              </div>

              {/* Actions */}
              <div style={{ flexShrink: 0, display: 'flex', gap: 8, alignItems: 'center' }}>
                <button className="btn" onClick={() => openEdit(pg)}>Edit</button>
                {!pg.is_default && (
                  pendingDelete === pg.id ? (
                    <>
                      <button className="btn danger" onClick={() => del(pg.id)}>Yes, delete</button>
                      <button className="btn" onClick={() => setPendingDelete(null)}>Cancel</button>
                    </>
                  ) : (
                    <button className="btn danger" onClick={() => setPendingDelete(pg.id)}>Delete</button>
                  )
                )}
              </div>
            </div>
          ))
        )}
      </div>

      {/* Create / Edit Modal */}
      {modal && (
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) closeModal() }}
        >
          <div className="modal">
            <h2 className="modal-title">{modal === 'create' ? 'New page' : 'Edit page'}</h2>

            {/* Title */}
            <div className="form-group">
              <label className="form-label">Title</label>
              <input
                className="form-input"
                autoFocus
                placeholder="e.g. Status Page"
                value={form.title}
                onChange={e => setForm(p => ({ ...p, title: e.target.value }))}
              />
            </div>

            {/* Domain */}
            <div className="form-group">
              <label className="form-label">Domain</label>
              {isDefaultEdit ? (
                <input
                  className="form-input"
                  disabled
                  placeholder="default — served on the main host"
                  value=""
                  style={{ color: 'var(--text-muted, #6b7280)', fontStyle: 'italic' }}
                />
              ) : (
                <input
                  className="form-input"
                  placeholder="e.g. status.example.com"
                  value={form.domain}
                  onChange={e => setForm(p => ({ ...p, domain: e.target.value }))}
                />
              )}
            </div>

            {/* Published */}
            <div className="form-group">
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 14, color: 'var(--text, #18181b)' }}>
                <input
                  type="checkbox"
                  checked={form.published}
                  onChange={e => setForm(p => ({ ...p, published: e.target.checked }))}
                  style={{ width: 15, height: 15, accentColor: '#4f46e5', cursor: 'pointer' }}
                />
                Published
              </label>
            </div>

            {/* Groups */}
            <div className="form-group">
              <label className="form-label">Monitor groups</label>
              <p style={{ fontSize: 12, color: 'var(--text-muted, #6b7280)', marginBottom: 10, lineHeight: 1.5 }}>
                Visitors reaching this domain see only the selected groups.
                {isDefaultEdit && ' The default page (no domain) shows all groups.'}
              </p>
              {groups.length === 0 ? (
                <p style={{ fontSize: 13, color: 'var(--text-muted, #6b7280)' }}>No groups created yet.</p>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6, maxHeight: 200, overflowY: 'auto', padding: '2px 0' }}>
                  {groups.map(g => (
                    <label
                      key={g.id}
                      style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 14, color: 'var(--text, #18181b)' }}
                    >
                      <input
                        type="checkbox"
                        checked={form.group_ids.includes(g.id)}
                        onChange={() => setForm(p => ({ ...p, group_ids: toggleItem(p.group_ids, g.id) }))}
                        style={{ width: 15, height: 15, accentColor: '#4f46e5', cursor: 'pointer' }}
                      />
                      {g.name}
                    </label>
                  ))}
                </div>
              )}
            </div>

            {error && <div className="error-msg">{error}</div>}

            <div className="modal-footer">
              <button className="btn" onClick={closeModal} disabled={busy}>Cancel</button>
              <button className="btn primary" onClick={save} disabled={busy}>
                {busy ? 'Saving…' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
