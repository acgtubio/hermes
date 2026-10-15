import type { HTMLAttributes } from 'react'
import './ui.css'

/** A bordered surface that groups one block of content. */
export function Panel({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <section className={className ? `panel ${className}` : 'panel'} {...props} />
}
