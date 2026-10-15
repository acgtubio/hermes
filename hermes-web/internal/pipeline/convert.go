package pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/acgtubio/hermes/internal/soa"
)

// The sidecar's JSON (hermes-cv/run.py process): table pages in rows.json's shapes.
type document struct {
	PageCount int    `json:"page_count"`
	Pages     []page `json:"pages"`
}

type page struct {
	Page int `json:"page"`
	Rows []struct {
		RowIndex int             `json:"row_index"`
		Cells    json.RawMessage `json:"cells"`
	} `json:"rows"`
	Subtotals []struct {
		AfterRow int             `json:"after_row"`
		Cells    json.RawMessage `json:"cells"`
	} `json:"subtotals"`
	Footer json.RawMessage `json:"footer"` // null when the page has none
}

// cell is the part of a pipeline cell the typed columns need; Cells keeps all of it.
type cell struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

// Cell statuses set by the pipeline (see hermes-cv validate.cell_status).
const (
	statusFailed   = "failed"
	statusInferred = "inferred"
)

var amountFields = []string{"sales", "vat", "total_sales_w_vat", "withholding_tax", "net_amount"}

// amountRe is how the report prints amounts: only the last thousands grouped, e.g. 1001,579.43.
// Same rule as hermes-cv validate.num.
var amountRe = regexp.MustCompile(`^\d+(,\d{3})?\.\d{2}$`)

func (d document) convert() (soa.Document, error) {
	out := soa.Document{PageCount: d.PageCount}
	for _, p := range d.Pages {
		for _, r := range p.Rows {
			row, err := convertRow(p.Page, r.RowIndex, r.Cells)
			if err != nil {
				return soa.Document{}, fmt.Errorf("page %d row %d: %w", p.Page, r.RowIndex, err)
			}
			out.Rows = append(out.Rows, row)
		}
		for _, s := range p.Subtotals {
			t, err := convertTotal(p.Page, soa.TotalSubtotal, s.Cells)
			if err != nil {
				return soa.Document{}, fmt.Errorf("page %d subtotal: %w", p.Page, err)
			}
			t.AfterRow = pgtype.Int4{Int32: int32(s.AfterRow), Valid: true}
			out.Totals = append(out.Totals, t)
		}
		if len(p.Footer) > 0 && string(p.Footer) != "null" {
			t, err := convertTotal(p.Page, soa.TotalFooter, p.Footer)
			if err != nil {
				return soa.Document{}, fmt.Errorf("page %d footer: %w", p.Page, err)
			}
			out.Totals = append(out.Totals, t)
		}
	}
	return out, nil
}

func convertRow(pageNo, index int, raw json.RawMessage) (soa.Row, error) {
	var cells map[string]cell
	if err := json.Unmarshal(raw, &cells); err != nil {
		return soa.Row{}, err
	}
	r := soa.Row{
		Page: pageNo, RowIndex: index,
		DocDate:           date(cells["doc_date"]),
		PONumber:          checkedText(cells["po_number"]),
		ServiceEntrySheet: checkedText(cells["service_entry_sheet"]),
		ReferenceNo:       checkedText(cells["reference_no"]),
		DocHeaderText:     checkedText(cells["doc_header_text"]),
		Description:       checkedText(cells["text"]),
		Amounts:           amounts(cells),
		Cells:             raw,
		NeedsReview:       needsReview(cells),
	}
	return r, nil
}

func convertTotal(pageNo int, kind string, raw json.RawMessage) (soa.Total, error) {
	var cells map[string]cell
	if err := json.Unmarshal(raw, &cells); err != nil {
		return soa.Total{}, err
	}
	return soa.Total{Page: pageNo, Kind: kind, Amounts: amounts(cells), Cells: raw, NeedsReview: needsReview(cells)}, nil
}

// needsReview: a cell failed its check or was inferred rather than read, or an amount is missing.
func needsReview(cells map[string]cell) bool {
	for _, c := range cells {
		if c.Status == statusFailed || c.Status == statusInferred {
			return true
		}
	}
	for _, k := range amountFields {
		if _, ok := cells[k]; !ok {
			return true
		}
	}
	return false
}

func amounts(cells map[string]cell) soa.Amounts {
	return soa.Amounts{
		Sales: amount(cells["sales"]), VAT: amount(cells["vat"]), TotalSalesWithVAT: amount(cells["total_sales_w_vat"]),
		WithholdingTax: amount(cells["withholding_tax"]), NetAmount: amount(cells["net_amount"]),
	}
}

// amount is NULL unless the cell reads as a well-formed amount and didn't fail its check.
func amount(c cell) pgtype.Numeric {
	var n pgtype.Numeric
	if c.Status == statusFailed || !amountRe.MatchString(c.Text) {
		return n
	}
	if err := n.Scan(strings.ReplaceAll(c.Text, ",", "")); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func date(c cell) pgtype.Date {
	t, err := time.Parse("02.01.2006", c.Text)
	if c.Status == statusFailed || err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// checkedText is the cell's text unless it's missing or failed its check.
func checkedText(c cell) pgtype.Text {
	if c.Text == "" || c.Status == statusFailed {
		return pgtype.Text{}
	}
	return pgtype.Text{String: c.Text, Valid: true}
}
