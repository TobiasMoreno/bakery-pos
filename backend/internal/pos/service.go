package pos

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

type Repository interface {
	AcquireDevice(context.Context, AcquireDeviceInput, uuid.UUID, string) (DeviceSession, error)
	ReleaseDevice(context.Context, ReleaseDeviceInput, uuid.UUID, string) error
	OpenShift(context.Context, OpenShiftInput, uuid.UUID, string) (CashShift, error)
	CurrentShift(context.Context) (*CashShift, error)
	RecordCashMovement(context.Context, RecordCashMovementInput, uuid.UUID, string) (CashShift, error)
	CloseShift(context.Context, CloseShiftInput, uuid.UUID, string) (CashShift, error)
	CreateSale(context.Context, CreateSaleInput, uuid.UUID, string, string) (Sale, error)
	ListSales(context.Context, SaleFilter) ([]Sale, int, error)
	GetSale(context.Context, uuid.UUID) (Sale, error)
	ValidateOfflinePreparation(context.Context, OfflineAuthorizationRequest, uuid.UUID) (DeviceSession, error)
	ReserveSyncOperation(context.Context, OfflineOperation, uuid.UUID, string) (SyncReservation, error)
	CompleteSyncOperation(context.Context, OfflineOperation, uuid.UUID, uuid.UUID, string) (SyncACK, error)
	SyncStatus(context.Context, uuid.UUID) (SyncStatus, error)
	ForceRecovery(context.Context, ForceRecoveryInput, uuid.UUID, string) (RecoveryCase, error)
	OpenOfflineShift(context.Context, OfflineOpenShiftInput, uuid.UUID, string) (CashShift, error)
	CloseOfflineShift(context.Context, CloseShiftInput, uuid.UUID, string) (CashShift, error)
	GetActiveRecovery(context.Context) (*RecoveryDetails, error)
	AddRecoveryDifference(context.Context, uuid.UUID, RecoveryDifferenceInput, uuid.UUID, string) (RecoveryDetails, error)
	UpdateRecoveryControls(context.Context, uuid.UUID, RecoveryControlsInput, uuid.UUID, string) (RecoveryDetails, error)
	CloseRecovery(context.Context, uuid.UUID, uuid.UUID, string) (RecoveryDetails, error)
	RecordReconstruction(context.Context, uuid.UUID, RecoveryReconstructionInput, Sale, uuid.UUID, string) error
}

type Service struct {
	repository Repository
	signingKey []byte
	now        func() time.Time
}

func NewService(repository Repository, signingKeys ...string) *Service {
	key := "tests-only-offline-signing-key-32-bytes"
	if len(signingKeys) > 0 && signingKeys[0] != "" {
		key = signingKeys[0]
	}
	return &Service{repository: repository, signingKey: []byte(key), now: time.Now}
}

func (s *Service) AcquireDevice(ctx context.Context, input AcquireDeviceInput, actorID uuid.UUID, requestID string) (DeviceSession, error) {
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	if input.DeviceID == uuid.Nil || len(input.DeviceName) < 1 || len(input.DeviceName) > 80 || actorID == uuid.Nil || requestID == "" {
		return DeviceSession{}, fmt.Errorf("%w: device id, name, actor and request id are required", ErrInvalid)
	}
	return s.repository.AcquireDevice(ctx, input, actorID, requestID)
}

func (s *Service) ReleaseDevice(ctx context.Context, input ReleaseDeviceInput, actorID uuid.UUID, requestID string) error {
	if input.SessionID == uuid.Nil || input.FencingVersion <= 0 || input.PendingOperations < 0 || actorID == uuid.Nil || requestID == "" {
		return fmt.Errorf("%w: invalid device release", ErrInvalid)
	}
	if input.PendingOperations != 0 {
		return fmt.Errorf("%w: synchronize pending operations before releasing the device", ErrConflict)
	}
	return s.repository.ReleaseDevice(ctx, input, actorID, requestID)
}

