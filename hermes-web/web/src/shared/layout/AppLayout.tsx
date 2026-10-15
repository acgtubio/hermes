import { Link, useMatches } from '@tanstack/react-router'
import { Fragment } from 'react'
import type { ReactNode } from 'react'
import { LogoMark } from '../ui/icons'
import './AppLayout.css'

declare module '@tanstack/react-router' {
  interface StaticDataRouteOption {
    /** This route's step in the header path, e.g. "Statement of Account". */
    crumb?: string
  }
}

/** Header crumbs from the matched routes: a loader's { crumb } wins over staticData.crumb. */
function useCrumbs() {
  return useMatches()
    .map((m) => ({
      path: m.pathname,
      label: (m.loaderData as { crumb?: string } | undefined)?.crumb ?? m.staticData?.crumb,
    }))
    .filter((c): c is { path: string; label: string } => !!c.label)
}

function Separator() {
  return (
    <svg className="app-sep" viewBox="0 0 16 16" width="16" height="16" fill="none" aria-hidden="true">
      <path d="M10.5 3L5.5 13" stroke="currentColor" strokeWidth="1.25" strokeLinecap="round" />
    </svg>
  )
}

export function AppLayout({ children }: { children: ReactNode }) {
  const crumbs = useCrumbs()
  return (
    <div className="app">
      <header className="app-header">
        <nav className="app-trail" aria-label="Breadcrumb">
          <Link to="/" className="app-brand" aria-label="Hermes" aria-current={crumbs.length === 0 ? 'page' : undefined}>
            <span className="app-logo">
              <LogoMark />
            </span>
            <span className="app-brand-name">Hermes</span>
          </Link>
          {crumbs.map((c, i) => (
            <Fragment key={c.path}>
              <Separator />
              {i === crumbs.length - 1 ? (
                <span className="app-crumb" aria-current="page">
                  {c.label}
                </span>
              ) : (
                <Link to={c.path} className="app-crumb">
                  {c.label}
                </Link>
              )}
            </Fragment>
          ))}
        </nav>
      </header>
      <main className="app-main">{children}</main>
    </div>
  )
}
