'use client'
import { useState, useEffect, useCallback } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { authStatus, authSetup, authLogin, authLogout, type AuthStatus } from '@/lib/api'
import { ConfirmProvider } from '@/components/ConfirmDialog'

const NAV = [
  { href: '/admin',               label: 'Overview' },
  { href: '/admin/pages',         label: 'Pages' },
  { href: '/admin/monitors',      label: 'Monitors' },
  { href: '/admin/incidents',     label: 'Incidents' },
  { href: '/admin/maintenance',   label: 'Maintenance' },
  { href: '/admin/agents',        label: 'Agents' },
  { href: '/admin/theme',         label: 'Theme' },
  { href: '/admin/notifications', label: 'Notifications' },
  { href: '/admin/api-keys',      label: 'API Keys' },
  { href: '/admin/settings',      label: 'Settings' },
]

function Logo({ size = 30 }: { size?: number }) {
  const ring = Math.round(size * 1.3)
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: Math.round(size * 0.36), fontWeight: 800, fontSize: size, letterSpacing: '-0.8px', color: 'var(--text)' }}>
      <span style={{ width: ring, height: ring, borderRadius: '50%', background: '#eef0ff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
        <svg width={ring * 0.62} height={ring * 0.5} viewBox="0 0 40 24" fill="none" stroke="#4f46e5" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M2 12 h8 l4 -8 5 16 4 -10 3 6 h9" />
        </svg>
      </span>
      Pulse
    </span>
  )
}

function AuthScreen({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24, background: 'var(--bg, #fbfbfc)' }}>
      <div style={{ width: '100%', maxWidth: 380, background: 'var(--card-bg, #fff)', border: '1px solid var(--border, #e6e8eb)', borderRadius: 14, padding: '34px 30px', boxShadow: '0 1px 3px rgba(16,24,40,.08), 0 1px 2px rgba(16,24,40,.04)' }}>
        <div style={{ textAlign: 'center', marginBottom: 18 }}>
          <div style={{ marginBottom: 12 }}><Logo size={36} /></div>
          <div style={{ fontSize: 17, fontWeight: 600, color: 'var(--text, #18181b)' }}>{title}</div>
          <div style={{ fontSize: 13, color: 'var(--text-muted, #6b7280)', marginTop: 4 }}>{subtitle}</div>
        </div>
        {children}
      </div>
    </div>
  )
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [code, setCode] = useState('')
  const [need2fa, setNeed2fa] = useState(false)

  const refresh = useCallback(() => {
    authStatus().then(setStatus).catch(() => setStatus({ needs_setup: false, authenticated: false, username: '', totp_enabled: false }))
  }, [])
  useEffect(() => {
    refresh()
    // Re-validate auth when the page is restored from the back/forward cache
    // (otherwise Back after sign-out shows a stale, "logged in" page).
    const onShow = (e: PageTransitionEvent) => { if (e.persisted) refresh() }
    window.addEventListener('pageshow', onShow)
    return () => window.removeEventListener('pageshow', onShow)
  }, [refresh])

  async function submitSetup(e: React.FormEvent) {
    e.preventDefault(); setError('')
    if (password.length < 8) { setError('Password must be at least 8 characters.'); return }
    if (password !== confirm) { setError("Passwords don't match."); return }
    try { await authSetup(username, password); setPassword(''); setConfirm(''); refresh() }
    catch (err) { setError(err instanceof Error ? err.message : 'Setup failed') }
  }
  async function submitLogin(e: React.FormEvent) {
    e.preventDefault(); setError('')
    try {
      const r = await authLogin(username, password, need2fa ? code : undefined)
      if (r === 'needs_2fa') { setNeed2fa(true); return }
      setCode(''); setNeed2fa(false); setPassword(''); refresh()
    }
    catch (err) { setError(err instanceof Error ? err.message : 'Login failed') }
  }
  async function signOut() { await authLogout(); setUsername(''); setPassword(''); setCode(''); setNeed2fa(false); refresh() }

  if (status === null) return null

  if (status.needs_setup) {
    return (
      <AuthScreen title="Welcome to Pulse" subtitle="Create your admin account to get started.">
        <form onSubmit={submitSetup}>
          <div className="form-group"><input className="form-input" placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} autoFocus /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Password (min 8 characters)" value={password} onChange={e => setPassword(e.target.value)} /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Confirm password" value={confirm} onChange={e => setConfirm(e.target.value)} /></div>
          {error && <div className="error-msg">{error}</div>}
          <button type="submit" className="btn primary" style={{ width: '100%', justifyContent: 'center', marginTop: 8 }}>Create account</button>
        </form>
      </AuthScreen>
    )
  }

  if (!status.authenticated) {
    return (
      <AuthScreen title="Welcome back" subtitle="Sign in to continue.">
        <form onSubmit={submitLogin}>
          <div className="form-group"><input className="form-input" placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} autoFocus={!need2fa} /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)} /></div>
          {need2fa && (
            <div className="form-group">
              <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 6, color: 'var(--text-muted, #6b7280)' }}>Authenticator code</label>
              <input
                className="form-input"
                placeholder="6-digit code"
                value={code}
                onChange={e => setCode(e.target.value)}
                maxLength={6}
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
              />
            </div>
          )}
          {error && <div className="error-msg">{error}</div>}
          <button type="submit" className="btn primary" style={{ width: '100%', justifyContent: 'center', marginTop: 8 }}>
            {need2fa ? 'Verify' : 'Sign in'}
          </button>
        </form>
      </AuthScreen>
    )
  }

  return (
    <div className="admin-layout">
      <aside className="admin-sidebar">
        <div className="admin-sidebar-title"><Logo size={20} /></div>
        {NAV.map(n => (
          <Link key={n.href} href={n.href} className={`admin-nav-link${pathname === n.href ? ' active' : ''}`}>{n.label}</Link>
        ))}
        <button className="admin-nav-link" style={{ border: 'none', background: 'none', cursor: 'pointer', width: '100%', textAlign: 'left', marginTop: 20, color: 'var(--down)' }} onClick={signOut}>
          Sign out{status.username ? ` (${status.username})` : ''}
        </button>
      </aside>
      <div className="admin-content">
        <ConfirmProvider>{children}</ConfirmProvider>
      </div>
    </div>
  )
}