func (s *Service) OpenShift(ctx context.Context, input OpenShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	input.OpeningNote = strings.TrimSpace(input.OpeningNote)
	if input.DeviceSessionID == uuid.Nil || input.FencingVersion <= 0 || input.OpeningCashCents < 0 || len(input.OpeningNote) > 300 || actorID == uuid.Nil || requestID == "" {
		return CashShift{}, fmt.Errorf("%w: invalid cash shift opening", ErrInvalid)
	}
	return s.repository.OpenShift(ctx, input, actorID, requestID)
}

func (s *Service) CurrentShift(ctx context.Context) (*CashShift, error) {
	return s.repository.CurrentShift(ctx)
}

func (s *Service) RecordCashMovement(ctx context.Context, input RecordCashMovementInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	input.Recipient = strings.TrimSpace(input.Recipient)
	if input.ID == uuid.Nil || input.CashShiftID == uuid.Nil || input.ExpectedVersion <= 0 || input.AmountCents <= 0 || actorID == uuid.Nil || requestID == "" {
		return CashShift{}, fmt.Errorf("%w: invalid cash movement", ErrInvalid)
	}
	if input.MovementType != CashMovementIncome && input.MovementType != CashMovementExpense && input.MovementType != CashMovementWithdrawal {
		return CashShift{}, fmt.Errorf("%w: unsupported manual cash movement type", ErrInvalid)
	}
	if len(input.Reason) < 3 || len(input.Reason) > 300 {
		return CashShift{}, fmt.Errorf("%w: a reason between 3 and 300 characters is required", ErrInvalid)
	}
	if input.MovementType == CashMovementWithdrawal && (len(input.Recipient) < 2 || len(input.Recipient) > 120) {
		return CashShift{}, fmt.Errorf("%w: withdrawal recipient is required", ErrInvalid)
	}
	return s.repository.RecordCashMovement(ctx, input, actorID, requestID)
}

func (s *Service) CloseShift(ctx context.Context, input CloseShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	input.ClosingNote = strings.TrimSpace(input.ClosingNote)
	if input.CashShiftID == uuid.Nil || input.ExpectedVersion <= 0 || input.CountedCashCents < 0 || len(input.ClosingNote) > 300 || actorID == uuid.Nil || requestID == "" {
		return CashShift{}, fmt.Errorf("%w: invalid cash shift closing", ErrInvalid)
	}
	return s.repository.CloseShift(ctx, input, actorID, requestID)
}

func (s *Service) CreateSale(ctx context.Context, input CreateSaleInput, actorID uuid.UUID, requestID string) (Sale, error) {
	if input.OperationID == uuid.Nil || input.DeviceSessionID == uuid.Nil || input.CashShiftID == uuid.Nil ||
		input.FencingVersion <= 0 || input.ShiftVersion <= 0 || input.CatalogVersion <= 0 || actorID == uuid.Nil || requestID == "" {
		return Sale{}, fmt.Errorf("%w: sale identifiers and versions are required", ErrInvalid)
	}
	if len(input.Items) == 0 || len(input.Items) > 100 || len(input.Payments) == 0 || len(input.Payments) > 10 {
		return Sale{}, fmt.Errorf("%w: a sale requires items and payments", ErrInvalid)
	}
	seenProducts := make(map[uuid.UUID]struct{}, len(input.Items))
	for _, item := range input.Items {
		if item.ProductID == uuid.Nil || item.Quantity.Milliunits() <= 0 {
			return Sale{}, fmt.Errorf("%w: item product and positive quantity are required", ErrInvalid)
		}
		if _, exists := seenProducts[item.ProductID]; exists {
			return Sale{}, fmt.Errorf("%w: duplicate products must be consolidated into one line", ErrInvalid)
		}
		seenProducts[item.ProductID] = struct{}{}
	}
	for _, payment := range input.Payments {
		if err := validatePayment(payment); err != nil {
			return Sale{}, err
		}
	}
	payloadHash, err := salePayloadHash(input, actorID)
	if err != nil {
		return Sale{}, fmt.Errorf("encode sale idempotency payload: %w", err)
	}
	return s.repository.CreateSale(ctx, input, actorID, requestID, payloadHash)
}

