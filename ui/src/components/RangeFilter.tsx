'use client'
import { createContext, useContext, useState } from 'react'

export type Range = '90d' | '60d' | '30d' | '15d' | '7d' | 'yesterday' | 'today'

export const RANGE_OPTIONS = [
  { key: '90d'       as Range, label: '90d',       bars: 90, days: 90 },
  { key: '60d'       as Range, label: '60d',       bars: 60, days: 60 },
  { key: '30d'       as Range, label: '30d',       bars: 30, days: 30 },
  { key: '15d'       as Range, label: '15d',       bars: 30, days: 15 },
  { key: '7d'        as Range, label: '1w',        bars: 28, days: 7  },
  { key: 'yesterday' as Range, label: 'Yesterday', bars: 24, days: 2  },
  { key: 'today'     as Range, label: 'Today',     bars: 24, days: 1  },
]

type RangeOpt = typeof RANGE_OPTIONS[0]
interface RangeCtx { range: Range; setRange: (r: Range) => void; currentOpt: RangeOpt }

const Ctx = createContext<RangeCtx>({
  range: 'today',
  setRange: () => {},
  currentOpt: RANGE_OPTIONS[6],
})

export function RangeProvider({ children }: { children: React.ReactNode }) {
  const [range, setRange] = useState<Range>('today')
  const currentOpt = RANGE_OPTIONS.find(o => o.key === range) ?? RANGE_OPTIONS[6]
  return <Ctx.Provider value={{ range, setRange, currentOpt }}>{children}</Ctx.Provider>
}

export function useRange() { return useContext(Ctx) }

export function RangeFilterBar() {
  const { range, setRange, currentOpt } = useRange()
  return (
    <div className="toolbar">
      <div className="toolbar-left">
        <span className="toolbar-label">Range</span>
        <div className="seg-group">
          {RANGE_OPTIONS.map(o => (
            <button
              key={o.key}
              className={`seg-btn${range === o.key ? ' active' : ''}`}
              onClick={() => setRange(o.key)}
            >
              {o.label}
            </button>
          ))}
        </div>
      </div>
      <div className="toolbar-right">
        {currentOpt.days === 1 ? 'today 00:00' : `${currentOpt.days}d ago`} — now
      </div>
    </div>
  )
}
