import { createLink } from '@tanstack/react-router'
import type { AnchorHTMLAttributes, ReactNode, Ref } from 'react'
import { ChevronRightIcon } from './icons'
import './ui.css'

/** A list of destinations (tools, features), one row each, divided rather than boxed. */
export function LinkList({ children }: { children: ReactNode }) {
  return <ul className="link-list">{children}</ul>
}

type RowProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'title'> & {
  title: string
  description: string
  ref?: Ref<HTMLAnchorElement>
}

function RowAnchor({ title, description, ref, ...props }: RowProps) {
  return (
    <li>
      <a ref={ref} className="link-row" {...props}>
        <span className="link-row-text">
          <span className="link-row-title">{title}</span>
          <span className="muted">{description}</span>
        </span>
        <ChevronRightIcon className="link-row-chevron" />
      </a>
    </li>
  )
}

/** A row that links to a route: <LinkListRow to="/soa" title=… description=… />. */
export const LinkListRow = createLink(RowAnchor)

/** A row for something that isn't available yet: greyed and not a link. */
export function LinkListSoon({ title, description }: { title: string; description?: string }) {
  return (
    <li className="link-row link-row-soon">
      <span className="link-row-text">
        <span className="link-row-title">{title}</span>
        {description && <span className="small">{description}</span>}
      </span>
      <span className="small">Coming soon</span>
    </li>
  )
}
