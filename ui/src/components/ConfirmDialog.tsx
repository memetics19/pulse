'use client'
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'

type ConfirmOptions = {
  title: string
  message?: string
  confirmLabel?: string
  /** When set, the confirm button stays disabled until the user types this exact
   *  string — a stronger guard for irreversible actions (delete, revoke). */
  requireText?: string
}

type Resolver = (ok: boolean) => void

const ConfirmContext = createContext<(o: ConfirmOptions) => Promise<boolean>>(
  () => Promise.resolve(false),
)

/** useConfirm returns an async function that opens the shared confirm dialog and
 *  resolves true only when the user explicitly confirms. Every destructive
 *  action should await it before calling the API — never a native confirm(). */
export function useConfirm() {
  return useContext(ConfirmContext)
}

export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null)
  const [typed, setTyped] = useState('')
  const resolver = useRef<Resolver | null>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)

  const confirm = useCallback((o: ConfirmOptions) => {
    setTyped('')
    setOpts(o)
    return new Promise<boolean>(resolve => { resolver.current = resolve })
  }, [])

  const close = useCallback((ok: boolean) => {
    resolver.current?.(ok)
    resolver.current = null
    setOpts(null)
  }, [])

  // Focus Cancel by default (never the destructive button) and wire Escape.
  useEffect(() => {
    if (!opts) return
    cancelRef.current?.focus()
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [opts, close])

  const needsText = opts?.requireText
  const confirmDisabled = !!needsText && typed !== needsText

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {opts && (
        <div className="modal-overlay" role="dialog" aria-modal="true"
             onClick={e => { if (e.target === e.currentTarget) close(false) }}>
          <div className="modal">
            <h2 className="modal-title">{opts.title}</h2>
            {opts.message && <p style={{ marginBottom: 12 }}>{opts.message}</p>}
            {needsText && (
              <div className="form-group">
                <label className="form-label">
                  Type <code>{needsText}</code> to confirm
                </label>
                <input className="form-input" value={typed} autoFocus
                       onChange={e => setTyped(e.target.value)} />
              </div>
            )}
            <div className="modal-footer">
              <button ref={cancelRef} className="btn" onClick={() => close(false)}>Cancel</button>
              <button className="btn danger" disabled={confirmDisabled} onClick={() => close(true)}>
                {opts.confirmLabel ?? 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  )
}
