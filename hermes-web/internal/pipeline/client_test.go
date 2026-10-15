package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/acgtubio/hermes/internal/trace"
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
		if id, _ := trace.FromTraceparent(r.Header.Get(trace.Header)); id != testTraceID {
			t.Errorf("traceparent %q doesn't carry trace %s", r.Header.Get(trace.Header), testTraceID)
		}
		w.WriteHeader(code)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, time.Minute)
}

const testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

// ctx carries the trace ID the sidecar stub expects in the traceparent header.
var ctx = trace.WithID(context.Background(), testTraceID)

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

// stream builds a sidecar response: one NDJSON line per message.
func stream(t *testing.T, msgs ...any) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, m := range msgs {
		line, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(append(line, '\n'))
	}
	return b.Bytes()
}

func progress(done, total int) map[string]any {
	return map[string]any{"type": "progress", "pages_done": done, "page_count": total}
}

func TestProcess(t *testing.T) {
	fixture, err := os.ReadFile("testdata/document.json")
	if err != nil {
		t.Fatal(err)
	}
	body := stream(t, progress(0, 28), progress(1, 28), progress(2, 28),
		map[string]any{"type": "result", "document": json.RawMessage(fixture)})
	var got []Progress
	doc, err := sidecar(t, http.StatusOK, body).Process(ctx, "http://objects/doc.pdf", func(p Progress) { got = append(got, p) })
	if err != nil {
		t.Fatal(err)
	}
	if want := []Progress{{0, 28}, {1, 28}, {2, 28}}; !reflect.DeepEqual(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
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
	// Mid-stream failures, after the 200: an error message, and a stream cut off before its result.
	_, err := sidecar(t, http.StatusOK, stream(t, progress(0, 3), map[string]any{"type": "error", "error": "KeyError: 'w'"})).
		Process(ctx, "http://objects/doc.pdf", nil)
	if err == nil || err.Error() != "pipeline: KeyError: 'w'" {
		t.Errorf("error message: err = %v", err)
	}
	_, err = sidecar(t, http.StatusOK, stream(t, progress(0, 3), progress(1, 3))).Process(ctx, "http://objects/doc.pdf", nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") || errors.Is(err, ErrRetryable) {
		t.Errorf("truncated stream: err = %v, want unexpected EOF", err)
	}

	_, err = sidecar(t, http.StatusServiceUnavailable, []byte(`{"error":"busy"}`)).Process(ctx, "http://objects/doc.pdf", nil)
	if !errors.Is(err, ErrRetryable) {
		t.Errorf("503: err = %v, want ErrRetryable", err)
	}
	_, err = sidecar(t, http.StatusUnprocessableEntity, []byte(`{"error":"FileDataError: broken pdf"}`)).Process(ctx, "http://objects/doc.pdf", nil)
	if err == nil || errors.Is(err, ErrRetryable) || err.Error() != "pipeline: FileDataError: broken pdf (HTTP 422)" {
		t.Errorf("422: err = %v", err)
	}
	_, err = New("http://127.0.0.1:1", time.Second).Process(ctx, "http://objects/doc.pdf", nil)
	if !errors.Is(err, ErrRetryable) {
		t.Errorf("sidecar down: err = %v, want ErrRetryable", err)
	}
}
