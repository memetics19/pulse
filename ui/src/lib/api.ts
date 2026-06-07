// src/lib/api.ts
import type {
  StatusResponse, Monitor, MonitorGroup, Incident,
  IncidentUpdate, CheckResult, ThemeConfig, Notification, InfraAgent
} from './types'

// In production the static export is served by the Go binary on the same origin,
// so relative /api/... paths are correct. Set NEXT_PUBLIC_API_URL only when you
// need to point a local dev build at a different host.
const BASE = process.env.NEXT_PUBLIC_API_URL ?? '' // same-origin in prod

export async function fetchStatus(): Promise<StatusResponse> {
  const res = await fetch(`${BASE}/api/status`, { cache: 'no-store' })
  if (!res.ok) throw new Error(`status fetch failed: ${res.status}`)
  return res.json()
}

export async function fetchCheckResults(monitorId: number, days: number): Promise<CheckResult[]> {
  const res = await fetch(`${BASE}/api/monitors/${monitorId}/checks?days=${days}`, { cache: 'no-store' })
  if (!res.ok) return []
  return res.json()
}

export async function fetchIncidentUpdates(incidentId: number): Promise<IncidentUpdate[]> {
  const res = await fetch(`${BASE}/api/incidents/${incidentId}/updates`, { cache: 'no-store' })
  if (!res.ok) return []
  return res.json()
}

const ADMIN_HEADERS = { 'Content-Type': 'application/json' }

export async function adminListMonitors(): Promise<Monitor[]> {
  const res = await fetch(`${BASE}/api/monitors`, { credentials: 'include' })
  if (!res.ok) throw new Error(`${res.status}`)
  return res.json()
}

