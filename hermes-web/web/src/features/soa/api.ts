// The SOA feature's API client. It's deliberately local to this feature: move pieces to shared/
// only once a second feature needs them (see CLAUDE.md).
//
// The types mirror api/openapi.yaml (Document, Error). Keep them in step by hand until type
// generation is set up.

export type DocumentStatus = 'queued' | 'processing' | 'ready' | 'failed'

export interface SoaDocument {
  id: string
  filename: string
  status: DocumentStatus
  error?: string
  page_count?: number
  pages_done?: number
  pages_total?: number
  row_count?: number
  needs_review_count?: number
  created_at: string
  processed_at?: string
  /** The upload's trace ID, shared by the processing job's logs. */
  trace_id?: string
}

export class ApiError extends Error {
  readonly status: number
  readonly traceId: string | null

  constructor(status: number, message: string, traceId: string | null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.traceId = traceId
  }

  /** For a 409 on upload: the ID of the document the same file was already uploaded as. */
  get duplicateOf(): string | undefined {
    if (this.status !== 409) return undefined
    return /document ([0-9a-f-]{36})/.exec(this.message)?.[1]
  }
}

export const isTerminal = (s: DocumentStatus | undefined) => s === 'ready' || s === 'failed'

export const documentKeys = {
  all: ['soa', 'documents'] as const,
  detail: (id: string) => [...documentKeys.all, id] as const,
}

export const eventsUrl = (id: string) => `/api/documents/${encodeURIComponent(id)}/events`

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, init)
  if (!res.ok) {
    // Errors are {message} JSON, except a few the generated wrapper writes as text/plain.
    const text = await res.text()
    let message = text.trim() || res.statusText
    try {
      message = (JSON.parse(text) as { message?: string }).message ?? message
    } catch {
      // not JSON; keep the text
    }
    throw new ApiError(res.status, message, res.headers.get('X-Trace-Id'))
  }
  return (await res.json()) as T
}

export function uploadDocument(file: File): Promise<SoaDocument> {
  const body = new FormData()
  body.append('file', file)
  return request('/documents', { method: 'POST', body })
}

export function getDocument(id: string): Promise<SoaDocument> {
  return request(`/documents/${encodeURIComponent(id)}`)
}
