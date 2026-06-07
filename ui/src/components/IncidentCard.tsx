'use client'
import { useEffect, useState } from 'react'
import type { Incident, IncidentUpdate } from '@/lib/types'
import { StatusBadge } from './StatusBadge'
import { fetchIncidentUpdates } from '@/lib/api'

function formatLocal(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

function dotClass(status: string, isLast: boolean): string {
  if (status === 'detected') return 'start'
  if (status === 'resolved') return 'done'
  return isLast ? 'active' : ''
}

export function IncidentCard({ incident }: { incident: Incident }) {
  const [updates, setUpdates] = useState<IncidentUpdate[]>([])

  useEffect(() => {
    fetchIncidentUpdates(incident.id).then(setUpdates).catch(() => {})
  }, [incident.id])

  const isResolved = incident.status === 'resolved'
  const duration = incident.resolved_at
    ? Math.round((new Date(incident.resolved_at).getTime() - new Date(incident.started_at).getTime()) / 60000)
    : null

  return (
    <div className="incident-card">
      <div className={`incident-header${isResolved ? ' resolved' : ''}`}>
        <StatusBadge status={incident.status} />
        <div>
          <div className="inc-title">{incident.title}</div>
          <div className="inc-meta">
            {formatLocal(incident.started_at)}
            {duration !== null && ` · Duration ${duration}m`}
            {` · ${incident.severity}`}
          </div>
        </div>
      </div>

      {updates.length > 0 && (
        <div className="incident-body">
          {updates.map((u, i) => (
            <div className="tl-row" key={u.id}>
              <div className={`tl-dot ${dotClass(u.status, i === updates.length - 1)}`} />
              <span className="tl-time">{formatLocal(u.created_at)}</span>
              <div>
                <div className={`tl-status ${u.status}`}>{u.status}</div>
                <div className="tl-msg">{u.message}</div>
              </div>
            </div>
          ))}

          {incident.rca && (
            <div className="rca-block">
              <div className="rca-label">Root Cause Analysis</div>
              <div className="rca-text">{incident.rca}</div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
