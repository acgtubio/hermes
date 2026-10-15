import type { ReactNode } from 'react'
import { FileIcon } from './icons'
import './ui.css'

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

/** A chosen file: name, size and an optional action (e.g. Remove). */
export function FileRow({ name, size, action }: { name: string; size: number; action?: ReactNode }) {
  return (
    <div className="file-row">
      <FileIcon size={18} className="file-row-icon" />
      <div className="file-row-text">
        <span className="file-row-name">{name}</span>
        <span className="muted small">{formatBytes(size)}</span>
      </div>
      {action}
    </div>
  )
}
