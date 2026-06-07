type IncidentStatus = 'detected' | 'investigating' | 'identified' | 'implementing' | 'monitoring' | 'resolved'

const LABELS: Record<IncidentStatus, string> = {
  detected:      'Detected',
  investigating: 'Investigating',
  identified:    'Identified',
  implementing:  'Implementing Fix',
  monitoring:    'Monitoring',
  resolved:      'Resolved',
}

export function StatusBadge({ status }: { status: IncidentStatus }) {
  return (
    <span className={`inc-badge ${status}`}>
      {LABELS[status] ?? status}
    </span>
  )
}
