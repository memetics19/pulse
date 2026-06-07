'use client'
import { useState, useEffect } from 'react'
import { adminGetTheme, adminUpdateTheme } from '@/lib/api'

const PRESETS = ['default-light', 'default-dark', 'terminal-dark']

interface FooterLink {
  label: string
  url: string
}

export default function ThemePage() {
  const [preset,        setPreset]        = useState('default-light')
  const [customCSS,     setCustomCSS]     = useState('')
  const [siteName,      setSiteName]      = useState('Status')
  const [logoURL,       setLogoURL]       = useState('')
  const [faviconURL,    setFaviconURL]    = useState('')
  const [saved,         setSaved]         = useState(false)
  const [error,         setError]         = useState('')

  // Footer state
  const [brandLine,      setBrandLine]      = useState('')
  const [tagline,        setTagline]        = useState('')
  const [copyright,      setCopyright]      = useState('')
  const [hidePoweredBy,  setHidePoweredBy]  = useState(false)
  const [footerLinks,    setFooterLinks]    = useState<FooterLink[]>([])

  useEffect(() => {
    adminGetTheme().then(t => {
      setPreset(t.preset)
      setCustomCSS(t.custom_css)
      try {
        const cfg = JSON.parse(t.config_json ?? '{}')
        setSiteName(cfg.site_name ?? 'Status')
        setLogoURL(cfg.logo_url ?? '')
        setFaviconURL(cfg.favicon_url ?? '')
        const f = cfg.footer ?? {}
        setBrandLine(f.brand_line ?? '')
        setTagline(f.tagline ?? '')
        setCopyright(f.copyright ?? '')
        setHidePoweredBy(f.hide_powered_by ?? false)
        setFooterLinks(Array.isArray(f.links) ? f.links : [])
      } catch {}
    }).catch(() => {})
  }, [])

  async function save() {
    setError(''); setSaved(false)
    try {
      await adminUpdateTheme({
        preset,
        custom_css: customCSS,
        config_json: JSON.stringify({
          site_name: siteName,
          logo_url: logoURL,
          favicon_url: faviconURL,
          footer: {
            brand_line: brandLine,
            tagline,
            copyright,
            hide_powered_by: hidePoweredBy,
            links: footerLinks.filter(l => l.label && l.url),
          },
        }),
      })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
  }

  function updateLink(index: number, field: keyof FooterLink, value: string) {
    setFooterLinks(prev => prev.map((l, i) => i === index ? { ...l, [field]: value } : l))
  }

  function removeLink(index: number) {
    setFooterLinks(prev => prev.filter((_, i) => i !== index))
  }

  function addLink() {
    setFooterLinks(prev => [...prev, { label: '', url: '' }])
  }

  return (
    <>
      <h1 className="admin-title">Theme</h1>
      <div style={{ maxWidth: 560 }}>
        <div className="form-group">
          <label className="form-label">Site Name</label>
          <input className="form-input" value={siteName} onChange={e => setSiteName(e.target.value)} placeholder="Acme Corp Status" />
        </div>
        <div className="form-group">
          <label className="form-label">Logo</label>
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 }}>
            PNG, SVG, or JPEG · Displayed in the top-left of the status page
          </p>
          {logoURL && (
            <div style={{ marginBottom: 10, display: 'flex', alignItems: 'center', gap: 12 }}>
              <img src={logoURL} alt="Logo" style={{ height: 36, maxWidth: 160, objectFit: 'contain', border: '1px solid var(--border)', borderRadius: 6, padding: 6, background: 'var(--bg2)' }} />
              <button
                className="btn danger"
                style={{ fontSize: 12, padding: '4px 10px' }}
                onClick={() => setLogoURL('')}
                type="button"
              >
                Remove
              </button>
            </div>
          )}
          <input
            type="file"
            accept="image/png,image/jpeg,image/svg+xml,image/webp"
            style={{ display: 'none' }}
            id="logo-upload"
            onChange={e => {
              const file = e.target.files?.[0]
              if (!file) return
              const reader = new FileReader()
              reader.onload = ev => setLogoURL(ev.target?.result as string)
              reader.readAsDataURL(file)
              e.target.value = ''
            }}
          />
          <label htmlFor="logo-upload" className="btn" style={{ cursor: 'pointer', display: 'inline-flex' }}>
            {logoURL ? '↑ Replace logo' : '↑ Upload logo'}
          </label>
        </div>
        <div className="form-group">
          <label className="form-label">Favicon</label>
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 }}>
            PNG, ICO, or SVG · Shown in the browser tab. Square images work best (32×32 or 64×64).
          </p>
          {faviconURL && (
            <div style={{ marginBottom: 10, display: 'flex', alignItems: 'center', gap: 12 }}>
              <img src={faviconURL} alt="Favicon" style={{ height: 32, width: 32, objectFit: 'contain', border: '1px solid var(--border)', borderRadius: 6, padding: 4, background: 'var(--bg2)' }} />
              <button
                className="btn danger"
                style={{ fontSize: 12, padding: '4px 10px' }}
                onClick={() => setFaviconURL('')}
                type="button"
              >
                Remove
              </button>
            </div>
          )}
          <input
            type="file"
            accept="image/png,image/x-icon,image/vnd.microsoft.icon,image/svg+xml"
            style={{ display: 'none' }}
            id="favicon-upload"
            onChange={e => {
              const file = e.target.files?.[0]
              if (!file) return
              const reader = new FileReader()
              reader.onload = ev => setFaviconURL(ev.target?.result as string)
              reader.readAsDataURL(file)
              e.target.value = ''
            }}
          />
          <label htmlFor="favicon-upload" className="btn" style={{ cursor: 'pointer', display: 'inline-flex' }}>
            {faviconURL ? '↑ Replace favicon' : '↑ Upload favicon'}
          </label>
        </div>
        <div className="form-group">
          <label className="form-label">Preset</label>
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', marginTop: 4 }}>
            {PRESETS.map(p => (
              <button key={p} className={`btn${preset === p ? ' primary' : ''}`} onClick={() => setPreset(p)}>
                {p}
              </button>
            ))}
          </div>
        </div>
        <div className="form-group">
          <label className="form-label">CSS Variable Overrides</label>
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 6 }}>
            Override any CSS var. Example: <code>--accent: #e11d48;</code>
          </p>
          <textarea
            className="form-textarea"
            style={{ minHeight: 100, fontFamily: 'var(--font-mono)' }}
            value={customCSS}
            onChange={e => setCustomCSS(e.target.value)}
            placeholder="--accent: #e11d48;"
          />
        </div>

        {/* Footer section */}
        <div style={{ borderTop: '1px solid var(--border)', marginTop: 8, paddingTop: 24, marginBottom: 8 }}>
          <h2 style={{ fontSize: 15, fontWeight: 600, marginBottom: 4, color: 'var(--text)' }}>Footer</h2>
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 20 }}>
            Shown in the footer of your public status page.
          </p>
          <div className="form-group">
            <label className="form-label">Brand line</label>
            <input
              className="form-input"
              value={brandLine}
              onChange={e => setBrandLine(e.target.value)}
              placeholder="Acme Inc."
            />
          </div>
          <div className="form-group">
            <label className="form-label">Tagline</label>
            <input
              className="form-input"
              value={tagline}
              onChange={e => setTagline(e.target.value)}
              placeholder="Real-time status for Acme."
            />
          </div>
          <div className="form-group">
            <label className="form-label">Copyright / legal line</label>
            <input
              className="form-input"
              value={copyright}
              onChange={e => setCopyright(e.target.value)}
              placeholder="© 2026 Acme Inc."
            />
          </div>
          <div className="form-group">
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer', userSelect: 'none' }}>
              <input
                type="checkbox"
                checked={hidePoweredBy}
                onChange={e => setHidePoweredBy(e.target.checked)}
                style={{ width: 15, height: 15, accentColor: 'var(--accent)', cursor: 'pointer' }}
              />
              <span className="form-label" style={{ margin: 0 }}>Hide &ldquo;Powered by Pulse&rdquo;</span>
            </label>
          </div>
          <div className="form-group">
            <label className="form-label">Footer links</label>
            {footerLinks.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 10 }}>
                {footerLinks.map((link, i) => (
                  <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <input
                      className="form-input"
                      style={{ flex: '0 0 140px' }}
                      value={link.label}
                      onChange={e => updateLink(i, 'label', e.target.value)}
                      placeholder="Label"
                    />
                    <input
                      className="form-input"
                      style={{ flex: 1 }}
                      value={link.url}
                      onChange={e => updateLink(i, 'url', e.target.value)}
                      placeholder="https://..."
                    />
                    <button
                      className="btn danger"
                      style={{ fontSize: 12, padding: '4px 10px', whiteSpace: 'nowrap' }}
                      onClick={() => removeLink(i)}
                      type="button"
                    >
                      Remove
                    </button>
                  </div>
                ))}
              </div>
            )}
            <button className="btn" onClick={addLink} type="button" style={{ fontSize: 13 }}>
              + Add link
            </button>
          </div>
        </div>

        {error && <div className="error-msg">{error}</div>}
        {saved && <div style={{ color: 'var(--up)', fontSize: 12, marginBottom: 8 }}>Saved!</div>}
        <button className="btn primary" onClick={save}>Save Theme</button>
      </div>
    </>
  )
}
