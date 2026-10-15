import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { documentKeys, eventsUrl, getDocument, isTerminal, uploadDocument } from './api'
import type { SoaDocument } from './api'

/**
 * The document, kept live while it's queued or processing: the events stream writes each
 * `document` event into the query cache. The stream closes on `end`, and isn't opened at all
 * for a document that's already ready or failed.
 */
export function useDocument(id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: documentKeys.detail(id), queryFn: () => getDocument(id) })
  const done = isTerminal(query.data?.status) || query.isError

  useEffect(() => {
    if (done) return
    const events = new EventSource(eventsUrl(id))
    events.addEventListener('document', (e) => {
      queryClient.setQueryData<SoaDocument>(documentKeys.detail(id), JSON.parse(e.data))
    })
    // Without this, EventSource would reconnect after the server closes the stream.
    events.addEventListener('end', () => events.close())
    return () => events.close()
  }, [id, done, queryClient])

  return query
}

export function useUploadDocument() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: uploadDocument,
    onSuccess: (doc) => queryClient.setQueryData(documentKeys.detail(doc.id), doc),
  })
}
