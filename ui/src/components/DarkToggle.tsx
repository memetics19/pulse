'use client'
import { useEffect, useState } from 'react'

type Theme = 'default-light' | 'default-dark' | 'terminal-dark'

export function DarkToggle() {
  const [theme, setTheme] = useState<Theme>('default-light')

  useEffect(() => {
    const saved = (localStorage.getItem('pulse-theme') as Theme) ?? 'default-light'
    setTheme(saved)
    document.documentElement.setAttribute('data-theme', saved)
  }, [])

  function toggle() {
    const next: Theme = theme === 'default-light' ? 'default-dark' : 'default-light'
    setTheme(next)
    localStorage.setItem('pulse-theme', next)
    document.documentElement.setAttribute('data-theme', next)
  }

  const isDark = theme !== 'default-light'
  return (
    <button className="dark-toggle" onClick={toggle} aria-label="Toggle dark mode">
      <span>{isDark ? '☀️' : '🌙'}</span>
      <div className={`toggle-track ${isDark ? 'on' : ''}`}>
        <div className={`toggle-knob ${isDark ? 'on' : ''}`} />
      </div>
    </button>
  )
}
