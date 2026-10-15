import type { ErrorComponentProps } from '@tanstack/react-router'
import { ButtonLink } from '../../shared/ui/Button'
import { Notice } from '../../shared/ui/Notice'
import { ApiError } from './api'
import './soa.css'

/** Shown when the document route's loader fails: an unknown ID, or the API is unreachable. */
export function DocumentError({ error }: ErrorComponentProps) {
  const missing = error instanceof ApiError && (error.status === 404 || error.status === 400)
  return (
    <>
      <div className="page-head">
        <h1>{missing ? 'Document not found' : "Couldn't load the document"}</h1>
      </div>
      <Notice tone="error">
        <p>
          {missing ? (
            <>There's no document at this address. The link may be wrong, or the document was deleted.</>
          ) : (
            <>
              <strong>Hermes didn't answer.</strong> Check your connection and reload the page.
            </>
          )}
        </p>
        {error instanceof ApiError && error.traceId && (
          <p className="muted small">
            Trace ID <span className="mono">{error.traceId}</span>
          </p>
        )}
      </Notice>
      <div className="soa-actions">
        <ButtonLink to="/soa/upload">Upload a statement</ButtonLink>
      </div>
    </>
  )
}
