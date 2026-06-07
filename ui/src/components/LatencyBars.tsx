'use client'
import { useState } from 'react'
import type { CheckResult } from '@/lib/types'

interface Props {
  results: CheckResult[]
  count: number
  days: number
}

interface Bar {
  status: 'up' | 'down' | 'degraded' | 'none'
  tooltip: string
  height: number
}

function buildBars(results: CheckResult[], days: number, count: number): Bar[] {
  const now = Date.now()
  const start = now - days * 24 * 60 * 60 * 1000
  const slotMs = (now - start) / count

  const bars: Bar[] = Array.from({ length: count }, () => ({
    status: 'none' as const,
    tooltip: '',
    height: 40,
  }))

  for (const r of results) {
    const t = new Date(r.checked_at).getTime()
    if (t < start || t > now) continue
    const slot = Math.min(count - 1, Math.floor((t - start) / slotMs))
    const ms = r.response_time_ms
    bars[slot] = {
      status: r.status as 'up' | 'down' | 'degraded',
      tooltip: `${new Date(r.checked_at).toLocaleString()}${ms != null ? `\n${ms}ms` : ''}`,
      height: r.status === 'down' ? 100 : r.status === 'degraded' ? 70 : ms != null ? Math.min(100, Math.max(20, ms / 10)) : 40,
    }
  }

  return bars
}

export function LatencyBars({ results, count, days }: Props) {
  const [tooltip, setTooltip] = useState<{ text: string; x: number; y: number } | null>(null)
  const bars = buildBars(results, days, count)
  const barW = Math.max(2, Math.floor(180 / count))

  return (
    <div style={{ position: 'relative' }}>
      <div className="bars">
        {bars.map((b, i) => (
          <div
            key={i}
            className={`bar${b.status !== 'none' && b.status !== 'up' ? ` ${b.status}` : ''}`}
            style={{
              width: barW,
              height: `${b.height}%`,
              background: b.status === 'none' ? 'var(--border)' : undefined,
              opacity: b.status === 'none' ? 0.4 : 1,
              cursor: b.status !== 'none' ? 'pointer' : 'default',
            }}
            onMouseEnter={b.status !== 'none' ? (e) => {
              const rect = (e.target as HTMLElement).getBoundingClientRect()
              setTooltip({ text: b.tooltip, x: rect.left, y: rect.top })
            } : undefined}
            onMouseLeave={() => setTooltip(null)}
          />
        ))}
      </div>

      {tooltip && (
        <div style={{
          position: 'fixed',
          left: tooltip.x,
          top: tooltip.y - 52,
          background: 'var(--bg2)',
          border: '1px solid var(--border)',
          borderRadius: 4,
          padding: '4px 8px',
          fontSize: 11,
          fontFamily: 'var(--font-mono)',
          color: 'var(--text)',
          whiteSpace: 'pre',
          zIndex: 50,
          pointerEvents: 'none',
          boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
        }}>
          {tooltip.text}
        </div>
      )}
    </div>
  )
}
