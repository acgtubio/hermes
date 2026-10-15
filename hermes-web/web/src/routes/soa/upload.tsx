import { createFileRoute } from '@tanstack/react-router'
import { UploadPage } from '../../features/soa/UploadPage'

export const Route = createFileRoute('/soa/upload')({
  staticData: { crumb: 'Upload' },
  component: UploadPage,
})
