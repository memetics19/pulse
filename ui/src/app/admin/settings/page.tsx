'use client'
import { useState, useEffect, useCallback } from 'react'
import { getSettings, updateSettings, authStatus, twoFASetup, twoFAEnable, twoFADisable } from '@/lib/api'

export default function SettingsPage() {
  const [retentionDays, setRetentionDays] = useState(90)
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState('')

  // 2FA state
  const [totpEnabled, setTotpEnabled] = useState(false)
  const [setupData, setSetupData] = useState<{ secret: string; otpauth_url: string; qr: string } | null>(null)
  const [totpCode, setTotpCode] = useState('')
  const [totpError, setTotpError] = useState('')
  const [totpSuccess, setTotpSuccess] = useState(false)
  const [totpLoading, setTotpLoading] = useState(false)

  const refreshAuth = useCallback(() => {
    authStatus()
      .then(s => setTotpEnabled(s.totp_enabled ?? false))
      .catch(() => {})
  }, [])

  useEffect(() => {
    getSettings()
      .then(s => setRetentionDays(s.retention_days))
      .catch(() => {/* keep default */})
    refreshAuth()
  }, [refreshAuth])

  async function handleSave() {
    setStatus('saving')
    setErrorMsg('')
    try {
      await updateSettings(retentionDays)
      setStatus('saved')
      setTimeout(() => setStatus('idle'), 2500)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : 'Save failed')
      setStatus('error')
    }
  }

  async function handleSetup2FA() {
    setTotpLoading(true)
    setTotpError('')
    try {
      const data = await twoFASetup()
      setSetupData(data)
      setTotpCode('')
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'Setup failed')
    } finally {
      setTotpLoading(false)
    }
  }

  async function handleEnable2FA() {
    setTotpLoading(true)
    setTotpError('')
    try {
      await twoFAEnable(totpCode)
      setTotpSuccess(true)
      setSetupData(null)
      setTotpCode('')
      refreshAuth()
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'Invalid code')
    } finally {
      setTotpLoading(false)
    }
  }

  async function handleDisable2FA() {
    setTotpLoading(true)
    setTotpError('')
    setTotpSuccess(false)
    try {
      await twoFADisable()
      refreshAuth()
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'Disable failed')
    } finally {
      setTotpLoading(false)
    }
  }

  return (
    <>
      <h1 className="admin-title">Settings</h1>
      <div style={{ display: 'grid', gap: 16, maxWidth: 640 }}>
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 6 }}>Instance</div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', lineHeight: 1.6 }}>
            Pulse — single-binary status page. Your admin account, branding, and database
            were configured during first-run setup. Branding (logo, site name, theme) can be
            changed under <a href="/admin/theme">Theme</a>.
          </p>
        </div>
        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 6 }}>Data retention</div>
          <div className="form-group" style={{ marginTop: 12, marginBottom: 0 }}>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 6 }}>
              Keep monitor history and agent diagnostics for
            </label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <input
                type="number"
                min={1}
                className="form-input"
                style={{ width: 120 }}
                value={retentionDays}
                onChange={e => {
                  const v = parseInt(e.target.value, 10)
                  if (!isNaN(v) && v >= 1) setRetentionDays(v)
                }}
              />
              <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>days</span>
            </div>
            <p style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 6, lineHeight: 1.5 }}>
              Check results older than this are pruned. Public uptime bars can only go back as far as this window.
            </p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 14 }}>
            <button
              className="btn primary"
              onClick={handleSave}
              disabled={status === 'saving'}
            >
              {status === 'saving' ? 'Saving…' : 'Save'}
            </button>
            {status === 'saved' && (
              <span style={{ fontSize: 13, color: 'var(--color-up, #22c55e)' }}>Saved ✓</span>
            )}
            {status === 'error' && (
              <span style={{ fontSize: 13, color: 'var(--color-down, #ef4444)' }}>{errorMsg}</span>
            )}
          </div>
        </div>

        <div className="card">
          <div style={{ fontWeight: 600, marginBottom: 6 }}>Two-factor authentication</div>

          {totpEnabled ? (
            <>
              <p style={{ fontSize: 13, color: 'var(--text-muted)', lineHeight: 1.6, marginBottom: 14 }}>
                Two-factor authentication is on.
              </p>
              {totpError && (
                <p style={{ fontSize: 13, color: 'var(--color-down, #ef4444)', marginBottom: 10 }}>{totpError}</p>
              )}
              <button className="btn danger" onClick={handleDisable2FA} disabled={totpLoading}>
                {totpLoading ? 'Disabling…' : 'Disable'}
              </button>
            </>
          ) : (
            <>
              {!setupData && (
                <>
                  <p style={{ fontSize: 13, color: 'var(--text-muted)', lineHeight: 1.6, marginBottom: 14 }}>
                    Add an authenticator app for an extra layer of security.
                  </p>
                  {totpSuccess && (
                    <p style={{ fontSize: 13, color: 'var(--color-up, #22c55e)', marginBottom: 10 }}>2FA enabled ✓</p>
                  )}
                  {totpError && (
                    <p style={{ fontSize: 13, color: 'var(--color-down, #ef4444)', marginBottom: 10 }}>{totpError}</p>
                  )}
                  <button className="btn primary" onClick={handleSetup2FA} disabled={totpLoading}>
                    {totpLoading ? 'Loading…' : 'Enable 2FA'}
                  </button>
                </>
              )}

              {setupData && (
                <div style={{ marginTop: 4 }}>
                  <p style={{ fontSize: 13, color: 'var(--text-muted)', lineHeight: 1.6, marginBottom: 14 }}>
                    Scan this QR code with your authenticator app, then enter the 6-digit code to confirm.
                  </p>
                  <div style={{ marginBottom: 16 }}>
                    <img
                      src={setupData.qr}
                      alt="Scan with your authenticator app"
                      style={{ width: 180, height: 180, display: 'block', borderRadius: 8, border: '1px solid var(--border, #e6e8eb)' }}
                    />
                  </div>
                  <div style={{ marginBottom: 16 }}>
                    <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 6, fontWeight: 500 }}>
                      Or enter this code manually
                    </div>
                    <code style={{
                      display: 'block',
                      fontFamily: 'monospace',
                      fontSize: 13,
                      background: 'var(--bg, #fbfbfc)',
                      border: '1px solid var(--border, #e6e8eb)',
                      borderRadius: 6,
                      padding: '8px 12px',
                      letterSpacing: '0.08em',
                      userSelect: 'all',
                      wordBreak: 'break-all',
                    }}>
                      {setupData.secret}
                    </code>
                  </div>
                  <div className="form-group" style={{ marginBottom: 12 }}>
                    <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 6 }}>
                      Authenticator code
                    </label>
                    <input
                      className="form-input"
                      placeholder="6-digit code"
                      value={totpCode}
                      onChange={e => setTotpCode(e.target.value)}
                      maxLength={6}
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      style={{ width: 160 }}
                    />
                  </div>
                  {totpError && (
                    <p style={{ fontSize: 13, color: 'var(--color-down, #ef4444)', marginBottom: 10 }}>{totpError}</p>
                  )}
                  <button className="btn primary" onClick={handleEnable2FA} disabled={totpLoading || totpCode.length < 6}>
                    {totpLoading ? 'Verifying…' : 'Verify & enable'}
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </>
  )
}
