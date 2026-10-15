import { useState } from 'react'
import { Badge } from '../../shared/ui/Badge'
import type { BadgeTone } from '../../shared/ui/Badge'
import { Button, ButtonLink } from '../../shared/ui/Button'
import { Facts } from '../../shared/ui/Facts'
import { CopyIcon } from '../../shared/ui/icons'
import { Notice } from '../../shared/ui/Notice'
import { Panel } from '../../shared/ui/Panel'
import { ProgressBar } from '../../shared/ui/ProgressBar'
import { Steps } from '../../shared/ui/Steps'
import type { StepState } from '../../shared/ui/Steps'
import type { DocumentStatus, SoaDocument } from './api'
import { useDocument } from './hooks'
import './soa.css'

const stepLabels = ['Uploaded', 'Queued', 'Reading pages', 'Ready']

const stepStates: Record<DocumentStatus, StepState[]> = {
  queued: ['done', 'current', 'pending', 'pending'],
  processing: ['done', 'done', 'current', 'pending'],
  ready: ['done', 'done', 'done', 'done'],
  failed: ['done', 'done', 'failed', 'pending'],
}

/** Green only when there's nothing left to check: a ready document with rows to review is amber. */
function badgeFor(doc: SoaDocument): { tone: BadgeTone; label: string } {
  switch (doc.status) {
    case 'queued':
      return { tone: 'neutral', label: 'Queued' }
    case 'processing':
      return { tone: 'progress', label: 'Processing' }
    case 'ready':
      return doc.needs_review_count ? { tone: 'review', label: 'Needs review' } : { tone: 'ok', label: 'Ready' }
    case 'failed':
      return { tone: 'failed', label: 'Failed' }
  }
}

const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

function duration(ms: number) {
  const s = Math.max(0, Math.round(ms / 1000))
  return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${s % 60} s`
}

function meta(doc: SoaDocument) {
  const uploaded = `Uploaded ${dateTime.format(new Date(doc.created_at))}.`
  if (doc.status !== 'ready' || !doc.processed_at) return uploaded
  const took = new Date(doc.processed_at).getTime() - new Date(doc.created_at).getTime()
  return `${uploaded} Ready ${duration(took)} after upload.`
}

export function DocumentPage({ id }: { id: string }) {
  // The route's loader has already filled the cache, so data is here on first render.
  const { data: doc } = useDocument(id)
  if (!doc) return null
  const badge = badgeFor(doc)

  return (
    <>
      <div className="page-head">
        <div className="soa-title">
          <h1>{doc.filename}</h1>
          <Badge tone={badge.tone}>{badge.label}</Badge>
        </div>
        <p className="muted small">{meta(doc)}</p>
      </div>

      <Panel aria-label="Processing status">
        <Steps steps={stepLabels.map((label, i) => ({ label, state: stepStates[doc.status][i] }))} />
        <DocumentBody doc={doc} />
      </Panel>

      <div className="soa-actions">
        <ButtonLink to="/soa/upload">Upload another SOA</ButtonLink>
      </div>
    </>
  )
}

const leaveHint = 'You can leave this page. Processing continues, and the status here updates on its own.'

function DocumentBody({ doc }: { doc: SoaDocument }) {
  switch (doc.status) {
    case 'queued':
      return (
        <div className="soa-stack-tight">
          <p>
            <strong>Waiting to start.</strong> Statements are read one at a time, so this one starts when the current
            one finishes.
          </p>
          <p className="muted small">{leaveHint}</p>
        </div>
      )
    case 'processing': {
      const { pages_done: done = 0, pages_total: total } = doc
      return (
        <>
          {total === undefined ? (
            <p>Preparing the pages…</p>
          ) : (
            <div className="soa-stack-tight" aria-live="polite">
              <ProgressBar label="Pages read" value={done} total={total} />
              <p>
                <strong>
                  {done} of {total}
                </strong>{' '}
                pages read
              </p>
            </div>
          )}
          <p className="muted small">{leaveHint}</p>
        </>
      )
    }
    case 'ready': {
      const review = doc.needs_review_count ?? 0
      return (
        <>
          <Facts
            items={[
              { label: 'Pages', value: doc.page_count ?? '–' },
              { label: 'Rows', value: doc.row_count ?? '–' },
              { label: 'Need review', value: review, tone: review ? 'warn' : undefined },
            ]}
          />
          {review > 0 ? (
            <Notice tone="warn">
              <p>
                <strong>
                  {review} {review === 1 ? 'row needs' : 'rows need'} review.
                </strong>{' '}
                A cell in each couldn't be read, or was worked out from the row's other amounts. Check them against the
                PDF before you rely on the totals.
              </p>
            </Notice>
          ) : (
            <p className="muted">Every row passed its checks.</p>
          )}
        </>
      )
    }
    case 'failed':
      return (
        <>
          <Notice tone="error">
            <p>
              <strong>Hermes couldn't read this statement.</strong> Report it with the trace ID below, so the problem
              can be found in the logs.
            </p>
            {doc.error && <p className="mono soa-error-detail">{doc.error}</p>}
          </Notice>
          {doc.trace_id && <TraceId id={doc.trace_id} />}
        </>
      )
  }
}

function TraceId({ id }: { id: string }) {
  const [copied, setCopied] = useState<'idle' | 'done' | 'failed'>('idle')
  const copy = () =>
    navigator.clipboard.writeText(id).then(
      () => setCopied('done'),
      () => setCopied('failed'),
    )
  return (
    <div className="soa-trace">
      <span className="muted small">Trace ID</span>
      <span className="mono">{id}</span>
      <Button variant="ghost" onClick={copy}>
        <CopyIcon size={14} />
        <span aria-live="polite">{copied === 'done' ? 'Copied' : copied === 'failed' ? "Couldn't copy" : 'Copy'}</span>
      </Button>
    </div>
  )
}
