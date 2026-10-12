package inventory

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateMovementInputRequiresReasonForManualChanges(t *testing.T) {
	quantity, err := ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	input := RecordMovementInput{
		ID:            uuid.New(),
		ProductID:     uuid.New(),
		QuantityDelta: quantity,
		MovementType:  MovementTypeAdjustment,
		ReferenceType: "manual_adjustment",
	}
	if err := validateMovementInput(input, uuid.New(), "request-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
	input.Reason = "Conteo físico"
	if err := validateMovementInput(input, uuid.New(), "request-1"); err != nil {
		t.Fatalf("validateMovementInput() error = %v", err)
	}
}

func TestMovementPayloadHashIgnoresRequestIDAndChangesWithPayload(t *testing.T) {
	quantity, err := ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	actorID := uuid.New()
	input := RecordMovementInput{
		ID:            uuid.New(),
		ProductID:     uuid.New(),
		QuantityDelta: quantity,
		MovementType:  MovementTypePurchase,
		ReferenceType: "purchase",
		ReferenceID:   uuidPointer(uuid.New()),
	}
	first, err := movementPayloadHash(input, actorID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := movementPayloadHash(input, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equal payloads produced different hashes")
	}
	input.ReferenceType = "receipt"
	changed, err := movementPayloadHash(input, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("different payloads produced the same hash")
	}
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	return &value
}
