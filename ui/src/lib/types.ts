// src/lib/types.ts

export interface Monitor {
  id: number
  name: string
  url: string
  type: 'http' | 'tcp' | 'ping' | 'dns' | 'ssl' | 'infra'
  interval_seconds: number
  timeout_seconds: number
  expected_status: number | null
  keyword_check: string
  degraded_threshold_ms: number
  down_threshold_ms: number
  is_active: number
  group_id: number | null
  source: string
  external_id: string
  created_at: string
}

export interface MonitorGroup {
  id: number
  name: string
  display_order: number
  description: string
  created_at: string
}

export interface CheckResult {
  id: number
  monitor_id: number
  checked_at: string
  status: 'up' | 'down' | 'degraded'
  response_time_ms: number | null
  status_code: number | null
  error_message: string
}

export interface Incident {
  id: number
  title: string
  severity: 'minor' | 'major' | 'critical'
  status: 'detected' | 'investigating' | 'identified' | 'implementing' | 'monitoring' | 'resolved'
  affected_monitor_ids: string
  started_at: string
  resolved_at: string | null
  rca: string | null
  source: string
  external_id: string
  created_at: string
}

export interface IncidentUpdate {
  id: number
  incident_id: number
  status: string
  message: string
  author: string
  created_at: string
}

export interface ThemeConfig {
  id: number
  preset: string
  custom_css: string
  config_json: string
  updated_at: string
}

export interface Notification {
  id: number
  channel: 'email' | 'slack'
  config_json: string
  monitor_id: number | null
  created_at: string
}

export interface InfraAgent {
  id: number
  name: string
  host_label: string
  token: string
  last_seen_at: string | null
  is_active: number
  created_at: string
}

export interface StatusResponse {
  groups: MonitorGroup[]
  monitors: Monitor[]
  incidents: Incident[]
  theme: ThemeConfig
}

export type OverallStatus = 'operational' | 'degraded' | 'outage'