func (s *Service) ListSales(ctx context.Context, filter SaleFilter) (SaleList, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		return SaleList{}, fmt.Errorf("%w: offset cannot be negative", ErrInvalid)
	}
	items, total, err := s.repository.ListSales(ctx, filter)
	if err != nil {
		return SaleList{}, err
	}
	return SaleList{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) GetSale(ctx context.Context, id uuid.UUID) (Sale, error) {
	if id == uuid.Nil {
		return Sale{}, fmt.Errorf("%w: sale id is required", ErrInvalid)
	}
	return s.repository.GetSale(ctx, id)
}

func (s *Service) IssueOfflineAuthorization(ctx context.Context, input OfflineAuthorizationRequest, actorID uuid.UUID) (OfflineAuthorization, error) {
	if input.DeviceSessionID == uuid.Nil || input.FencingVersion <= 0 || input.CashShiftID == uuid.Nil || input.ShiftVersion <= 0 || input.CatalogVersion <= 0 || actorID == uuid.Nil {
		return OfflineAuthorization{}, fmt.Errorf("%w: identifiers and versions are required", ErrInvalid)
	}
	session, err := s.repository.ValidateOfflinePreparation(ctx, input, actorID)
	if err != nil {
		return OfflineAuthorization{}, err
	}
	now := s.now().UTC()
	grant := OfflineAuthorization{
		UserID: actorID, DeviceID: session.DeviceID, DeviceSessionID: session.ID,
		FencingVersion: session.FencingVersion, CashShiftID: input.CashShiftID,
		ShiftVersion: input.ShiftVersion, CatalogVersion: input.CatalogVersion,
		IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	}
	body, err := json.Marshal(grant)
	if err != nil {
		return OfflineAuthorization{}, fmt.Errorf("encode offline authorization: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(encoded))
	grant.Token = encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return grant, nil
}

func (s *Service) SyncOperations(ctx context.Context, input SyncBatchInput, requestID string) (SyncBatchResult, error) {
	if len(input.Operations) == 0 || len(input.Operations) > 50 || requestID == "" {
		return SyncBatchResult{}, fmt.Errorf("%w: a batch requires between 1 and 50 operations", ErrInvalid)
	}
	result := SyncBatchResult{ACKs: make([]SyncACK, 0, len(input.Operations))}
	for index := range input.Operations {
		op := input.Operations[index]
		grant, err := s.validateOfflineOperation(op)
		if err != nil {
			return result, err
		}
		reservation, err := s.repository.ReserveSyncOperation(ctx, op, grant.UserID, requestID)
		if err != nil {
			return result, err
		}
		if reservation.ACK != nil {
			result.ACKs = append(result.ACKs, *reservation.ACK)
			if reservation.ACK.Status != "acknowledged" {
				return result, nil
			}
			continue
		}
		if reservation.Evidence {
			ack, err := s.repository.CompleteSyncOperation(ctx, op, uuid.Nil, uuid.Nil, requestID)
			if err != nil {
				return result, err
			}
			result.ACKs = append(result.ACKs, ack)
			return result, nil
		}
		var saleID, shiftID uuid.UUID
		switch op.OperationType {
		case "sale":
			var payload CreateSaleInput
			if err := strictDecode(op.Payload, &payload); err != nil {
				return result, fmt.Errorf("%w: invalid sale payload", ErrInvalid)
			}
			payload.offlineCatalogAt = &grant.IssuedAt
			payload.offlineCreatedAt = &op.CreatedAtLocal
			sale, err := s.CreateSale(ctx, payload, grant.UserID, requestID)
			if err != nil {
				return result, err
			}
			saleID = sale.ID
		case "shift_close":
			var payload OfflineCloseShiftInput
			if err := strictDecode(op.Payload, &payload); err != nil {
				return result, fmt.Errorf("%w: invalid shift close payload", ErrInvalid)
			}
			closed, err := s.repository.CloseOfflineShift(ctx, CloseShiftInput(payload), grant.UserID, requestID)
			if err != nil {
				return result, err
			}
			shiftID = closed.ID
		case "shift_open":
			var payload OfflineOpenShiftInput
			if err := strictDecode(op.Payload, &payload); err != nil {
				return result, fmt.Errorf("%w: invalid shift open payload", ErrInvalid)
			}
			opened, err := s.repository.OpenOfflineShift(ctx, payload, grant.UserID, requestID)
			if err != nil {
				return result, err
			}
			shiftID = opened.ID
		default:
			return result, fmt.Errorf("%w: unsupported offline operation", ErrInvalid)
		}
		ack, err := s.repository.CompleteSyncOperation(ctx, op, saleID, shiftID, requestID)
		if err != nil {
			return result, err
		}
		result.ACKs = append(result.ACKs, ack)
	}
	return result, nil
}

func (s *Service) SyncStatus(ctx context.Context, sessionID uuid.UUID) (SyncStatus, error) {
	if sessionID == uuid.Nil {
		return SyncStatus{}, fmt.Errorf("%w: device session is required", ErrInvalid)
	}
	return s.repository.SyncStatus(ctx, sessionID)
}

func (s *Service) ForceRecovery(ctx context.Context, input ForceRecoveryInput, actorID uuid.UUID, requestID string) (RecoveryCase, error) {
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.PreviousSessionID == uuid.Nil || input.DeviceID == uuid.Nil || len(input.DeviceName) < 1 || len(input.DeviceName) > 80 || len(input.Reason) < 10 || len(input.Reason) > 500 || input.CashCountCents < 0 || actorID == uuid.Nil || requestID == "" {
		return RecoveryCase{}, fmt.Errorf("%w: invalid recovery request", ErrInvalid)
	}
	return s.repository.ForceRecovery(ctx, input, actorID, requestID)
}

func (s *Service) ActiveRecovery(ctx context.Context) (*RecoveryDetails, error) {
	return s.repository.GetActiveRecovery(ctx)
}

func (s *Service) AddRecoveryDifference(ctx context.Context, caseID uuid.UUID, input RecoveryDifferenceInput, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	input.Description = strings.TrimSpace(input.Description)
	validKind := input.Kind == "unknown_sale_total" || input.Kind == "cash" || input.Kind == "digital_payment" || input.Kind == "inventory" || input.Kind == "sequence_gap"
	if caseID == uuid.Nil || input.ID == uuid.Nil || !validKind || len(input.Description) < 3 || len(input.Description) > 500 || actorID == uuid.Nil || requestID == "" {
		return RecoveryDetails{}, fmt.Errorf("%w: invalid recovery difference", ErrInvalid)
	}
	return s.repository.AddRecoveryDifference(ctx, caseID, input, actorID, requestID)
}

func (s *Service) UpdateRecoveryControls(ctx context.Context, caseID uuid.UUID, input RecoveryControlsInput, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	if caseID == uuid.Nil || actorID == uuid.Nil || requestID == "" || (input.JournalStatus != "imported" && input.JournalStatus != "unrecoverable") {
		return RecoveryDetails{}, fmt.Errorf("%w: invalid recovery controls", ErrInvalid)
	}
	return s.repository.UpdateRecoveryControls(ctx, caseID, input, actorID, requestID)
}

func (s *Service) CloseRecovery(ctx context.Context, caseID, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	if caseID == uuid.Nil || actorID == uuid.Nil || requestID == "" {
		return RecoveryDetails{}, fmt.Errorf("%w: invalid recovery closure", ErrInvalid)
	}
	return s.repository.CloseRecovery(ctx, caseID, actorID, requestID)
}

func (s *Service) ReconstructRecoverySale(ctx context.Context, caseID uuid.UUID, input RecoveryReconstructionInput, actorID uuid.UUID, requestID string) (Sale, error) {
	input.Evidence = strings.TrimSpace(input.Evidence)
	if caseID == uuid.Nil || input.ReconciliationOperationID == uuid.Nil || actorID == uuid.Nil || requestID == "" || (input.Certainty != "exact" && input.Certainty != "probable") || len(input.Evidence) < 3 || len(input.Evidence) > 500 {
		return Sale{}, fmt.Errorf("%w: invalid recovery reconstruction", ErrInvalid)
	}
	input.Sale.OperationID = input.ReconciliationOperationID
	input.Sale.recoveryCaseID = &caseID
	sale, err := s.CreateSale(ctx, input.Sale, actorID, requestID)
	if err != nil {
		return Sale{}, err
	}
	if err := s.repository.RecordReconstruction(ctx, caseID, input, sale, actorID, requestID); err != nil {
		return Sale{}, err
	}
	return sale, nil
}

func (s *Service) validateOfflineOperation(op OfflineOperation) (OfflineAuthorization, error) {
	if op.OperationID == uuid.Nil || op.DeviceID == uuid.Nil || op.DeviceSessionID == uuid.Nil || op.FencingVersion <= 0 || op.Sequence <= 0 || op.CreatedAtLocal.IsZero() || len(op.Timezone) < 1 || len(op.Timezone) > 80 || !validHash(op.PayloadHash) || (op.PreviousHash != "" && !validHash(op.PreviousHash)) || len(op.Payload) == 0 {
		return OfflineAuthorization{}, fmt.Errorf("%w: invalid operation envelope", ErrInvalid)
	}
	if (op.Timezone != "UTC" && !strings.Contains(op.Timezone, "/")) || op.CreatedAtLocal.After(s.now().UTC().Add(5*time.Minute)) {
		return OfflineAuthorization{}, fmt.Errorf("%w: invalid local timestamp or timezone", ErrInvalid)
	}
	grant, err := s.decodeGrant(op.Authorization)
	if err != nil {
		return OfflineAuthorization{}, err
	}
	if grant.ExpiresAt.Before(op.CreatedAtLocal) || op.CreatedAtLocal.Before(grant.IssuedAt.Add(-5*time.Minute)) ||
		grant.ExpiresAt.Before(grant.IssuedAt) || grant.ExpiresAt.After(grant.IssuedAt.Add(24*time.Hour+time.Minute)) ||
		grant.DeviceID != op.DeviceID || grant.DeviceSessionID != op.DeviceSessionID || grant.FencingVersion != op.FencingVersion {
		return OfflineAuthorization{}, ErrGrantInvalid
	}
	switch op.OperationType {
	case "sale":
		var payload CreateSaleInput
		if err := strictDecode(op.Payload, &payload); err != nil || payload.OperationID != op.OperationID || payload.DeviceSessionID != op.DeviceSessionID || payload.FencingVersion != op.FencingVersion || payload.CatalogVersion != grant.CatalogVersion {
			return OfflineAuthorization{}, ErrGrantInvalid
		}
		for _, payment := range payload.Payments {
			if payment.Method != PaymentCash && payment.Status == PaymentConfirmed {
				return OfflineAuthorization{}, fmt.Errorf("%w: offline digital payments can only be reported", ErrInvalid)
			}
		}
	case "shift_close":
		var payload OfflineCloseShiftInput
		if err := strictDecode(op.Payload, &payload); err != nil || payload.CashShiftID != grant.CashShiftID {
			return OfflineAuthorization{}, ErrGrantInvalid
		}
	case "shift_open":
		var payload OfflineOpenShiftInput
		if err := strictDecode(op.Payload, &payload); err != nil || payload.ID == uuid.Nil || payload.PreviousCashShiftID != grant.CashShiftID || payload.DeviceSessionID != op.DeviceSessionID || payload.FencingVersion != op.FencingVersion || payload.OpeningCashCents < 0 {
			return OfflineAuthorization{}, ErrGrantInvalid
		}
	default:
		return OfflineAuthorization{}, fmt.Errorf("%w: unsupported operation type", ErrInvalid)
	}
	hash, err := canonicalRawPayloadHash(op.Payload)
	if err != nil || !hmac.Equal([]byte(hash), []byte(op.PayloadHash)) {
		return OfflineAuthorization{}, fmt.Errorf("%w: payload hash does not match", ErrSyncConflict)
	}
	return grant, nil
}

func (s *Service) decodeGrant(token string) (OfflineAuthorization, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return OfflineAuthorization{}, ErrGrantInvalid
	}
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return OfflineAuthorization{}, ErrGrantInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return OfflineAuthorization{}, ErrGrantInvalid
	}
	var grant OfflineAuthorization
	if err := json.Unmarshal(body, &grant); err != nil || grant.UserID == uuid.Nil {
		return OfflineAuthorization{}, ErrGrantInvalid
	}
	grant.Token = token
	return grant, nil
}

