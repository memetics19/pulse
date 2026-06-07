'use client'
import { useState, useEffect } from 'react'
import { setupState, runSetup } from '@/lib/api'

function Logo({ size = 34 }: { size?: number }) {
  const ring = Math.round(size * 1.3)
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: Math.round(size * 0.36), fontWeight: 800, fontSize: size, letterSpacing: '-0.6px', color: '#15171c' }}>
      <span style={{ width: ring, height: ring, borderRadius: '50%', background: '#eef0ff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}>
        <svg width={ring * 0.62} height={ring * 0.5} viewBox="0 0 40 24" fill="none" stroke="#4f46e5" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><path d="M2 12 h8 l4 -8 5 16 4 -10 3 6 h9" /></svg>
      </span>
      Pulse
    </span>
  )
}

const STEPS = ['Account', 'Branding', 'Database']

export default function SetupWizard() {
  const [step, setStep] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [siteName, setSiteName] = useState('')
  const [logoURL, setLogoURL] = useState('')
  const [sqlitePath, setSqlitePath] = useState('')

  useEffect(() => {
    setupState().then(s => {
      if (s.configured) { window.location.href = '/admin/'; return }
      setSqlitePath(s.default_sqlite_path)
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  function next() {
    setError('')
    if (step === 0) {
      if (!username || password.length < 8) { setError('Username and a password of at least 8 characters are required.'); return }
      if (password !== confirm) { setError("Passwords don't match."); return }
    }
    setStep(s => Math.min(s + 1, STEPS.length - 1))
  }
  function back() { setError(''); setStep(s => Math.max(s - 1, 0)) }

  async function finish() {
    setError('')
    try {
      await runSetup({ name, email, username, password, site_name: siteName, logo_url: logoURL, sqlite_path: sqlitePath })
      window.location.href = '/admin/'
    } catch (e) { setError(e instanceof Error ? e.message : 'Setup failed') }
  }

  if (loading) return null

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24, background: '#fbfbfc' }}>
      <div style={{ width: '100%', maxWidth: 440, background: '#fff', border: '1px solid #e6e8eb', borderRadius: 14, padding: '30px 30px 26px', boxShadow: '0 1px 3px rgba(16,24,40,.08),0 1px 2px rgba(16,24,40,.04)' }}>
        <div style={{ textAlign: 'center', marginBottom: 18 }}><Logo /></div>

        <div style={{ display: 'flex', gap: 8, marginBottom: 22 }}>
          {STEPS.map((s, i) => (
            <div key={s} style={{ flex: 1, textAlign: 'center' }}>
              <div style={{ height: 4, borderRadius: 2, background: i <= step ? '#4f46e5' : '#e6e8eb', marginBottom: 6 }} />
              <div style={{ fontSize: 11, fontWeight: 600, color: i <= step ? '#4f46e5' : '#9aa0a8' }}>{s}</div>
            </div>
          ))}
        </div>

        {step === 0 && (<>
          <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 14 }}>Create your admin account</div>
          <div className="form-group"><input className="form-input" placeholder="Full name" value={name} onChange={e => setName(e.target.value)} /></div>
          <div className="form-group"><input className="form-input" type="email" placeholder="Email" value={email} onChange={e => setEmail(e.target.value)} /></div>
          <div className="form-group"><input className="form-input" placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} autoFocus /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Password (min 8 characters)" value={password} onChange={e => setPassword(e.target.value)} /></div>
          <div className="form-group"><input className="form-input" type="password" placeholder="Confirm password" value={confirm} onChange={e => setConfirm(e.target.value)} /></div>
        </>)}

        {step === 1 && (<>
          <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 14 }}>Branding</div>
          <div className="form-group"><input className="form-input" placeholder="Status page name (e.g. Acme Status)" value={siteName} onChange={e => setSiteName(e.target.value)} /></div>
          <div className="form-group">
            <label className="form-label">Logo</label>
            {logoURL && <div style={{ margin: '6px 0' }}><img src={logoURL} alt="logo" style={{ height: 38, maxWidth: 160, objectFit: 'contain', border: '1px solid #e6e8eb', borderRadius: 6, padding: 6 }} /></div>}
            <input type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp" id="wiz-logo" style={{ display: 'none' }} onChange={e => { const f = e.target.files?.[0]; if (!f) return; const rd = new FileReader(); rd.onload = ev => setLogoURL(ev.target?.result as string); rd.readAsDataURL(f); e.target.value = '' }} />
            <label htmlFor="wiz-logo" className="btn" style={{ cursor: 'pointer', display: 'inline-flex' }}>{logoURL ? 'Replace logo' : 'Upload logo'}</label>
          </div>
        </>)}

        {step === 2 && (<>
          <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 14 }}>Database</div>
          <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start', border: '1px solid #4f46e5', borderRadius: 8, padding: '10px 12px', marginBottom: 10 }}>
            <input type="radio" checked readOnly />
            <span><div style={{ fontWeight: 600, fontSize: 14 }}>SQLite <span style={{ color: '#6b7280', fontWeight: 400 }}>· recommended</span></div><div style={{ fontSize: 12, color: '#6b7280' }}>A single file, zero setup. Perfect for self-hosting.</div></span>
          </label>
          <div className="form-group"><label className="form-label">Database file path</label><input className="form-input" value={sqlitePath} onChange={e => setSqlitePath(e.target.value)} /></div>
          <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start', border: '1px solid #e6e8eb', borderRadius: 8, padding: '10px 12px', opacity: .6 }}>
            <input type="radio" disabled />
            <span><div style={{ fontWeight: 600, fontSize: 14 }}>PostgreSQL <span style={{ color: '#9aa0a8', fontWeight: 400 }}>· coming soon</span></div><div style={{ fontSize: 12, color: '#9aa0a8' }}>External Postgres support is on the roadmap.</div></span>
          </label>
        </>)}

        {error && <div className="error-msg" style={{ marginTop: 6 }}>{error}</div>}

        <div style={{ display: 'flex', gap: 10, marginTop: 18 }}>
          {step > 0 && <button className="btn" onClick={back}>Back</button>}
          {step < STEPS.length - 1
            ? <button className="btn primary" style={{ flex: 1, justifyContent: 'center' }} onClick={next}>Continue</button>
            : <button className="btn primary" style={{ flex: 1, justifyContent: 'center' }} onClick={finish}>Finish setup</button>}
        </div>
      </div>
    </div>
  )
}
