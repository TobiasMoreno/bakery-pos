package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"bakery-pos/backend/internal/pos"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) acquirePOSDevice(w http.ResponseWriter, r *http.Request) {
	var input pos.AcquireDeviceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	session, err := s.pos.AcquireDevice(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) releasePOSDevice(w http.ResponseWriter, r *http.Request) {
	var input pos.ReleaseDeviceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	if err := s.pos.ReleaseDevice(r.Context(), input, user.ID, RequestIDFromContext(r.Context())); err != nil {
		s.posError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) openCashShift(w http.ResponseWriter, r *http.Request) {
	var input pos.OpenShiftInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	shift, err := s.pos.OpenShift(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, shift)
}

func (s *Server) currentCashShift(w http.ResponseWriter, r *http.Request) {
	shift, err := s.pos.CurrentShift(r.Context())
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shift": shift})
}

func (s *Server) recordCashMovement(w http.ResponseWriter, r *http.Request) {
	shiftID, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	var input pos.RecordCashMovementInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	input.CashShiftID = shiftID
	user, _ := UserFromContext(r.Context())
	shift, err := s.pos.RecordCashMovement(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, shift)
}

func (s *Server) closeCashShift(w http.ResponseWriter, r *http.Request) {
	shiftID, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	var input pos.CloseShiftInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	input.CashShiftID = shiftID
	user, _ := UserFromContext(r.Context())
	shift, err := s.pos.CloseShift(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, shift)
}

func (s *Server) createSale(w http.ResponseWriter, r *http.Request) {
	var input pos.CreateSaleInput
	if err := decodeJSONLimit(w, r, &input, 64<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	sale, err := s.pos.CreateSale(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	status := http.StatusCreated
	if sale.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, sale)
}

func (s *Server) listSales(w http.ResponseWriter, r *http.Request) {
	filter := pos.SaleFilter{}
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El límite no es válido.", nil)
			return
		}
		filter.Limit = parsed
	}
	if value := strings.TrimSpace(r.URL.Query().Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_filter", "El desplazamiento no es válido.", nil)
			return
		}
		filter.Offset = parsed
	}
	result, err := s.pos.ListSales(r.Context(), filter)
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getSale(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	sale, err := s.pos.GetSale(r.Context(), id)
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sale)
}

func (s *Server) issueOfflineAuthorization(w http.ResponseWriter, r *http.Request) {
	var input pos.OfflineAuthorizationRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	grant, err := s.pos.IssueOfflineAuthorization(r.Context(), input, user.ID)
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, grant)
}

func (s *Server) syncOperations(w http.ResponseWriter, r *http.Request) {
	var input pos.SyncBatchInput
	if err := decodeJSONLimit(w, r, &input, 2<<20); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "El lote de sincronización no es válido.", map[string]any{"reason": err.Error()})
		return
	}
	result, err := s.pos.SyncOperations(r.Context(), input, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("device_session_id")))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_filter", "La sesión del dispositivo no es válida.", nil)
		return
	}
	status, err := s.pos.SyncStatus(r.Context(), sessionID)
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) forcePOSRecovery(w http.ResponseWriter, r *http.Request) {
	var input pos.ForceRecoveryInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La solicitud no es válida.", map[string]any{"reason": err.Error()})
		return
	}
	user, _ := UserFromContext(r.Context())
	recovery, err := s.pos.ForceRecovery(r.Context(), input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, recovery)
}

func (s *Server) currentPOSRecovery(w http.ResponseWriter, r *http.Request) {
	recovery, err := s.pos.ActiveRecovery(r.Context())
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery": recovery})
}

func (s *Server) addRecoveryDifference(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	var input pos.RecoveryDifferenceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La diferencia no es válida.", nil)
		return
	}
	user, _ := UserFromContext(r.Context())
	details, err := s.pos.AddRecoveryDifference(r.Context(), id, input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, details)
}

func (s *Server) resolveRecoveryDifference(w http.ResponseWriter, r *http.Request) {
	caseID, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	differenceID, ok := parseNamedUUID(w, r, "difference_id")
	if !ok {
		return
	}
	user, _ := UserFromContext(r.Context())
	details, err := s.pos.ResolveRecoveryDifference(r.Context(), caseID, differenceID, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) updateRecoveryControls(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	var input pos.RecoveryControlsInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Los controles no son válidos.", nil)
		return
	}
	user, _ := UserFromContext(r.Context())
	details, err := s.pos.UpdateRecoveryControls(r.Context(), id, input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) reconstructRecoverySale(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	var input pos.RecoveryReconstructionInput
	if err := decodeJSONLimit(w, r, &input, 64<<10); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "La reconstrucción no es válida.", nil)
		return
	}
	user, _ := UserFromContext(r.Context())
	sale, err := s.pos.ReconstructRecoverySale(r.Context(), id, input, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sale)
}

func (s *Server) closePOSRecovery(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNamedUUID(w, r, "id")
	if !ok {
		return
	}
	user, _ := UserFromContext(r.Context())
	details, err := s.pos.CloseRecovery(r.Context(), id, user.ID, RequestIDFromContext(r.Context()))
	if err != nil {
		s.posError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) posError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pos.ErrInvalid):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Revisá los datos de la operación.", map[string]any{"reason": err.Error()})
	case errors.Is(err, pos.ErrPaymentIncomplete):
		writeError(w, r, http.StatusUnprocessableEntity, "payment_incomplete", "Los pagos efectivos deben cubrir exactamente el total.", nil)
	case errors.Is(err, pos.ErrGrantInvalid):
		writeError(w, r, http.StatusUnauthorized, "offline_authorization_invalid", "La autorización offline no es válida o venció.", nil)
	case errors.Is(err, pos.ErrSyncConflict):
		writeError(w, r, http.StatusConflict, "sync_conflict", "La secuencia o integridad del diario no coincide.", nil)
	case errors.Is(err, pos.ErrRecoveryRequired):
		writeError(w, r, http.StatusConflict, "recovery_required", "El dispositivo está en modo de recuperación.", nil)
	case errors.Is(err, pos.ErrNoActiveDevice):
		writeError(w, r, http.StatusConflict, "pos_device_required", "Este equipo ya no es el puesto de venta activo.", nil)
	case errors.Is(err, pos.ErrNoOpenShift):
		writeError(w, r, http.StatusConflict, "cash_shift_required", "Abrí un turno de caja antes de continuar.", nil)
	case errors.Is(err, pos.ErrConflict):
		writeError(w, r, http.StatusConflict, "pos_conflict", "La operación quedó desactualizada. Refrescá el estado antes de reintentar.", nil)
	case errors.Is(err, pos.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "No encontramos el recurso solicitado.", nil)
	default:
		s.options.Logger.ErrorContext(r.Context(), "pos request failed", "request_id", RequestIDFromContext(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, "internal_error", "No pudimos completar la operación del mostrador.", nil)
	}
}

func parseNamedUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_id", "El identificador no es válido.", nil)
		return uuid.Nil, false
	}
	return id, true
}