func canonicalPayloadHash(payload CreateSaleInput) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return canonicalRawPayloadHash(raw)
}

func canonicalRawPayloadHash(raw json.RawMessage) (string, error) {
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func strictDecode(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validatePayment(payment PaymentInput) error {
	payment.ExternalReference = strings.TrimSpace(payment.ExternalReference)
	payment.VerificationMethod = strings.TrimSpace(payment.VerificationMethod)
	if payment.AmountCents <= 0 || len(payment.ExternalReference) > 160 || len(payment.VerificationMethod) > 120 {
		return fmt.Errorf("%w: invalid payment amount or evidence", ErrInvalid)
	}
	switch payment.Method {
	case PaymentCash:
		if payment.Status != PaymentConfirmed || payment.ReceivedCents == nil || *payment.ReceivedCents < payment.AmountCents {
			return fmt.Errorf("%w: cash must be confirmed and received amount must cover its allocation", ErrInvalid)
		}
	case PaymentTransfer, PaymentQR, PaymentCard, PaymentOther:
		if payment.ReceivedCents != nil {
			return fmt.Errorf("%w: received amount only applies to cash", ErrInvalid)
		}
		if payment.Status == PaymentConfirmed && len(strings.TrimSpace(payment.VerificationMethod)) < 3 {
			return fmt.Errorf("%w: confirmed digital payments require a verification method", ErrInvalid)
		}
		if payment.Status != PaymentReported && payment.Status != PaymentConfirmed {
			return fmt.Errorf("%w: pending or rejected payments cannot complete a sale", ErrPaymentIncomplete)
		}
	default:
		return fmt.Errorf("%w: unsupported payment method", ErrInvalid)
	}
	return nil
}

// CalculateSubtotal applies the documented half-up rule using integer arithmetic.
// Quantity is expressed in the product base unit: units for unit sales and grams for weight sales.
func CalculateSubtotal(priceCents int64, saleMode string, quantity inventory.Quantity) (int64, error) {
	if priceCents <= 0 || quantity.Milliunits() <= 0 {
		return 0, fmt.Errorf("%w: price and quantity must be positive", ErrInvalid)
	}
	denominator := int64(1_000)
	if saleMode == "unit" {
		if !quantity.IsInteger() {
			return 0, fmt.Errorf("%w: unit sales require an integer quantity", ErrInvalid)
		}
	} else if saleMode == "weight" {
		denominator = 1_000_000
	} else {
		return 0, fmt.Errorf("%w: unsupported sale mode", ErrInvalid)
	}

	numerator := new(big.Int).Mul(big.NewInt(priceCents), big.NewInt(quantity.Milliunits()))
	numerator.Add(numerator, big.NewInt(denominator/2))
	result := new(big.Int).Quo(numerator, big.NewInt(denominator))
	if !result.IsInt64() || result.Sign() <= 0 {
		return 0, fmt.Errorf("%w: sale subtotal is out of range", ErrInvalid)
	}
	return result.Int64(), nil
}

func salePayloadHash(input CreateSaleInput, actorID uuid.UUID) (string, error) {
	payload, err := json.Marshal(struct {
		ActorID uuid.UUID       `json:"actor_id"`
		Input   CreateSaleInput `json:"input"`
	}{ActorID: actorID, Input: input})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
