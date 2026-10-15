package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
)

var pdfMagic = []byte("%PDF-")

// CreateDocument streams the uploaded PDF straight to object storage, hashing it on the way, then
// queues it. The body isn't buffered: the request validator skips this route (see NewRouter).
func (s *Server) CreateDocument(ctx context.Context, req api.CreateDocumentRequestObject) (api.CreateDocumentResponseObject, error) {
	part, err := filePart(req.Body)
	if err != nil {
		return api.CreateDocument400JSONResponse{ErrorJSONResponse: api.ErrorJSONResponse{Message: err.Error()}}, nil
	}
	defer part.Close()

	body := &limitedReader{r: part, n: s.MaxUpload}
	br := bufio.NewReader(body)
	magic, err := br.Peek(len(pdfMagic))
	if body.exceeded {
		return s.tooLarge(), nil
	}
	if err != nil || !bytes.Equal(magic, pdfMagic) {
		return api.CreateDocument415JSONResponse{Message: "file is not a PDF"}, nil
	}

	id := uuid.New()
	key := fmt.Sprintf("documents/%s.pdf", id)
	hash := sha256.New()
	var size countWriter
	if err := s.Objects.Put(ctx, key, "application/pdf", io.TeeReader(br, io.MultiWriter(hash, &size))); err != nil {
		s.deleteObject(ctx, key) // a failed multipart upload is aborted, but don't rely on it
		if body.exceeded {
			return s.tooLarge(), nil
		}
		return nil, err
	}

	doc, err := s.Docs.Create(ctx, database.NewDocument{
		ID: id, Filename: part.FileName(), ObjectKey: key, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: int64(size),
	})
	if err != nil {
		s.deleteObject(ctx, key)
		var dup *database.DuplicateError
		if errors.As(err, &dup) {
			return api.CreateDocument409JSONResponse{Message: dup.Error()}, nil
		}
		return nil, err
	}
	s.Log.InfoContext(ctx, "document queued", "document_id", id, "filename", doc.Filename, "bytes", int64(size))
	s.Wake()
	return api.CreateDocument202JSONResponse(toAPI(doc)), nil
}

func (s *Server) tooLarge() api.CreateDocument413JSONResponse {
	return api.CreateDocument413JSONResponse{Message: fmt.Sprintf("file is larger than %d bytes", s.MaxUpload)}
}

func (s *Server) deleteObject(ctx context.Context, key string) {
	// The request context may be what failed; cleanup shouldn't depend on it.
	if err := s.Objects.Delete(context.WithoutCancel(ctx), key); err != nil {
		s.Log.WarnContext(ctx, "delete orphaned upload", "key", key, "err", err)
	}
}

// filePart returns the multipart part named "file".
func filePart(r *multipart.Reader) (*multipart.Part, error) {
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errors.New(`missing "file" field`)
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart body: %w", err)
		}
		if p.FormName() == "file" {
			return p, nil
		}
		p.Close()
	}
}

var errTooLarge = errors.New("upload too large")

// limitedReader fails once more than n bytes are read and remembers that it did, since the
// error may come back wrapped (or flattened) by the object store client.
type limitedReader struct {
	r        io.Reader
	n        int64
	exceeded bool
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n < 0 {
		l.exceeded = true
		return 0, errTooLarge
	}
	if int64(len(p)) > l.n+1 {
		p = p[:l.n+1]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	if l.n < 0 {
		l.exceeded = true
		return 0, errTooLarge
	}
	return n, err
}

type countWriter int64

func (c *countWriter) Write(p []byte) (int, error) { *c += countWriter(len(p)); return len(p), nil }

// uploadRoute is the one operation whose body the request validator must not read: it would
// buffer the whole (up to MaxUpload) file in memory.
func uploadRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == apiPrefix+"/documents"
}
