import { ApiError } from './api'

/** Keep in step with the API's HERMES_MAX_UPLOAD_MB (default 100). */
export const MAX_UPLOAD_MB = 100

/** What went wrong and what to do, in the interface's words rather than the server's. */
export interface UploadProblem {
  title: string
  detail: string
  traceId?: string | null
}

export const notPdf: UploadProblem = {
  title: "This file isn't a PDF.",
  detail: "Choose the SOA's PDF file and try again.",
}

export const tooLarge: UploadProblem = {
  title: `This file is larger than ${MAX_UPLOAD_MB} MB.`,
  detail: 'Scan the statement at a lower resolution, or split it, and upload again.',
}

/** Checks the browser can make before uploading; the server checks again. */
export function checkFile(file: File): UploadProblem | null {
  const pdf = file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')
  if (!pdf) return notPdf
  if (file.size > MAX_UPLOAD_MB * 1024 * 1024) return tooLarge
  return null
}

/** Turns a failed upload into a problem to show. A 409 (already uploaded) is handled separately. */
export function uploadProblem(err: Error): UploadProblem {
  if (!(err instanceof ApiError)) {
    return { title: "Couldn't reach Hermes.", detail: 'Check your connection and try again.' }
  }
  switch (err.status) {
    case 413:
      return tooLarge
    case 415:
      return notPdf
    default:
      return { title: 'The upload failed.', detail: err.message, traceId: err.traceId }
  }
}
