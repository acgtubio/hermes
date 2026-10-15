import { createFileRoute } from '@tanstack/react-router'
import { LinkList, LinkListRow } from '../shared/ui/LinkList'

// The landing page: one row per tool. Each tool lives under its own route and features/<name>/,
// and its code is only downloaded when the user opens it.
export const Route = createFileRoute('/')({
  component: Landing,
})

function Landing() {
  return (
    <>
      <div className="page-head">
        <h1>Tools</h1>
        <p className="muted">Choose what you're working on.</p>
      </div>
      <LinkList>
        <LinkListRow
          to="/soa"
          title="Statement of Account"
          description="Read vendors' scanned statements and check their rows and totals."
        />
      </LinkList>
    </>
  )
}
