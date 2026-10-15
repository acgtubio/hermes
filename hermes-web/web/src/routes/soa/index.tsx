import { createFileRoute } from '@tanstack/react-router'
import { SoaHomePage } from '../../features/soa/SoaHomePage'

export const Route = createFileRoute('/soa/')({
  component: SoaHomePage,
})
