'use client'
import type { Monitor, MonitorGroup } from '@/lib/types'
import { MonitorRow } from './MonitorRow'

type Status = 'up' | 'down' | 'degraded'
interface Props { group: MonitorGroup; monitors: Monitor[]; latestStatuses: Record<number, Status> }

function groupStatus(monitors: Monitor[], statuses: Record<number, Status>): Status {
  const ss = monitors.map(m => statuses[m.id] ?? 'up')
  if (ss.some(s => s === 'down')) return 'down'
  if (ss.some(s => s === 'degraded')) return 'degraded'
  return 'up'
}

const STATUS_LABEL: Record<Status, string> = {
  up: 'Operational', degraded: 'Degraded', down: 'Partial Outage',
}

export function MonitorGroupCard({ group, monitors, latestStatuses }: Props) {
  const status = groupStatus(monitors, latestStatuses)
  return (
    <div className="group-card">
      <div className="group-header">
        <span className="group-name">{group.name}</span>
        <span className={`status-pill ${status}`}>{STATUS_LABEL[status]}</span>
      </div>
      {monitors.map(m => (
        <MonitorRow key={m.id} monitor={m} latestStatus={latestStatuses[m.id] ?? 'up'} />
      ))}
    </div>
  )
}

export function UngroupedMonitors({ monitors, latestStatuses }: Omit<Props, 'group'>) {
  if (monitors.length === 0) return null
  const status = groupStatus(monitors, latestStatuses)
  return (
    <div className="group-card">
      <div className="group-header">
        <span className="group-name">Services</span>
        <span className={`status-pill ${status}`}>{STATUS_LABEL[status]}</span>
      </div>
      {monitors.map(m => (
        <MonitorRow key={m.id} monitor={m} latestStatus={latestStatuses[m.id] ?? 'up'} />
      ))}
    </div>
  )
}
