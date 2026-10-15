import { createFileRoute } from '@tanstack/react-router'
import { documentKeys, getDocument } from '../../../features/soa/api'
import { DocumentError } from '../../../features/soa/DocumentError'
import { DocumentPage } from '../../../features/soa/DocumentPage'

export const Route = createFileRoute('/soa/documents/$documentId')({
  // Load into the query cache the page reads (useDocument), and name the header crumb after the file.
  loader: async ({ context, params }) => {
    const doc = await context.queryClient.ensureQueryData({
      queryKey: documentKeys.detail(params.documentId),
      queryFn: () => getDocument(params.documentId),
    })
    return { crumb: doc.filename }
  },
  component: Document,
  errorComponent: DocumentError,
})

function Document() {
  const { documentId } = Route.useParams()
  // key: a fresh page (and stream) per document, not the previous one's state
  return <DocumentPage key={documentId} id={documentId} />
}
