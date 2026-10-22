'use client'
import { useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'

export function AutoRefresh({ intervalMs = 30000 }: { intervalMs?: number }) {
  const router = useRouter()
  const ref = useRef(router)

  // Keep the ref current in an effect — assigning during render breaks
  // concurrent rendering (react-hooks/refs).
  useEffect(() => {
    ref.current = router
  }, [router])

  useEffect(() => {
    const id = setInterval(() => ref.current.refresh(), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs]) // stable — only re-runs if intervalMs changes

  return null
}
