import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = {
  title: 'Status',
  description: 'Service status page',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" data-theme="default-light">
      <body>{children}</body>
    </html>
  )
}
