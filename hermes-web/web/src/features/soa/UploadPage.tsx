import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { Button, ButtonLink } from '../../shared/ui/Button'
import { DropZone } from '../../shared/ui/DropZone'
import { FileRow } from '../../shared/ui/FileRow'
import { Notice } from '../../shared/ui/Notice'
import { ApiError } from './api'
import { useUploadDocument } from './hooks'
import { MAX_UPLOAD_MB, checkFile, uploadProblem } from './uploadErrors'
import type { UploadProblem } from './uploadErrors'
import './soa.css'

export function UploadPage() {
  const [file, setFile] = useState<File | null>(null)
  const [rejected, setRejected] = useState<UploadProblem | null>(null)
  const upload = useUploadDocument()
  const navigate = useNavigate()

  const choose = (f: File) => {
    upload.reset()
    const problem = checkFile(f)
    setRejected(problem)
    setFile(problem ? null : f)
  }

  const remove = () => {
    upload.reset()
    setFile(null)
  }

  const start = () => {
    if (!file) return
    upload.mutate(file, {
      onSuccess: (doc) => navigate({ to: '/soa/documents/$documentId', params: { documentId: doc.id } }),
    })
  }

  const err = upload.error
  const duplicateOf = err instanceof ApiError ? err.duplicateOf : undefined
  const problem = err && !duplicateOf ? uploadProblem(err) : rejected

  return (
    <>
      <div className="page-head">
        <h1>Upload a statement</h1>
        <p className="muted">
          A vendor's scanned SOA, as a PDF. Hermes reads every page and extracts the rows and totals, which takes a
          minute or two.
        </p>
      </div>

      <div className="soa-stack">
        {file ? (
          <FileRow
            name={file.name}
            size={file.size}
            action={
              <Button variant="ghost" onClick={remove} disabled={upload.isPending}>
                Remove
              </Button>
            }
          />
        ) : (
          <DropZone
            onFile={choose}
            accept="application/pdf,.pdf"
            prompt="Drag the SOA PDF here, or"
            button="Choose file"
            hint={`PDF, up to ${MAX_UPLOAD_MB} MB`}
          />
        )}

        {duplicateOf && (
          <Notice tone="info">
            <p>
              <strong>This file was already uploaded.</strong> Hermes keeps one copy of each statement, so it won't be
              read twice.
            </p>
            <div>
              <ButtonLink variant="primary" to="/soa/documents/$documentId" params={{ documentId: duplicateOf }}>
                Open the earlier upload
              </ButtonLink>
            </div>
          </Notice>
        )}

        {problem && (
          <Notice tone="error">
            <p>
              <strong>{problem.title}</strong> {problem.detail}
            </p>
            {problem.traceId && (
              <p className="muted small">
                Trace ID <span className="mono">{problem.traceId}</span>
              </p>
            )}
          </Notice>
        )}

        {file && !duplicateOf && (
          <div className="soa-actions" aria-live="polite">
            <Button variant="primary" onClick={start} disabled={upload.isPending}>
              {upload.isPending ? 'Uploading…' : 'Upload and read'}
            </Button>
            <span className="muted small">
              {upload.isPending
                ? 'Keep this page open until the upload finishes. Large scans take a few seconds.'
                : "You'll see each page's progress on the next screen."}
            </span>
          </div>
        )}
      </div>
    </>
  )
}
