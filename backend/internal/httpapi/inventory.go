package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

func (s *Server) listInventoryBalances(w http.ResponseWriter, r *http.Request) {
	filter := inventory.BalanceFilter{Query: r.URL.Query().Get("q")}
	limit, offset, ok := parseInventoryPagination(w, r)
	if !ok {
		return
	}
	filter.Limit = limit
	filter.Offset = offset

	balances, err := s.inventory.ListBalances(r.Context(), filter)
	if err != nil {
		s.inventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, balances)
}

func (s *Server) listInventoryMovements(w http.ResponseWriter, r *http.Request) {
	filter := inventory.MovementFilter{}
	if value := strings.TrimSpace(r.URL.Query().Get("product_id")); value != "" {
		productID, err := uuid.Parse(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El producto indicado no es válido.", nil)
			return
		}
		filter.ProductID = &productID
	}
	if value := strings.TrimSpace(r.URL.Query().Get("movement_type")); value != "" {
		movementType := inventory.MovementType(value)
		filter.MovementType = &movementType
	}
	limit, offset, ok := parseInventoryPagination(w, r)
	if !ok {
		return
	}
	filter.Limit = limit
	filter.Offset = offset

	movements, err := s.inventory.ListMovements(r.Context(), filter)
	if err != nil {
		s.inventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, movements)
}

func parseInventoryPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	var limit, offset int
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El límite no es válido.", nil)
			return 0, 0, false
		}
		limit = parsed
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El desplazamiento no es válido.", nil)
			return 0, 0, false
		}
		offset = parsed
	}
	return limit, offset, true
}

func (s *Server) inventoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, inventory.ErrInvalid), errors.Is(err, inventory.ErrInvalidQuantity):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Revisá los filtros de inventario.", nil)
	case errors.Is(err, inventory.ErrConflict):
		writeError(w, r, http.StatusConflict, "inventory_conflict", "La operación de inventario entra en conflicto con una anterior.", nil)
	case errors.Is(err, inventory.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "No encontramos el elemento de inventario solicitado.", nil)
	default:
		s.options.Logger.ErrorContext(r.Context(), "inventory request failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos consultar el inventario.", nil)
	}
}
