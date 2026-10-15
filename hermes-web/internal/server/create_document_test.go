package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acgtubio/hermes/internal/api"
)

func upload(t *testing.T, h http.Handler, field, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/documents", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var samplePDF = []byte("%PDF-1.7\nnot really a pdf, but the magic is right\n%%EOF\n")

func TestCreateDocument(t *testing.T) {
	docs, objects := newFakeDocs(), newFakeObjects()
	woken := 0
	h := newRouterWith(t, Deps{Docs: docs, Objects: objects, Wake: func() { woken++ }})

	rec := upload(t, h, "file", "Sep 1-15, 2026 SOA.pdf", samplePDF)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want 202; body %s", rec.Code, rec.Body)
	}
	var got api.Document
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != api.DocumentStatusQueued || got.Filename != "Sep 1-15, 2026 SOA.pdf" {
		t.Errorf("document = %+v", got)
	}
	if woken != 1 {
		t.Errorf("wake called %d times, want 1", woken)
	}
	stored := objects.objs["documents/"+got.Id.String()+".pdf"]
	if !bytes.Equal(stored, samplePDF) {
		t.Errorf("stored object = %q, want the upload", stored)
	}
	sum := sha256.Sum256(samplePDF)
	if docs.bySum[hex.EncodeToString(sum[:])] != got.Id {
		t.Error("document not stored under the file's sha256")
	}
	if traceID := rec.Header().Get(TraceIDHeader); docs.created[0].TraceID != traceID || traceID == "" {
		t.Errorf("document stored with trace ID %q, want the request's %q", docs.created[0].TraceID, traceID)
	}

	t.Run("duplicate", func(t *testing.T) {
		rec := upload(t, h, "file", "again.pdf", samplePDF)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status code = %d, want 409", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), got.Id.String()) {
			t.Errorf("409 body %s doesn't name the existing document", rec.Body)
		}
		if len(objects.objs) != 1 {
			t.Errorf("%d objects stored, want the duplicate's deleted", len(objects.objs))
		}
	})
}

func TestCreateDocumentRejects(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		content  []byte
		wantCode int
	}{
		{"not a pdf", "file", []byte("hello, world"), http.StatusUnsupportedMediaType},
		{"empty", "file", nil, http.StatusUnsupportedMediaType},
		{"too large", "file", append(append([]byte{}, samplePDF...), make([]byte, 2<<10)...), http.StatusRequestEntityTooLarge},
		{"wrong field", "upload", samplePDF, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := newFakeObjects()
			h := newRouterWith(t, Deps{Objects: objects, MaxUpload: 1 << 10})
			rec := upload(t, h, tt.field, "x.pdf", tt.content)
			if rec.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
			}
			if len(objects.objs) != 0 {
				t.Errorf("%d objects left in storage", len(objects.objs))
			}
		})
	}
}
