// Package pipeline calls the Python OCR sidecar (hermes-cv/server.py) and converts its output to
// soa types.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"time"

	"github.com/acgtubio/hermes/internal/soa"
	"github.com/acgtubio/hermes/internal/trace"
)

// ErrRetryable marks failures that say nothing about the document (the sidecar is busy or not
// running), so the document should be requeued rather than failed.
var ErrRetryable = errors.New("pipeline unavailable")

type Client struct {
	url  string
	http *http.Client
}

// New returns a client for the sidecar at baseURL. timeout bounds a whole document, which takes
// a minute or two on the sample SOAs.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{url: baseURL, http: &http.Client{Timeout: timeout}}
}

// Progress reports that PagesDone of PageCount pages have been processed.
type Progress struct {
	PagesDone int `json:"pages_done"`
	PageCount int `json:"page_count"`
}

// message is one line of the sidecar's newline-delimited JSON response (see hermes-cv/server.py).
type message struct {
	Type     string    `json:"type"` // progress, result or error
	Document *document `json:"document"`
	Error    string    `json:"error"`
	Progress
}

// Process has the sidecar fetch the PDF from pdfURL and run the pipeline on it, calling onProgress
// (if non-nil) as pages finish. The context's trace ID, if any, is forwarded in a traceparent header.
func (c *Client) Process(ctx context.Context, pdfURL string, onProgress func(Progress)) (soa.Document, error) {
	body, _ := json.Marshal(map[string]string{"url": pdfURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/process", bytes.NewReader(body))
	if err != nil {
		return soa.Document{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if id := trace.FromContext(ctx); id != "" {
		req.Header.Set(trace.Header, trace.Traceparent(id)) // the sidecar prefixes its log lines with it
	}
	resp, err := c.http.Do(req)
	if errors.Is(err, syscall.ECONNREFUSED) {
		return soa.Document{}, fmt.Errorf("%w: %v", ErrRetryable, err)
	}
	if err != nil {
		return soa.Document{}, fmt.Errorf("call pipeline: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return readStream(resp.Body, onProgress)
	case http.StatusServiceUnavailable:
		return soa.Document{}, fmt.Errorf("%w: sidecar busy", ErrRetryable)
	default:
		var e struct{ Error string }
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			e.Error = string(b)
		}
		return soa.Document{}, fmt.Errorf("pipeline: %s (HTTP %d)", e.Error, resp.StatusCode)
	}
}

// readStream reads progress messages until the result or an error. The result line is the whole
// document (megabytes), so it's decoded as a stream rather than scanned as a line.
func readStream(r io.Reader, onProgress func(Progress)) (soa.Document, error) {
	dec := json.NewDecoder(r)
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF // the sidecar always ends with a result or an error
			}
			return soa.Document{}, fmt.Errorf("read pipeline output: %w", err)
		}
		switch m.Type {
		case "progress":
			if onProgress != nil {
				onProgress(m.Progress)
			}
		case "result":
			if m.Document == nil {
				return soa.Document{}, errors.New("pipeline result without a document")
			}
			return m.Document.convert()
		case "error":
			return soa.Document{}, fmt.Errorf("pipeline: %s", m.Error)
		default:
			return soa.Document{}, fmt.Errorf("unknown pipeline message type %q", m.Type)
		}
	}
}
