import type { ReactNode } from 'react'
import { ErrorIcon, InfoIcon, WarnIcon } from './icons'
import './ui.css'

export type NoticeTone = 'info' | 'warn' | 'error'

const icons = { info: InfoIcon, warn: WarnIcon, error: ErrorIcon }

/** An inline message: what happened and what to do. Errors are announced as alerts. */
export function Notice({ tone, children }: { tone: NoticeTone; children: ReactNode }) {
  const Icon = icons[tone]
  return (
    <div className={`notice notice-${tone}`} role={tone === 'error' ? 'alert' : 'status'}>
      <Icon className="notice-icon" />
      <div className="notice-body">{children}</div>
    </div>
  )
}
