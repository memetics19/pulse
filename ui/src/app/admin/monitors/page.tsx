'use client'
import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import type { CheckResult, Monitor, MonitorAuthInput, MonitorGroup } from '@/lib/types'
import {
  adminListMonitors, adminListGroups, adminCreateMonitor,
  adminUpdateMonitor, adminDeleteMonitor, adminCreateGroup,
  adminListCheckResults, adminRotatePushToken,
} from '@/lib/api'
import { useConfirm } from '@/components/ConfirmDialog'

type MonitorType = Monitor['type']

// TYPE_INFO describes each type in the form. target is the label of the URL
// field, or null when the type has none. https is an older alias of http and
// is only offered when an existing monitor already uses it.
const TYPE_INFO: Record<MonitorType, { label: string; hint: string; target: string | null; placeholder: string }> = {
  http:  { label: 'HTTP(S)', hint: 'Sends a request and checks the response.', target: 'URL', placeholder: 'https://example.com/health' },
  https: { label: 'HTTPS', hint: 'Sends a request and checks the response.', target: 'URL', placeholder: 'https://example.com/health' },
  tcp:   { label: 'TCP port', hint: 'Up when the port accepts a connection.', target: 'Host and port', placeholder: 'db.example.com:5432' },
  ping:  { label: 'Ping', hint: 'Up when the host answers an ICMP ping.', target: 'Host', placeholder: 'example.com' },
  dns:   { label: 'DNS', hint: 'Up when the name resolves.', target: 'Hostname', placeholder: 'example.com' },
  ssl:   { label: 'TLS certificate', hint: 'Tracks the days left on the certificate.', target: 'Host and port', placeholder: 'example.com:443' },
  push:  { label: 'Push (heartbeat)', hint: 'Your job calls a Pulse URL. Down when no call arrives within the interval.', target: null, placeholder: '' },
  infra: { label: 'Agent metrics', hint: 'Host metrics sent by the Pulse agent.', target: 'Agent host', placeholder: 'web-1' },
}
const TYPE_ORDER: MonitorType[] = ['http', 'tcp', 'ping', 'dns', 'ssl', 'push', 'infra']
// Only these checkers retry a failed attempt within one check.
const RETRY_TYPES: MonitorType[] = ['http', 'https', 'tcp', 'ssl']
const METHODS: Monitor['method'][] = ['GET', 'HEAD', 'POST']
// How many recent checks the list shows per monitor.
const HISTORY = 30

const EMPTY: Partial<Monitor> = {
  name: '', url: '', type: 'http', interval_seconds: 60, timeout_seconds: 10,
  degraded_threshold_ms: 500, down_threshold_ms: 2000, is_active: true,
  keyword_check: '', source: 'internal', external_id: '',
  failures_before_incident: 2, recoveries_before_resolve: 2, retries: 1, retry_interval_ms: 500,
  method: 'GET', headers: {}, body: '', auth: null, tls_verify: true, ca_pem: '',
  follow_redirects: true, accepted_statuses: [], keyword_mode: 'contains',
  tls_warn_days: 14, tls_critical_days: 7,
}

const isHTTP = (t?: MonitorType) => t === 'http' || t === 'https'

type State = 'up' | 'degraded' | 'down' | 'paused' | 'pending'
const STATE_LABEL: Record<State, string> = { up: 'Up', degraded: 'Degraded', down: 'Down', paused: 'Paused', pending: 'No data' }

function stateOf(m: Monitor, results?: CheckResult[]): State {
  if (!m.is_active) return 'paused'
  return results?.[0]?.status ?? 'pending'
}

function formatInterval(s: number): string {
  if (s >= 3600 && s % 3600 === 0) return `${s / 3600}h`
  if (s >= 60 && s % 60 === 0) return `${s / 60}m`
  return `${s}s`
}

function ago(iso: string, now: number): string {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 10) return 'just now'
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

// historyDays is the window that holds the last HISTORY checks, so a monitor
// that checks hourly still fills its strip.
function historyDays(m: Monitor): number {
  return Math.min(90, Math.max(1, Math.ceil((m.interval_seconds * HISTORY) / 86400)))
}

function StateLabel({ state }: { state: State }) {
  return <span className={`state ${state}`}><span className="state-dot" aria-hidden="true" />{STATE_LABEL[state]}</span>
}

