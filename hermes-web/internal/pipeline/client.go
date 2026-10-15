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

// Process has the sidecar fetch the PDF from pdfURL and run the pipeline on it.
func (c *Client) Process(ctx context.Context, pdfURL string) (soa.Document, error) {
	body, _ := json.Marshal(map[string]string{"url": pdfURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/process", bytes.NewReader(body))
	if err != nil {
		return soa.Document{}, err
	}
	req.Header.Set("Content-Type", "application/json")
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
		var d document
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			return soa.Document{}, fmt.Errorf("decode pipeline output: %w", err)
		}
		return d.convert()
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
