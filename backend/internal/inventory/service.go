package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Service struct {
	repository *PostgresRepository
}

func NewService(repository *PostgresRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListBalances(ctx context.Context, filter BalanceFilter) (BalanceList, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit, filter.Offset = normalizedPagination(filter.Limit, filter.Offset)
	items, total, err := s.repository.ListBalances(ctx, filter)
	if err != nil {
		return BalanceList{}, err
	}
	return BalanceList{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) ListMovements(ctx context.Context, filter MovementFilter) (MovementList, error) {
	filter.Limit, filter.Offset = normalizedPagination(filter.Limit, filter.Offset)
	if filter.MovementType != nil && !validMovementType(*filter.MovementType) {
		return MovementList{}, fmt.Errorf("%w: unknown movement type", ErrInvalid)
	}
	items, total, err := s.repository.ListMovements(ctx, filter)
	if err != nil {
		return MovementList{}, err
	}
	return MovementList{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) RecordMovement(ctx context.Context, input RecordMovementInput, actorID uuid.UUID, requestID string) (RecordResult, error) {
	input.ReferenceType = strings.TrimSpace(input.ReferenceType)
	input.Reason = strings.Join(strings.Fields(input.Reason), " ")
	requestID = strings.TrimSpace(requestID)
	if err := validateMovementInput(input, actorID, requestID); err != nil {
		return RecordResult{}, err
	}
	payloadHash, err := movementPayloadHash(input, actorID)
	if err != nil {
		return RecordResult{}, err
	}
	return s.repository.RecordMovement(ctx, input, actorID, requestID, payloadHash)
}

func (s *Service) ReconstructBalance(ctx context.Context, productID uuid.UUID) (Quantity, error) {
	if productID == uuid.Nil {
		return Quantity{}, fmt.Errorf("%w: product is required", ErrInvalid)
	}
	return s.repository.ReconstructBalance(ctx, productID)
}

func validateMovementInput(input RecordMovementInput, actorID uuid.UUID, requestID string) error {
	if input.ID == uuid.Nil {
		return fmt.Errorf("%w: movement id is required", ErrInvalid)
	}
	if input.ProductID == uuid.Nil {
		return fmt.Errorf("%w: product is required", ErrInvalid)
	}
	if input.QuantityDelta.IsZero() {
		return fmt.Errorf("%w: quantity delta cannot be zero", ErrInvalid)
	}
	if !validMovementType(input.MovementType) {
		return fmt.Errorf("%w: unknown movement type", ErrInvalid)
	}
	if utf8.RuneCountInString(input.ReferenceType) < 1 || utf8.RuneCountInString(input.ReferenceType) > 80 {
		return fmt.Errorf("%w: reference type must contain between 1 and 80 characters", ErrInvalid)
	}
	if utf8.RuneCountInString(input.Reason) > 500 {
		return fmt.Errorf("%w: reason is too long", ErrInvalid)
	}
	if requiresReason(input.MovementType) && input.Reason == "" {
		return fmt.Errorf("%w: reason is required for this movement type", ErrInvalid)
	}
	if actorID == uuid.Nil {
		return fmt.Errorf("%w: actor is required", ErrInvalid)
	}
	if requestID == "" || len(requestID) > 200 {
		return fmt.Errorf("%w: request id is required", ErrInvalid)
	}
	return nil
}

func movementPayloadHash(input RecordMovementInput, actorID uuid.UUID) (string, error) {
	payload := struct {
		ProductID     uuid.UUID    `json:"product_id"`
		QuantityDelta string       `json:"quantity_delta"`
		MovementType  MovementType `json:"movement_type"`
		ReferenceType string       `json:"reference_type"`
		ReferenceID   *uuid.UUID   `json:"reference_id"`
		ActorID       uuid.UUID    `json:"actor_id"`
		Reason        string       `json:"reason"`
	}{
		ProductID:     input.ProductID,
		QuantityDelta: input.QuantityDelta.String(),
		MovementType:  input.MovementType,
		ReferenceType: input.ReferenceType,
		ReferenceID:   input.ReferenceID,
		ActorID:       actorID,
		Reason:        input.Reason,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode inventory idempotency payload: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func normalizedPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func validMovementType(value MovementType) bool {
	switch value {
	case MovementTypeInitial, MovementTypePurchase, MovementTypeProduction, MovementTypeSale,
		MovementTypeReturn, MovementTypeWaste, MovementTypeAdjustment:
		return true
	default:
		return false
	}
}

func requiresReason(value MovementType) bool {
	return value == MovementTypeInitial || value == MovementTypeWaste || value == MovementTypeAdjustment
}
