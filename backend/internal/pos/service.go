package pos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

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
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
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
