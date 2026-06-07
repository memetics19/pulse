'use client'
import { useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'

export function AutoRefresh({ intervalMs = 30000 }: { intervalMs?: number }) {
  const router = useRouter()
  const ref = useRef(router)
  ref.current = router

  useEffect(() => {
    const id = setInterval(() => ref.current.refresh(), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs]) // stable — only re-runs if intervalMs changes

  return null
}