// CheckStrip shows the last HISTORY checks, oldest on the left.
function CheckStrip({ results }: { results?: CheckResult[] }) {
  const rs = (results ?? []).slice(0, HISTORY).reverse()
  const pad = HISTORY - rs.length
  const up = rs.filter(r => r.status === 'up').length
  return (
    <div className="check-strip" role="img" aria-label={rs.length ? `${up} of the last ${rs.length} checks up` : 'No checks yet'}>
      {Array.from({ length: pad }, (_, i) => <i key={`e${i}`} className="empty" />)}
      {rs.map(r => (
        <i key={r.id} className={r.status} title={`${STATE_LABEL[r.status]}${r.response_time_ms != null ? ` · ${r.response_time_ms} ms` : ''} · ${new Date(r.checked_at).toLocaleString()}`} />
      ))}
    </div>
  )
}

function CertExpiry({ m, days }: { m: Monitor; days: number | null | undefined }) {
  if (m.type !== 'ssl' || days == null) return null
  const text = days === 0 ? 'expires in under a day' : `expires in ${days} ${days === 1 ? 'day' : 'days'}`
  const color = days <= m.tls_critical_days ? 'var(--down)'
    : days <= m.tls_warn_days ? 'var(--warn)' : 'var(--text-muted)'
  return <span style={{ fontSize: 12, color }}>{text}</span>
}

type Filter = 'all' | 'issues' | 'paused'

