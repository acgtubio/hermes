package server

import (
	"context"
	"errors"

	"github.com/acgtubio/hermes/internal/api"
	"github.com/acgtubio/hermes/internal/database"
)

func (s *Server) GetDocument(ctx context.Context, req api.GetDocumentRequestObject) (api.GetDocumentResponseObject, error) {
	doc, err := s.Docs.Get(ctx, req.Id)
	if errors.Is(err, database.ErrNotFound) {
		return api.GetDocument404JSONResponse{ErrorJSONResponse: api.ErrorJSONResponse{Message: "document not found"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetDocument200JSONResponse(toAPI(doc)), nil
}
