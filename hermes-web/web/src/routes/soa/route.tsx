import { Outlet, createFileRoute } from '@tanstack/react-router'

// The Statement of Account section: every /soa page sits under this crumb.
export const Route = createFileRoute('/soa')({
  staticData: { crumb: 'Statement of Account' },
  component: Outlet,
})
