import { LinkList, LinkListRow, LinkListSoon } from '../../shared/ui/LinkList'

export function SoaHomePage() {
  return (
    <>
      <div className="page-head">
        <h1>Statement of Account</h1>
        <p className="muted">Work with vendors' statements of account.</p>
      </div>
      <LinkList>
        <LinkListRow
          to="/soa/upload"
          title="Upload a statement"
          description="Upload a scanned SOA PDF and extract its rows and totals."
        />
        <LinkListSoon title="Coming soon" />
        <LinkListSoon title="Coming soon" />
      </LinkList>
    </>
  )
}
