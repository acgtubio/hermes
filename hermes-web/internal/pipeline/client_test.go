package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// testdata/document.json is real pipeline output, trimmed: Sep 1-15 2026 page 4 rows 8-9 (row 9's
// withholding inferred from "44 97"), its subtotal and footer, and Jun 16-30 page 17 row 5, where
// sales and total both misread so every amount failed.
func sidecar(t *testing.T, code int, body []byte) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ URL string }
		if r.URL.Path != "/process" || json.NewDecoder(r.Body).Decode(&req) != nil || req.URL != "http://objects/doc.pdf" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.WriteHeader(code)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, time.Minute)
}

func numeric(t *testing.T, n pgtype.Numeric) string {
	t.Helper()
	if !n.Valid {
		return "NULL"
	}
	v, err := n.Value()
	if err != nil {
		t.Fatal(err)
	}
	return v.(string)
}

func TestProcess(t *testing.T) {
	fixture, err := os.ReadFile("testdata/document.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := sidecar(t, http.StatusOK, fixture).Process(context.Background(), "http://objects/doc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if doc.PageCount != 28 || len(doc.Rows) != 3 || len(doc.Totals) != 2 {
		t.Fatalf("page_count %d, %d rows, %d totals; want 28, 3, 2", doc.PageCount, len(doc.Rows), len(doc.Totals))
	}

	ok := doc.Rows[0] // page 4 row 8, every check passed
	if ok.NeedsReview || numeric(t, ok.Sales) != "2448.70" || ok.PONumber.String != "4602107138" ||
		ok.DocDate.Time.Format("2006-01-02") != "2026-09-12" {
		t.Errorf("clean row = page %d row %d sales %s po %v date %v review %v",
			ok.Page, ok.RowIndex, numeric(t, ok.Sales), ok.PONumber, ok.DocDate, ok.NeedsReview)
	}

	inferred := doc.Rows[1] // page 4 row 9
	if !inferred.NeedsReview || numeric(t, inferred.WithholdingTax) != "44.97" {
		t.Errorf("inferred row: withholding %s, needs_review %v; want 44.97, true",
			numeric(t, inferred.WithholdingTax), inferred.NeedsReview)
	}
	var cells map[string]map[string]any
	if err := json.Unmarshal(inferred.Cells, &cells); err != nil {
		t.Fatal(err)
	}
	if c := cells["withholding_tax"]; c["first_read"] != "44 97" || c["status"] != "inferred" || c["conf"] == nil {
		t.Errorf("raw cell not kept: %v", c)
	}

	failed := doc.Rows[2] // Jun page 17 row 5
	if !failed.NeedsReview || failed.Sales.Valid || failed.NetAmount.Valid {
		t.Errorf("failed row: sales %s net %s needs_review %v; want NULL amounts and review",
			numeric(t, failed.Sales), numeric(t, failed.NetAmount), failed.NeedsReview)
	}
	if failed.PONumber.String != "4601996716" {
		t.Errorf("failed row lost its passing ID fields: po %v", failed.PONumber)
	}

	sub, footer := doc.Totals[0], doc.Totals[1]
	if sub.Kind != "subtotal" || sub.AfterRow.Int32 != 11 || footer.Kind != "footer" || footer.AfterRow.Valid {
		t.Errorf("totals = %s after %v, %s after %v", sub.Kind, sub.AfterRow, footer.Kind, footer.AfterRow)
	}
	if numeric(t, footer.Sales) != "98379.54" || footer.NeedsReview {
		t.Errorf("footer sales %s review %v, want 98379.54 false", numeric(t, footer.Sales), footer.NeedsReview)
	}
}

func TestProcessErrors(t *testing.T) {
	_, err := sidecar(t, http.StatusServiceUnavailable, []byte(`{"error":"busy"}`)).Process(context.Background(), "http://objects/doc.pdf")
	if !errors.Is(err, ErrRetryable) {
		t.Errorf("503: err = %v, want ErrRetryable", err)
	}
	_, err = sidecar(t, http.StatusUnprocessableEntity, []byte(`{"error":"FileDataError: broken pdf"}`)).Process(context.Background(), "http://objects/doc.pdf")
	if err == nil || errors.Is(err, ErrRetryable) || err.Error() != "pipeline: FileDataError: broken pdf (HTTP 422)" {
		t.Errorf("422: err = %v", err)
	}
	_, err = New("http://127.0.0.1:1", time.Second).Process(context.Background(), "http://objects/doc.pdf")
	if !errors.Is(err, ErrRetryable) {
		t.Errorf("sidecar down: err = %v, want ErrRetryable", err)
	}
}
