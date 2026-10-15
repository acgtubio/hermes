import { CheckIcon, CrossIcon } from './icons'
import './ui.css'

export type StepState = 'done' | 'current' | 'failed' | 'pending'

/** A short, fixed sequence of stages with the current one marked. */
export function Steps({ steps }: { steps: { label: string; state: StepState }[] }) {
  return (
    <ol className="steps">
      {steps.map((s, i) => (
        <li key={s.label} className={`step step-${s.state}`} aria-current={s.state === 'current' ? 'step' : undefined}>
          <span className="step-mark">
            {s.state === 'done' ? <CheckIcon size={12} /> : s.state === 'failed' ? <CrossIcon size={12} /> : i + 1}
          </span>
          {s.label}
          {s.state === 'failed' && <span className="visually-hidden"> (failed)</span>}
        </li>
      ))}
    </ol>
  )
}