export async function adminCreateMonitor(data: Partial<Monitor>): Promise<Monitor> {
  const res = await fetch(`${BASE}/api/monitors`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminUpdateMonitor(id: number, data: Partial<Monitor>): Promise<Monitor> {
  const res = await fetch(`${BASE}/api/monitors/${id}`, {
    method: 'PUT', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminDeleteMonitor(id: number): Promise<void> {
  await fetch(`${BASE}/api/monitors/${id}`, { method: 'DELETE', credentials: 'include' })
}

export async function adminListGroups(): Promise<MonitorGroup[]> {
  const res = await fetch(`${BASE}/api/groups`, { credentials: 'include' })
  if (!res.ok) return []
  return res.json()
}

export async function adminCreateGroup(data: { name: string; display_order: number; description: string }): Promise<MonitorGroup> {
  const res = await fetch(`${BASE}/api/groups`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminListIncidents(): Promise<Incident[]> {
  const res = await fetch(`${BASE}/api/incidents`, { credentials: 'include' })
  if (!res.ok) return []
  return res.json()
}

export async function adminCreateIncident(data: {
  title: string; severity: string; affected_monitor_ids: string
}): Promise<Incident> {
  const res = await fetch(`${BASE}/api/incidents`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminDeleteIncident(id: number): Promise<void> {
  await fetch(`${BASE}/api/incidents/${id}`, { method: 'DELETE', credentials: 'include' })
}

export async function adminUpdateIncidentStatus(id: number, status: string, rca?: string): Promise<Incident> {
  const res = await fetch(`${BASE}/api/incidents/${id}/status`, {
    method: 'PUT', credentials: 'include', headers: ADMIN_HEADERS,
    body: JSON.stringify({ status, rca: rca ?? null })
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminAddIncidentUpdate(incidentId: number, data: {
  status: string; message: string
}): Promise<IncidentUpdate> {
  const res = await fetch(`${BASE}/api/incidents/${incidentId}/updates`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminGetTheme(): Promise<ThemeConfig> {
  const res = await fetch(`${BASE}/api/theme`, { credentials: 'include' })
  if (!res.ok) throw new Error(`${res.status}`)
  return res.json()
}

export async function adminUpdateTheme(data: {
  preset: string; custom_css: string; config_json: string
}): Promise<ThemeConfig> {
  const res = await fetch(`${BASE}/api/theme`, {
    method: 'PUT', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminListNotifications(): Promise<Notification[]> {
  const res = await fetch(`${BASE}/api/notifications`, { credentials: 'include' })
  if (!res.ok) return []
  return res.json()
}

export async function adminCreateNotification(data: {
  channel: string; config_json: string; monitor_id: number | null
}): Promise<Notification> {
  const res = await fetch(`${BASE}/api/notifications`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminDeleteNotification(id: number): Promise<void> {
  await fetch(`${BASE}/api/notifications/${id}`, { method: 'DELETE', credentials: 'include' })
}

export async function adminListAgents(): Promise<InfraAgent[]> {
  const res = await fetch(`${BASE}/api/agents`, { credentials: 'include' })
  if (!res.ok) return []
  return res.json()
}

export async function adminCreateAgent(data: { name: string; host_label: string }): Promise<InfraAgent> {
  const res = await fetch(`${BASE}/api/agents`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS, body: JSON.stringify(data)
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function adminDeleteAgent(id: number): Promise<void> {
  await fetch(`${BASE}/api/agents/${id}`, { method: 'DELETE', credentials: 'include' })
}

export type Overview = {
  overall: 'operational' | 'degraded' | 'outage'
  counts: { up: number; degraded: number; down: number }
  total_monitors: number
  agent_count: number
  active_incidents: Incident[]
  attention: { kind: string; message: string }[]
}
export async function fetchOverview(): Promise<Overview> {
  const res = await fetch(`${BASE}/api/overview`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`)
  return res.json()
}

export type SetupState = { configured: boolean; default_sqlite_path: string }
export async function setupState(): Promise<SetupState> {
  const res = await fetch(`${BASE}/api/setup/state`, { cache: 'no-store' })
  return res.json()
}
export async function runSetup(payload: {
  name: string; email: string; username: string; password: string
  site_name: string; logo_url: string; sqlite_path: string
}): Promise<void> {
  const res = await fetch(`${BASE}/api/setup`, {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  })
  if (!res.ok) throw new Error(await res.text())
}

export type ApiKey = {
  id: number
  name: string
  prefix: string
  scopes: string[]
  last_used_at: string | null
  created_at: string
}
export async function listApiKeys(): Promise<ApiKey[]> {
  const res = await fetch(`${BASE}/api/keys`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`)
  return res.json()
}
export async function createApiKey(name: string, scopes: string[]): Promise<ApiKey & { key: string }> {
  const res = await fetch(`${BASE}/api/keys`, {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, scopes }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
export async function revokeApiKey(id: number): Promise<void> {
  await fetch(`${BASE}/api/keys/${id}`, { method: 'DELETE', credentials: 'include' })
}

export async function getSettings(): Promise<{ retention_days: number }> {
  const res = await fetch(`${BASE}/api/settings`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`); return res.json()
}
export async function updateSettings(retentionDays: number): Promise<void> {
  const res = await fetch(`${BASE}/api/settings`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ retention_days: retentionDays }) })
  if (!res.ok) throw new Error(await res.text())
}

export type Maintenance = { id: number; title: string; description: string; status: string; affected_monitor_ids: number[]; starts_at: string; ends_at: string }
export async function listMaintenance(): Promise<Maintenance[]> {
  const res = await fetch(`${BASE}/api/maintenance`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`); return res.json()
}
export async function createMaintenance(p: { title: string; description: string; affected_monitor_ids: number[]; starts_at: string; ends_at: string; start_now: boolean }): Promise<void> {
  const res = await fetch(`${BASE}/api/maintenance`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(p) })
  if (!res.ok) throw new Error(await res.text())
}
export async function updateMaintenanceStatus(id: number, status: string): Promise<void> {
  await fetch(`${BASE}/api/maintenance/${id}/status`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status }) })
}
export async function deleteMaintenance(id: number): Promise<void> {
  await fetch(`${BASE}/api/maintenance/${id}`, { method: 'DELETE', credentials: 'include' })
}

export type StatusPage = { id: number; domain: string; title: string; is_default: boolean; published: boolean; group_ids: number[] }
export async function listPages(): Promise<StatusPage[]> {
  const res = await fetch(`${BASE}/api/pages`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`${res.status}`); return res.json()
}
export async function createPage(p: { domain: string; title: string; published: boolean; group_ids: number[] }): Promise<void> {
  const res = await fetch(`${BASE}/api/pages`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(p) })
  if (!res.ok) throw new Error(await res.text())
}
export async function updatePage(id: number, p: { domain: string; title: string; published: boolean; group_ids: number[] }): Promise<void> {
  const res = await fetch(`${BASE}/api/pages/${id}`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(p) })
  if (!res.ok) throw new Error(await res.text())
}
export async function deletePage(id: number): Promise<void> {
  await fetch(`${BASE}/api/pages/${id}`, { method: 'DELETE', credentials: 'include' })
}

export type AuthStatus = { needs_setup: boolean; authenticated: boolean; username: string; totp_enabled: boolean }

export async function authStatus(): Promise<AuthStatus> {
  const res = await fetch(`${BASE}/api/auth/status`, { credentials: 'include', cache: 'no-store' })
  return res.json()
}
export async function authSetup(username: string, password: string): Promise<void> {
  const res = await fetch(`${BASE}/api/auth/setup`, {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) throw new Error(await res.text())
}
export async function authLogin(username: string, password: string, code?: string): Promise<'ok' | 'needs_2fa'> {
  const res = await fetch(`${BASE}/api/auth/login`, {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password, code: code ?? '' }),
  })
  if (res.ok) return 'ok'
  if (res.status === 401) {
    try { const b = await res.json(); if (b && b.needs_2fa) return 'needs_2fa' } catch {}
  }
  throw new Error('Invalid username or password')
}
export async function twoFASetup(): Promise<{ secret: string; otpauth_url: string; qr: string }> {
  const res = await fetch(`${BASE}/api/auth/2fa/setup`, { method: 'POST', credentials: 'include' })
  if (!res.ok) throw new Error(await res.text()); return res.json()
}
export async function twoFAEnable(code: string): Promise<void> {
  const res = await fetch(`${BASE}/api/auth/2fa/enable`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code }) })
  if (!res.ok) throw new Error('Invalid code')
}
export async function twoFADisable(): Promise<void> {
  await fetch(`${BASE}/api/auth/2fa/disable`, { method: 'POST', credentials: 'include' })
}
export async function authLogout(): Promise<void> {
  await fetch(`${BASE}/api/auth/logout`, { method: 'POST', credentials: 'include' })
}
