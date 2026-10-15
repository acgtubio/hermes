import type { QueryClient } from '@tanstack/react-query'
import { Outlet, createRootRouteWithContext } from '@tanstack/react-router'
import { AppLayout } from '../shared/layout/AppLayout'

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: () => (
    <AppLayout>
      <Outlet />
    </AppLayout>
  ),
  notFoundComponent: () => (
    <>
      <h1>Page not found</h1>
      <p className="muted">There's nothing at this address.</p>
    </>
  ),
})
