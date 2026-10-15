import type { ReactNode } from 'react'
import './ui.css'

/** Status tones. Always shown with text, never colour alone. */
export type BadgeTone = 'neutral' | 'progress' | 'ok' | 'review' | 'failed'

export function Badge({ tone, children }: { tone: BadgeTone; children: ReactNode }) {
  return (
    <span className={`badge badge-${tone}`}>
      <span className="badge-dot" aria-hidden="true" />
      {children}
    </span>
  )
}
