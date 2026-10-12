package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bakery-pos/backend/internal/catalog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.catalog.ListCategories(r.Context())
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": categories})
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	category, err := s.catalog.CreateCategory(r.Context(), input.Name, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, category)
}

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	filter := catalog.ProductFilter{Query: r.URL.Query().Get("q")}
	if value := strings.TrimSpace(r.URL.Query().Get("active")); value != "" {
		active, err := strconv.ParseBool(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El filtro active no es válido.", nil)
			return
		}
		filter.Active = &active
	}
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El límite no es válido.", nil)
			return
		}
		filter.Limit = limit
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		offset, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El desplazamiento no es válido.", nil)
			return
		}
		filter.Offset = offset
	}
	products, err := s.catalog.ListProducts(r.Context(), filter)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r)
	if !ok {
		return
	}
	product, err := s.catalog.GetProduct(r.Context(), id)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var input catalog.CreateProductInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	product, err := s.catalog.CreateProduct(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r)
	if !ok {
		return
	}
	var input catalog.UpdateProductInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	if emptyProductUpdate(input) {
		writeError(w, r, http.StatusBadRequest, "validation_error", "Indicá al menos un campo para modificar.", nil)
		return
	}
	user, _ := UserFromContext(r.Context())
	product, err := s.catalog.UpdateProduct(r.Context(), id, input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) priceHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r)
	if !ok {
		return
	}
	prices, err := s.catalog.PriceHistory(r.Context(), id)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": prices})
}

func (s *Server) catalogSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.catalog.Snapshot(r.Context())
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) previewBulkPrices(w http.ResponseWriter, r *http.Request) {
	var input catalog.BulkPriceRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	preview, err := s.catalog.PreviewBulkPrices(r.Context(), input)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) commitBulkPrices(w http.ResponseWriter, r *http.Request) {
	var input catalog.BulkPriceCommitRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	preview, err := s.catalog.CommitBulkPrices(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) previewProductImport(w http.ResponseWriter, r *http.Request) {
	var input catalog.ImportPreviewRequest
	if err := decodeJSONLimit(w, r, &input, 2<<20); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "El archivo o la solicitud no son válidos.", map[string]any{"reason": err.Error()})
		return
	}
	preview, err := s.catalog.PreviewImport(r.Context(), input.CSV)
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) commitProductImport(w http.ResponseWriter, r *http.Request) {
	var input catalog.ImportCommitRequest
	if err := decodeJSONLimit(w, r, &input, 2<<20); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "El archivo o la solicitud no son válidos.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	result, err := s.catalog.CommitImport(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) catalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Revisá los datos del catálogo.", map[string]any{"reason": err.Error()})
	case errors.Is(err, catalog.ErrConflict):
		writeError(w, r, http.StatusConflict, "catalog_conflict", "Ya existe un producto, categoría o código con ese valor.", nil)
	case errors.Is(err, catalog.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "No encontramos el elemento solicitado.", nil)
	default:
		s.options.Logger.ErrorContext(r.Context(), "catalog request failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos completar la operación de catálogo.", nil)
	}
}

func parsePathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_id", "El identificador no es válido.", nil)
		return uuid.Nil, false
	}
	return id, true
}

func emptyProductUpdate(input catalog.UpdateProductInput) bool {
	return input.CategoryID == nil && input.Name == nil && input.ProductType == nil &&
		input.SaleMode == nil && input.BaseUnit == nil && input.Sellable == nil &&
		input.TrackLots == nil && input.Active == nil && input.Barcode == nil && input.PriceCents == nil
}
