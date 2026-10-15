import type { ReactNode } from 'react'
import './ui.css'

/** A row of labelled figures. tone 'warn' marks a figure that needs attention. */
export function Facts({ items }: { items: { label: string; value: ReactNode; tone?: 'warn' }[] }) {
  return (
    <dl className="facts">
      {items.map((f) => (
        <div key={f.label} className={f.tone ? `fact fact-${f.tone}` : 'fact'}>
          <dt>{f.label}</dt>
          <dd>{f.value}</dd>
        </div>
      ))}
    </dl>
  )
}