export default function MonitorsPage() {
  const confirm = useConfirm()
  const [monitors, setMonitors] = useState<Monitor[] | null>(null)
  const [groups,   setGroups]   = useState<MonitorGroup[]>([])
  const [results,  setResults]  = useState<Record<number, CheckResult[]>>({})
  const [loadErr,  setLoadErr]  = useState('')
  const [query,    setQuery]    = useState('')
  const [filter,   setFilter]   = useState<Filter>('all')
  const [editing,  setEditing]  = useState<{ mode: 'create' | 'edit'; monitor: Partial<Monitor> } | null>(null)
  const [now,      setNow]      = useState(() => Date.now())

  const loadResults = useCallback((ms: Monitor[]) => {
    for (const m of ms) {
      adminListCheckResults(m.id, historyDays(m), HISTORY)
        .then(rs => setResults(p => ({ ...p, [m.id]: rs })))
        .catch(() => {})
    }
  }, [])

  const load = useCallback(() => {
    adminListMonitors()
      .then(ms => { setMonitors(ms); setLoadErr(''); loadResults(ms) })
      .catch(() => setLoadErr('Could not load monitors. Check that Pulse is running and reload the page.'))
    adminListGroups().then(setGroups).catch(() => {})
  }, [loadResults])

  useEffect(() => { load() }, [load])

  // Refresh statuses every 30 seconds while the tab is visible, and keep the
  // "checked" times current.
  useEffect(() => {
    const id = setInterval(() => {
      setNow(Date.now())
      if (document.visibilityState === 'visible' && monitors) loadResults(monitors)
    }, 30_000)
    return () => clearInterval(id)
  }, [monitors, loadResults])

  const states = useMemo(() => {
    const out: Record<number, State> = {}
    for (const m of monitors ?? []) out[m.id] = stateOf(m, results[m.id])
    return out
  }, [monitors, results])

  const counts = useMemo(() => {
    const c = { all: 0, issues: 0, paused: 0, up: 0, degraded: 0, down: 0 }
    for (const m of monitors ?? []) {
      const s = states[m.id]
      c.all++
      if (s === 'down' || s === 'degraded') c.issues++
      if (s === 'paused') c.paused++
      if (s === 'up' || s === 'degraded' || s === 'down') c[s]++
    }
    return c
  }, [monitors, states])

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (monitors ?? []).filter(m => {
      const s = states[m.id]
      if (filter === 'issues' && s !== 'down' && s !== 'degraded') return false
      if (filter === 'paused' && s !== 'paused') return false
      return !q || m.name.toLowerCase().includes(q) || m.url.toLowerCase().includes(q) || m.type.includes(q)
    })
  }, [monitors, states, query, filter])

  // Sections in the group order of the status page. Monitors without a group
  // come last. With no groups at all, the table has no section rows.
  const sections = useMemo(() => {
    const byGroup = new Map<number | null, Monitor[]>()
    for (const m of visible) {
      const key = groups.some(g => g.id === m.group_id) ? m.group_id : null
      byGroup.set(key, [...(byGroup.get(key) ?? []), m])
    }
    const ordered = [...groups].sort((a, b) => a.display_order - b.display_order)
      .filter(g => byGroup.has(g.id))
      .map(g => ({ key: String(g.id), name: g.name, monitors: byGroup.get(g.id)! }))
    if (byGroup.has(null)) ordered.push({ key: 'none', name: groups.length ? 'No group' : '', monitors: byGroup.get(null)! })
    return ordered
  }, [visible, groups])

  async function remove(m: Monitor) {
    const ok = await confirm({
      title: `Delete ${m.name}?`,
      message: 'Its check history is deleted with it. This cannot be undone.',
      confirmLabel: 'Delete monitor',
    })
    if (!ok) return
    try {
      await adminDeleteMonitor(m.id)
      setMonitors(p => (p ?? []).filter(x => x.id !== m.id))
    } catch {
      setLoadErr(`Could not delete ${m.name}. Try again.`)
    }
  }

  function saved(m: Monitor, created: boolean) {
    setMonitors(p => created ? [...(p ?? []), m] : (p ?? []).map(x => x.id === m.id ? m : x))
    loadResults([m])
  }

  const summary = monitors && monitors.length > 0
    ? [`${counts.all} ${counts.all === 1 ? 'monitor' : 'monitors'}`,
       counts.down ? `${counts.down} down` : '',
       counts.degraded ? `${counts.degraded} degraded` : '',
       counts.paused ? `${counts.paused} paused` : ''].filter(Boolean).join(' · ')
    : ''

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="admin-title">Monitors</h1>
          {summary && <div className="page-sub">{summary}</div>}
        </div>
        <button className="btn primary" onClick={() => setEditing({ mode: 'create', monitor: EMPTY })}>Add monitor</button>
      </div>

      {loadErr && <div className="callout" role="alert" style={{ marginBottom: 12 }}>{loadErr}</div>}

      {monitors === null && !loadErr && (
        <div style={{ display: 'grid', gap: 8 }} aria-busy="true">
          {[0, 1, 2].map(i => <div key={i} className="skeleton" style={{ height: 44 }} />)}
        </div>
      )}

      {monitors && monitors.length === 0 && (
        <div className="empty-state">
          <h2>No monitors yet</h2>
          <p>A monitor checks one thing on a schedule: a URL, a port, a host, a certificate, or a heartbeat from a job. Its results drive the status page and alerts.</p>
          <button className="btn primary" onClick={() => setEditing({ mode: 'create', monitor: EMPTY })}>Add your first monitor</button>
        </div>
      )}

      {monitors && monitors.length > 0 && (
        <>
          <div className="list-toolbar">
            <input
              className="form-input search-input"
              type="search"
              placeholder="Search by name, URL or type"
              aria-label="Search monitors"
              value={query}
              onChange={e => setQuery(e.target.value)}
            />
            <div className="filter-tabs" role="tablist" aria-label="Filter monitors">
              {([['all', 'All'], ['issues', 'Issues'], ['paused', 'Paused']] as [Filter, string][]).map(([f, label]) => (
                <button key={f} role="tab" aria-selected={filter === f} className={`filter-tab${filter === f ? ' active' : ''}`} onClick={() => setFilter(f)}>
                  {label}<span className="filter-count">{counts[f]}</span>
                </button>
              ))}
            </div>
          </div>

          {visible.length === 0 ? (
            <div className="empty-state">
              <h2>No monitors match</h2>
              <p>Try another search, or show all monitors.</p>
              <button className="btn" onClick={() => { setQuery(''); setFilter('all') }}>Show all</button>
            </div>
          ) : (
            <table className="admin-table monitor-table">
              <thead>
                <tr>
                  <th style={{ width: 110 }}>Status</th>
                  <th>Monitor</th>
                  <th className="hide-sm">Type</th>
                  <th className="hide-sm">Last {HISTORY} checks</th>
                  <th className="hide-sm" style={{ textAlign: 'right' }}>Response</th>
                  <th>Checked</th>
                  <th className="hide-sm">Every</th>
                  <th><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {sections.map(sec => (
                  <SectionRows key={sec.key} name={sec.name}>
                    {sec.monitors.map(m => {
                      const rs = results[m.id]
                      const last = rs?.[0]
                      const state = states[m.id]
                      return (
                        <tr
                          key={m.id}
                          className="monitor-line"
                          tabIndex={0}
                          onClick={() => setEditing({ mode: 'edit', monitor: m })}
                          onKeyDown={e => { if (e.key === 'Enter' && e.target === e.currentTarget) setEditing({ mode: 'edit', monitor: m }) }}
                        >
                          <td><StateLabel state={state} /></td>
                          <td>
                            <div className="m-name-cell">
                              <strong>{m.name}</strong>
                              {state === 'down' && last?.error_message
                                ? <span className="err-line" title={last.error_message}>{last.error_message}</span>
                                : m.type === 'ssl'
                                  ? <CertExpiry m={m} days={last?.cert_days_left} />
                                  : m.type === 'push'
                                    ? <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Waits for heartbeats</span>
                                    : <span className="mono-muted" title={m.url}>{m.url}</span>}
                            </div>
                          </td>
                          <td className="hide-sm"><span className="type-badge">{m.type}</span></td>
                          <td className="hide-sm"><CheckStrip results={rs} /></td>
                          <td className="hide-sm num" style={{ textAlign: 'right', color: 'var(--text-muted)' }}>
                            {last?.response_time_ms != null ? `${last.response_time_ms} ms` : '–'}
                          </td>
                          <td className="num" style={{ color: 'var(--text-muted)' }} title={last ? new Date(last.checked_at).toLocaleString() : undefined}>
                            {last ? ago(last.checked_at, now) : 'Never'}
                          </td>
                          <td className="hide-sm num" style={{ color: 'var(--text-muted)' }}>{formatInterval(m.interval_seconds)}</td>
                          <td onClick={e => e.stopPropagation()}>
                            <div className="row-actions">
                              <button className="btn ghost sm" onClick={() => setEditing({ mode: 'edit', monitor: m })} aria-label={`Edit ${m.name}`}>Edit</button>
                              <button className="btn ghost sm danger" onClick={() => remove(m)} aria-label={`Delete ${m.name}`}>Delete</button>
                            </div>
                          </td>
                        </tr>
                      )
                    })}
                  </SectionRows>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}

      {editing && (
        <MonitorDrawer
          mode={editing.mode}
          initial={editing.monitor}
          groups={groups}
          onGroupCreated={g => setGroups(p => [...p, g])}
          onClose={() => setEditing(null)}
          onSaved={saved}
        />
      )}
    </>
  )
}

function SectionRows({ name, children }: { name: string; children: React.ReactNode }) {
  return (
    <>
      {name && <tr className="group-line"><td colSpan={8}>{name}</td></tr>}
      {children}
    </>
  )
}

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

function MonitorDrawer({ mode, initial, groups, onGroupCreated, onClose, onSaved }: {
  mode: 'create' | 'edit'
  initial: Partial<Monitor>
  groups: MonitorGroup[]
  onGroupCreated: (g: MonitorGroup) => void
  onClose: () => void
  onSaved: (m: Monitor, created: boolean) => void
}) {
  const confirm = useConfirm()
  const [form, setForm] = useState<Partial<Monitor>>(initial)
  const [headersText, setHeadersText] = useState(headersToText(initial.headers))
  const [statusesText, setStatusesText] = useState((initial.accepted_statuses ?? []).join(', '))
  const [authType, setAuthType] = useState<AuthType>(initial.auth?.type ?? 'none')
  const [authUser, setAuthUser] = useState(initial.auth?.username ?? '')
  const [authSecret, setAuthSecret] = useState('')
  const [newGroup, setNewGroup] = useState<string | null>(null)
  const [error, setError] = useState('')
  // invalid names the field a client-side check rejected, so it can be marked.
  const [invalid, setInvalid] = useState<'name' | 'url' | null>(null)
  const [saving, setSaving] = useState(false)
  const urlRef = useRef<HTMLInputElement>(null)
  // The push URL is only known right after a create or a rotate.
  const [pushURL, setPushURL] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  const type = (form.type ?? 'http') as MonitorType
  const info = TYPE_INFO[type]
  const types = type === 'https' ? [...TYPE_ORDER, 'https' as MonitorType] : TYPE_ORDER

  useEffect(() => {
    nameRef.current?.focus()
    // Escape closes the drawer, unless a confirm dialog on top handles it.
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-overlay')) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const set = <K extends keyof Monitor>(k: K, v: Monitor[K]) => setForm(p => ({ ...p, [k]: v }))
  const num = (k: keyof Monitor) => ({
    value: form[k] == null ? '' : String(form[k]),
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => setForm(p => ({ ...p, [k]: e.target.value === '' ? undefined : Number(e.target.value) })),
  })

  // The secret is write-only. An empty secret keeps the stored one.
  function authInput(): MonitorAuthInput {
    if (authType === 'none') return { type: 'none' }
    if (authType === 'bearer') return { type: 'bearer', ...(authSecret ? { token: authSecret } : {}) }
    return { type: 'basic', username: authUser, ...(authSecret ? { password: authSecret } : {}) }
  }

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError(''); setInvalid(null)
    if (!form.name?.trim()) { setError('Give the monitor a name.'); setInvalid('name'); nameRef.current?.focus(); return }
    if (info.target && !form.url?.trim()) { setError(`Enter the ${info.target.toLowerCase()} to check.`); setInvalid('url'); urlRef.current?.focus(); return }
    setSaving(true)
    try {
      const payload = {
        ...form,
        name: form.name.trim(),
        url: info.target ? form.url?.trim() : '',
        headers: isHTTP(type) ? parseHeaders(headersText) : {},
        accepted_statuses: isHTTP(type) ? statusesText.split(',').map(s => s.trim()).filter(Boolean) : [],
        auth: isHTTP(type) ? authInput() : { type: 'none' as const },
      }
      if (mode === 'create') {
        const m = await adminCreateMonitor(payload)
        onSaved(m, true)
        if (m.push_url) { setPushURL(m.push_url); setForm(m); return }
      } else if (form.id) {
        onSaved(await adminUpdateMonitor(form.id, payload), false)
      }
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  async function rotate() {
    if (!form.id) return
    const ok = await confirm({
      title: 'Rotate the push URL?',
      message: 'The current URL stops working right away. Update your job with the new one.',
      confirmLabel: 'Rotate URL',
    })
    if (!ok) return
    try { setPushURL((await adminRotatePushToken(form.id)).push_url) }
    catch (err) { setError(err instanceof Error ? err.message : String(err)) }
  }

  async function addGroup() {
    const name = newGroup?.trim()
    if (!name) return
    try {
      const g = await adminCreateGroup({ name, display_order: groups.length, description: '' })
      onGroupCreated(g)
      set('group_id', g.id)
      setNewGroup(null)
    } catch (err) { setError(err instanceof Error ? err.message : String(err)) }
  }

  // After a push monitor is created, the drawer shows its URL instead of the form.
  if (mode === 'create' && pushURL) {
    return (
      <Drawer title={`${form.name} is ready`} onClose={onClose} footer={<button className="btn primary" onClick={onClose}>Done</button>}>
        <PushURL url={pushURL} interval={form.interval_seconds ?? 60} fresh />
      </Drawer>
    )
  }

  return (
    <Drawer
      title={mode === 'create' ? 'Add monitor' : `Edit ${initial.name}`}
      onClose={onClose}
      onSubmit={save}
      footer={<>
        {error && <div className="error-msg" role="alert">{error}</div>}
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn primary" disabled={saving}>{saving ? 'Saving…' : mode === 'create' ? 'Create monitor' : 'Save changes'}</button>
      </>}
    >
      <section className="form-section">
        <div className="form-group">
          <label className="form-label" htmlFor="m-name">Name</label>
          <input id="m-name" ref={nameRef} className="form-input" value={form.name ?? ''} onChange={e => set('name', e.target.value)} placeholder="Billing API" aria-invalid={invalid === 'name' || undefined} aria-describedby="m-name-hint" />
          <div className="form-hint" id="m-name-hint">Shown on the status page and in alerts.</div>
        </div>
        <div className="form-group">
          <label className="form-label" htmlFor="m-type">Type</label>
          <select id="m-type" className="form-select" value={type} onChange={e => set('type', e.target.value as MonitorType)} disabled={mode === 'edit' && type === 'push'}>
            {types.map(t => <option key={t} value={t}>{TYPE_INFO[t].label}</option>)}
          </select>
          <div className="form-hint">{info.hint}</div>
        </div>
        {info.target && (
          <div className="form-group">
            <label className="form-label" htmlFor="m-url">{info.target}</label>
            <input id="m-url" ref={urlRef} className="form-input" value={form.url ?? ''} onChange={e => set('url', e.target.value)} placeholder={info.placeholder} spellCheck={false} autoCapitalize="off" aria-invalid={invalid === 'url' || undefined} />
          </div>
        )}
        <div className="form-group" style={{ marginBottom: 0 }}>
          <label className="form-label" htmlFor="m-group">Group</label>
          {newGroup === null ? (
            <div style={{ display: 'flex', gap: 8 }}>
              <select id="m-group" className="form-select" value={form.group_id ?? ''} onChange={e => set('group_id', e.target.value ? Number(e.target.value) : null)}>
                <option value="">No group</option>
                {groups.map(g => <option key={g.id} value={g.id}>{g.name}</option>)}
              </select>
              <button type="button" className="btn" style={{ whiteSpace: 'nowrap' }} onClick={() => setNewGroup('')}>New group</button>
            </div>
          ) : (
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                className="form-input"
                autoFocus
                aria-label="New group name"
                placeholder="Core API"
                value={newGroup}
                onChange={e => setNewGroup(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addGroup() } if (e.key === 'Escape') { e.stopPropagation(); setNewGroup(null) } }}
              />
              <button type="button" className="btn" onClick={addGroup} disabled={!newGroup.trim()}>Add</button>
              <button type="button" className="btn ghost" onClick={() => setNewGroup(null)}>Cancel</button>
            </div>
          )}
          <div className="form-hint">Groups are the sections of the status page.</div>
        </div>
      </section>

      {mode === 'edit' && type === 'push' && (
        <section className="form-section">
          <div className="form-section-title">Push URL</div>
          {pushURL
            ? <PushURL url={pushURL} interval={form.interval_seconds ?? 60} fresh />
            : <>
                <div className="form-section-desc">Pulse only stores a hash of the URL, so it cannot show it again. Rotate it to get a new one.</div>
                <button type="button" className="btn" onClick={rotate}>Rotate push URL</button>
              </>}
        </section>
      )}

      <section className="form-section">
        <div className="form-section-title">Schedule</div>
        <div className="form-section-desc">{type === 'push' ? 'The monitor is down when no heartbeat arrives within the interval.' : 'How often Pulse checks, and how long one check may take.'}</div>
        <div className="form-row">
          <div className="form-group">
            <label className="form-label" htmlFor="m-interval">{type === 'push' ? 'Expect a heartbeat every' : 'Check every'}</label>
            <div className="input-suffix"><input id="m-interval" className="form-input" type="number" min={1} {...num('interval_seconds')} /><span>sec</span></div>
            <div className="form-hint">{form.interval_seconds ? `That is every ${formatInterval(form.interval_seconds)}.` : ' '}</div>
          </div>
          {type !== 'push' && (
            <div className="form-group">
              <label className="form-label" htmlFor="m-timeout">Timeout</label>
              <div className="input-suffix"><input id="m-timeout" className="form-input" type="number" min={1} {...num('timeout_seconds')} /><span>sec</span></div>
            </div>
          )}
        </div>
        <label className="form-check">
          <input type="checkbox" checked={!(form.is_active ?? true)} onChange={e => set('is_active', !e.target.checked)} />
          <span>Pause this monitor<div className="form-hint" style={{ marginTop: 1 }}>A paused monitor runs no checks and opens no incidents.</div></span>
        </label>
      </section>

      {type !== 'push' && (
        <section className="form-section">
          <div className="form-section-title">When is it degraded or down?</div>
          <div className="form-section-desc">A failed check is always down. A slow answer can be marked degraded or down too.</div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label" htmlFor="m-degraded">Degraded when slower than</label>
              <div className="input-suffix"><input id="m-degraded" className="form-input" type="number" min={1} {...num('degraded_threshold_ms')} /><span>ms</span></div>
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="m-down">Down when slower than</label>
              <div className="input-suffix"><input id="m-down" className="form-input" type="number" min={1} {...num('down_threshold_ms')} /><span>ms</span></div>
            </div>
          </div>
          {type === 'ssl' && (
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="m-tlswarn">Degraded with this many days left</label>
                <div className="input-suffix"><input id="m-tlswarn" className="form-input" type="number" min={1} {...num('tls_warn_days')} /><span>days</span></div>
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="m-tlscrit">Down with this many days left</label>
                <div className="input-suffix"><input id="m-tlscrit" className="form-input" type="number" min={0} {...num('tls_critical_days')} /><span>days</span></div>
              </div>
            </div>
          )}
          {isHTTP(type) && (
            <div className="form-row">
              <div className="form-group">
                <label className="form-label" htmlFor="m-keyword">Keyword</label>
                <input id="m-keyword" className="form-input" value={form.keyword_check ?? ''} onChange={e => set('keyword_check', e.target.value)} placeholder="Optional" />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="m-kwmode">The body must</label>
                <select id="m-kwmode" className="form-select" value={form.keyword_mode ?? 'contains'} onChange={e => set('keyword_mode', e.target.value as Monitor['keyword_mode'])} disabled={!form.keyword_check}>
                  <option value="contains">contain the keyword</option>
                  <option value="not_contains">not contain the keyword</option>
                </select>
              </div>
            </div>
          )}
        </section>
      )}

      <section className="form-section">
        <div className="form-section-title">Alerting</div>
        <div className="form-section-desc">How many results in a row it takes to open or resolve an incident.</div>
        <div className="form-row">
          <div className="form-group">
            <label className="form-label" htmlFor="m-fails">Open an incident after</label>
            <div className="input-suffix"><input id="m-fails" className="form-input" type="number" min={1} max={10} {...num('failures_before_incident')} /><span>fails</span></div>
          </div>
          <div className="form-group">
            <label className="form-label" htmlFor="m-recov">Resolve it after</label>
            <div className="input-suffix"><input id="m-recov" className="form-input" type="number" min={1} max={10} {...num('recoveries_before_resolve')} /><span>passes</span></div>
          </div>
        </div>
        {RETRY_TYPES.includes(type) && (
          <div className="form-row">
            <div className="form-group" style={{ marginBottom: 0 }}>
              <label className="form-label" htmlFor="m-retries">Retries per check</label>
              <input id="m-retries" className="form-input" type="number" min={0} max={5} {...num('retries')} />
              <div className="form-hint">Retries a network error or a 5xx before the check counts as failed.</div>
            </div>
            <div className="form-group" style={{ marginBottom: 0 }}>
              <label className="form-label" htmlFor="m-retryms">Wait between retries</label>
              <div className="input-suffix"><input id="m-retryms" className="form-input" type="number" min={0} max={60000} {...num('retry_interval_ms')} /><span>ms</span></div>
            </div>
          </div>
        )}
      </section>

      {isHTTP(type) && (
        <section className="form-section">
          <details className="advanced" open={form.method !== 'GET' || headersText !== '' || authType !== 'none' || statusesText !== '' || form.follow_redirects === false || form.tls_verify === false || !!form.ca_pem}>
            <summary>Request options</summary>
            <div className="advanced-body">
              <div className="form-row">
                <div className="form-group">
                  <label className="form-label" htmlFor="m-method">Method</label>
                  <select id="m-method" className="form-select" value={form.method ?? 'GET'} onChange={e => set('method', e.target.value as Monitor['method'])}>
                    {METHODS.map(m => <option key={m} value={m}>{m}</option>)}
                  </select>
                </div>
                <div className="form-group">
                  <label className="form-label" htmlFor="m-statuses">Accepted statuses</label>
                  <input id="m-statuses" className="form-input" value={statusesText} onChange={e => setStatusesText(e.target.value)} placeholder="Any 2xx" />
                  <div className="form-hint">Codes or ranges, such as 200-299, 301.</div>
                </div>
              </div>
              {form.method === 'POST' && (
                <div className="form-group">
                  <label className="form-label" htmlFor="m-body">Request body</label>
                  <textarea id="m-body" className="form-textarea" value={form.body ?? ''} onChange={e => set('body', e.target.value)} />
                </div>
              )}
              <div className="form-group">
                <label className="form-label" htmlFor="m-headers">Headers</label>
                <textarea id="m-headers" className="form-textarea" value={headersText} onChange={e => setHeadersText(e.target.value)} placeholder="X-Api-Key: ..." spellCheck={false} />
                <div className="form-hint">One Name: value per line. Header values can be read back, so put secrets in auth.</div>
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label className="form-label" htmlFor="m-auth">Auth</label>
                  <select id="m-auth" className="form-select" value={authType} onChange={e => { setAuthType(e.target.value as AuthType); setAuthSecret('') }}>
                    <option value="none">None</option>
                    <option value="basic">Basic</option>
                    <option value="bearer">Bearer token</option>
                  </select>
                </div>
                {authType === 'basic' && (
                  <div className="form-group">
                    <label className="form-label" htmlFor="m-user">Username</label>
                    <input id="m-user" className="form-input" value={authUser} onChange={e => setAuthUser(e.target.value)} autoComplete="off" />
                  </div>
                )}
              </div>
              {authType !== 'none' && (
                <div className="form-group">
                  <label className="form-label" htmlFor="m-secret">{authType === 'basic' ? 'Password' : 'Token'}</label>
                  <input
                    id="m-secret"
                    className="form-input"
                    type="password"
                    autoComplete="new-password"
                    value={authSecret}
                    onChange={e => setAuthSecret(e.target.value)}
                    placeholder={form.auth?.secret_set && form.auth.type === authType ? 'Saved. Leave empty to keep it.' : ''}
                  />
                  <div className="form-hint">Write-only. Pulse never shows it again.</div>
                </div>
              )}
              <div style={{ display: 'grid', gap: 10, marginBottom: 16 }}>
                <label className="form-check">
                  <input type="checkbox" checked={form.follow_redirects ?? true} onChange={e => set('follow_redirects', e.target.checked)} />
                  <span>Follow redirects</span>
                </label>
                <label className="form-check">
                  <input type="checkbox" checked={form.tls_verify ?? true} onChange={e => set('tls_verify', e.target.checked)} />
                  <span>Verify the TLS certificate</span>
                </label>
              </div>
              {(form.tls_verify ?? true) && (
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label" htmlFor="m-ca">Private CA certificate</label>
                  <textarea id="m-ca" className="form-textarea" value={form.ca_pem ?? ''} onChange={e => set('ca_pem', e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" spellCheck={false} />
                  <div className="form-hint">Optional. Trusted on top of the system roots, for a homelab or internal CA.</div>
                </div>
              )}
            </div>
          </details>
        </section>
      )}
    </Drawer>
  )
}

function Drawer({ title, onClose, onSubmit, footer, children }: {
  title: string
  onClose: () => void
  onSubmit?: (e: React.FormEvent) => void
  footer: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <>
      <div className="drawer-backdrop" onClick={onClose} />
      <form className="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" noValidate onSubmit={onSubmit ?? (e => e.preventDefault())}>
        <div className="drawer-head">
          <h2 id="drawer-title">{title}</h2>
          <button type="button" className="btn ghost sm" onClick={onClose} aria-label="Close">Close</button>
        </div>
        <div className="drawer-body">{children}</div>
        <div className="drawer-foot">{footer}</div>
      </form>
    </>
  )
}

function PushURL({ url, interval, fresh }: { url: string; interval: number; fresh?: boolean }) {
  const [copied, setCopied] = useState(false)
  async function copy() {
    try { await navigator.clipboard.writeText(url); setCopied(true); setTimeout(() => setCopied(false), 1500) } catch { /* the URL stays selectable */ }
  }
  return (
    <div className={`callout${fresh ? ' ok' : ''}`}>
      <div style={{ fontWeight: 600 }}>Copy the push URL now</div>
      <div className="form-hint" style={{ marginTop: 2 }}>Pulse only keeps a hash of it. Call it at least every {formatInterval(interval)} from your job.</div>
      <div className="copy-field">
        <code>{url}</code>
        <button type="button" className="btn sm" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
      <div className="form-hint" style={{ marginTop: 10 }}>For example, at the end of a cron job:</div>
      <div className="copy-field"><code>curl -fsS {url}</code></div>
    </div>
  )
}
