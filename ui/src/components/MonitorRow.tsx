'use client'
import { useEffect, useState } from 'react'
import type { Monitor, CheckResult } from '@/lib/types'
import { LatencyBars } from './LatencyBars'
import { useRange } from './RangeFilter'
import { fetchCheckResults } from '@/lib/api'

interface Props { monitor: Monitor; latestStatus: 'up' | 'down' | 'degraded' }

export function MonitorRow({ monitor, latestStatus }: Props) {
  const { currentOpt } = useRange()
  const [results, setResults] = useState<CheckResult[]>([])

  useEffect(() => {
    fetchCheckResults(monitor.id, currentOpt.days).then(setResults).catch(() => {})
  }, [monitor.id, currentOpt.days])

  const avgMs = results.length > 0
    ? Math.round(results.reduce((s, r) => s + (r.response_time_ms ?? 0), 0) / results.length)
    : null
  const upCount = results.filter(r => r.status === 'up').length
  const upPct = results.length > 0 ? ((upCount / results.length) * 100).toFixed(1) : null
  const pctClass = !upPct ? '' : parseFloat(upPct) >= 99 ? '' : parseFloat(upPct) >= 95 ? ' warn' : ' down'

  // SSL chip: statusCode from check results holds days-until-expiry for SSL monitors
  const sslResult = monitor.type === 'ssl' ? results[0] : null
  const sslDays = sslResult?.status_code ?? null
  const sslClass = sslDays === null ? '' : sslDays <= 7 ? 'ssl-crit' : sslDays <= 14 ? 'ssl-warn' : 'ssl-ok'

  return (
    <div className="monitor-row">
      <div className={`m-dot${latestStatus !== 'up' ? ` ${latestStatus}` : ''}`} />
      <span className="m-name">{monitor.name}</span>
      {monitor.type === 'ssl' && sslDays !== null && (
        <span className={`m-tag ${sslClass}`}>SSL {sslDays}d</span>
      )}
      {monitor.type === 'infra' && (
        <span className="m-tag">infra</span>
      )}
      <div className="bars-wrap">
        <LatencyBars results={results} count={currentOpt.bars} days={currentOpt.days} />
        <div className="bars-range">
          <span>{currentOpt.days === 1 ? '00:00' : `${currentOpt.days}d ago`}</span>
          <span>now</span>
        </div>
      </div>
      {avgMs !== null && <span className="m-ms">{avgMs}ms</span>}
      {upPct !== null && <span className={`m-pct${pctClass}`}>{upPct}%</span>}
    </div>
  )
}
