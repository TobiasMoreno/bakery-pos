package inventory

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("inventory item not found")
	ErrConflict = errors.New("inventory operation conflict")
	ErrInvalid  = errors.New("invalid inventory data")
)

type BaseUnit string

const (
	BaseUnitUnit BaseUnit = "unit"
	BaseUnitGram BaseUnit = "g"
	BaseUnitML   BaseUnit = "ml"
)

type MovementType string

const (
	MovementTypeInitial    MovementType = "initial"
	MovementTypePurchase   MovementType = "purchase"
	MovementTypeProduction MovementType = "production"
	MovementTypeSale       MovementType = "sale"
	MovementTypeReturn     MovementType = "return"
	MovementTypeWaste      MovementType = "waste"
	MovementTypeAdjustment MovementType = "adjustment"
)

type Balance struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	BaseUnit    BaseUnit  `json:"base_unit"`
	Quantity    Quantity  `json:"quantity"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BalanceFilter struct {
	Query  string
	Limit  int
	Offset int
}

type BalanceList struct {
	Items  []Balance `json:"items"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

type Movement struct {
	ID            uuid.UUID    `json:"id"`
	ProductID     uuid.UUID    `json:"product_id"`
	ProductName   string       `json:"product_name"`
	BaseUnit      BaseUnit     `json:"base_unit"`
	QuantityDelta Quantity     `json:"quantity_delta"`
	MovementType  MovementType `json:"movement_type"`
	ReferenceType string       `json:"reference_type"`
	ReferenceID   *uuid.UUID   `json:"reference_id"`
	ActorID       uuid.UUID    `json:"actor_id"`
	RequestID     string       `json:"request_id"`
	Reason        string       `json:"reason,omitempty"`
	OccurredAt    time.Time    `json:"occurred_at"`
	CreatedAt     time.Time    `json:"created_at"`
}

type MovementFilter struct {
	ProductID    *uuid.UUID
	MovementType *MovementType
	Limit        int
	Offset       int
}

type MovementList struct {
	Items  []Movement `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type RecordMovementInput struct {
	ID            uuid.UUID
	ProductID     uuid.UUID
	QuantityDelta Quantity
	MovementType  MovementType
	ReferenceType string
	ReferenceID   *uuid.UUID
	Reason        string
}

type RecordResult struct {
	Movement Movement `json:"movement"`
	Balance  Balance  `json:"balance"`
	Replayed bool     `json:"replayed"`
}
