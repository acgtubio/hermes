// Package soa holds the parsed contents of a Statement of Account, as stored in Postgres.
package soa

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// Document is everything the pipeline extracted from one PDF.
type Document struct {
	PageCount int
	Rows      []Row
	Totals    []Total
}

// Amounts are the five money columns. A column is NULL (Valid false) when its cell didn't parse.
type Amounts struct {
	Sales, VAT, TotalSalesWithVAT, WithholdingTax, NetAmount pgtype.Numeric
}

// Row is one table row. Typed fields are NULL when the cell failed its check; the raw OCR read,
// confidence, bbox and status of every cell are always kept in Cells.
type Row struct {
	Page, RowIndex    int
	DocDate           pgtype.Date
	PONumber          pgtype.Text
	ServiceEntrySheet pgtype.Text
	ReferenceNo       pgtype.Text
	DocHeaderText     pgtype.Text
	Description       pgtype.Text
	Amounts
	Cells       json.RawMessage
	NeedsReview bool
}

const (
	TotalSubtotal = "subtotal"
	TotalFooter   = "footer"
)

// Total is a subtotal line or a page's NET AMOUNT footer (a running total over the document).
type Total struct {
	Page     int
	Kind     string
	AfterRow pgtype.Int4 // subtotals only
	Amounts
	Cells       json.RawMessage
	NeedsReview bool
}
