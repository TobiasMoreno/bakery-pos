package pos

import (
	"encoding/json"
	"errors"
	"time"

	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

var (
	ErrConflict          = errors.New("pos conflict")
	ErrInvalid           = errors.New("invalid pos data")
	ErrNotFound          = errors.New("pos resource not found")
	ErrNoActiveDevice    = errors.New("active pos device required")
	ErrNoOpenShift       = errors.New("open cash shift required")
	ErrPaymentIncomplete = errors.New("effective payments must equal sale total")
	ErrSyncConflict      = errors.New("offline synchronization conflict")
	ErrGrantInvalid      = errors.New("offline authorization is invalid or expired")
	ErrRecoveryRequired  = errors.New("device recovery is required")
)

type DeviceSession struct {
	ID             uuid.UUID  `json:"id"`
	DeviceID       uuid.UUID  `json:"device_id"`
	DeviceName     string     `json:"device_name"`
	UserID         uuid.UUID  `json:"user_id"`
	Status         string     `json:"status"`
	FencingVersion int64      `json:"fencing_version"`
	AcquiredAt     time.Time  `json:"acquired_at"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
}

type AcquireDeviceInput struct {
	DeviceID   uuid.UUID `json:"device_id"`
	DeviceName string    `json:"device_name"`
}

type ReleaseDeviceInput struct {
	SessionID         uuid.UUID `json:"session_id"`
	FencingVersion    int64     `json:"fencing_version"`
	PendingOperations int       `json:"pending_operations"`
}

type CashShift struct {
	ID                uuid.UUID  `json:"id"`
	Status            string     `json:"status"`
	Version           int64      `json:"version"`
	DeviceSessionID   uuid.UUID  `json:"device_session_id"`
	OpenedBy          uuid.UUID  `json:"opened_by"`
	OpeningCashCents  int64      `json:"opening_cash_cents"`
	OpeningNote       string     `json:"opening_note,omitempty"`
	OpenedAt          time.Time  `json:"opened_at"`
	ClosedBy          *uuid.UUID `json:"closed_by,omitempty"`
	CountedCashCents  *int64     `json:"counted_cash_cents,omitempty"`
	ExpectedCashCents int64      `json:"expected_cash_cents"`
	DifferenceCents   *int64     `json:"difference_cents,omitempty"`
	ClosingNote       string     `json:"closing_note,omitempty"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

type OpenShiftInput struct {
	DeviceSessionID  uuid.UUID `json:"device_session_id"`
	FencingVersion   int64     `json:"fencing_version"`
	OpeningCashCents int64     `json:"opening_cash_cents"`
	OpeningNote      string    `json:"opening_note"`
}

type CashMovementType string

const (
	CashMovementSale       CashMovementType = "sale"
	CashMovementIncome     CashMovementType = "income"
	CashMovementExpense    CashMovementType = "expense"
	CashMovementWithdrawal CashMovementType = "withdrawal"
	CashMovementRefund     CashMovementType = "refund"
)

type CashMovement struct {
	ID           uuid.UUID        `json:"id"`
	CashShiftID  uuid.UUID        `json:"cash_shift_id"`
	MovementType CashMovementType `json:"movement_type"`
	AmountCents  int64            `json:"amount_cents"`
	Reason       string           `json:"reason,omitempty"`
	Recipient    string           `json:"recipient,omitempty"`
	SaleID       *uuid.UUID       `json:"sale_id,omitempty"`
	ActorID      uuid.UUID        `json:"actor_id"`
	RequestID    string           `json:"request_id"`
	OccurredAt   time.Time        `json:"occurred_at"`
}

type RecordCashMovementInput struct {
	ID              uuid.UUID        `json:"id"`
	CashShiftID     uuid.UUID        `json:"cash_shift_id"`
	ExpectedVersion int64            `json:"expected_version"`
	MovementType    CashMovementType `json:"movement_type"`
	AmountCents     int64            `json:"amount_cents"`
	Reason          string           `json:"reason"`
	Recipient       string           `json:"recipient"`
}

type CloseShiftInput struct {
	CashShiftID      uuid.UUID `json:"cash_shift_id"`
	ExpectedVersion  int64     `json:"expected_version"`
	CountedCashCents int64     `json:"counted_cash_cents"`
	ClosingNote      string    `json:"closing_note"`
}

type PaymentMethod string

const (
	PaymentCash     PaymentMethod = "cash"
	PaymentTransfer PaymentMethod = "transfer"
	PaymentQR       PaymentMethod = "qr"
	PaymentCard     PaymentMethod = "card"
	PaymentOther    PaymentMethod = "other"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentReported  PaymentStatus = "reported"
	PaymentConfirmed PaymentStatus = "confirmed"
	PaymentRejected  PaymentStatus = "rejected"
)

type SaleItemInput struct {
	ProductID uuid.UUID          `json:"product_id"`
	Quantity  inventory.Quantity `json:"quantity"`
}

type PaymentInput struct {
	Method             PaymentMethod `json:"method"`
	AmountCents        int64         `json:"amount_cents"`
	Status             PaymentStatus `json:"status"`
	ReceivedCents      *int64        `json:"received_cents,omitempty"`
	ExternalReference  string        `json:"external_reference"`
	VerificationMethod string        `json:"verification_method"`
}

type CreateSaleInput struct {
	OperationID      uuid.UUID       `json:"operation_id"`
	DeviceSessionID  uuid.UUID       `json:"device_session_id"`
	FencingVersion   int64           `json:"fencing_version"`
	CashShiftID      uuid.UUID       `json:"cash_shift_id"`
	ShiftVersion     int64           `json:"shift_version"`
	CatalogVersion   int64           `json:"catalog_version"`
	Items            []SaleItemInput `json:"items"`
	Payments         []PaymentInput  `json:"payments"`
	offlineCatalogAt *time.Time
	offlineCreatedAt *time.Time
	recoveryCaseID   *uuid.UUID
}

type OfflineAuthorizationRequest struct {
	DeviceSessionID uuid.UUID `json:"device_session_id"`
	FencingVersion  int64     `json:"fencing_version"`
	CashShiftID     uuid.UUID `json:"cash_shift_id"`
	ShiftVersion    int64     `json:"shift_version"`
	CatalogVersion  int64     `json:"catalog_version"`
}

type OfflineAuthorization struct {
	Token           string    `json:"token"`
	UserID          uuid.UUID `json:"user_id"`
	DeviceID        uuid.UUID `json:"device_id"`
	DeviceSessionID uuid.UUID `json:"device_session_id"`
	FencingVersion  int64     `json:"fencing_version"`
	CashShiftID     uuid.UUID `json:"cash_shift_id"`
	ShiftVersion    int64     `json:"shift_version"`
	CatalogVersion  int64     `json:"catalog_version"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type OfflineOperation struct {
	OperationID     uuid.UUID       `json:"operation_id"`
	DeviceID        uuid.UUID       `json:"device_id"`
	DeviceSessionID uuid.UUID       `json:"device_session_id"`
	FencingVersion  int64           `json:"fencing_version"`
	Sequence        int64           `json:"sequence"`
	CreatedAtLocal  time.Time       `json:"created_at_local"`
	Timezone        string          `json:"timezone"`
	PreviousHash    string          `json:"previous_hash"`
	PayloadHash     string          `json:"payload_hash"`
	Authorization   string          `json:"authorization"`
	OperationType   string          `json:"operation_type"`
	Payload         json.RawMessage `json:"payload"`
}

type OfflineCloseShiftInput struct {
	CashShiftID      uuid.UUID `json:"cash_shift_id"`
	ExpectedVersion  int64     `json:"expected_version"`
	CountedCashCents int64     `json:"counted_cash_cents"`
	ClosingNote      string    `json:"closing_note"`
}

type OfflineOpenShiftInput struct {
	ID                  uuid.UUID `json:"id"`
	PreviousCashShiftID uuid.UUID `json:"previous_cash_shift_id"`
	DeviceSessionID     uuid.UUID `json:"device_session_id"`
	FencingVersion      int64     `json:"fencing_version"`
	OpeningCashCents    int64     `json:"opening_cash_cents"`
	OpeningNote         string    `json:"opening_note"`
}

type SyncBatchInput struct {
	Operations []OfflineOperation `json:"operations"`
}

type SyncACK struct {
	OperationID uuid.UUID  `json:"operation_id"`
	Sequence    int64      `json:"sequence"`
	PayloadHash string     `json:"payload_hash"`
	Status      string     `json:"status"`
	SaleID      *uuid.UUID `json:"sale_id,omitempty"`
	ShiftID     *uuid.UUID `json:"shift_id,omitempty"`
	Replayed    bool       `json:"replayed"`
}

type SyncBatchResult struct {
	ACKs []SyncACK `json:"acks"`
}

type SyncStatus struct {
	DeviceSessionID uuid.UUID  `json:"device_session_id"`
	LastSequence    int64      `json:"last_sequence"`
	LastHash        string     `json:"last_hash"`
	RecoveryCaseID  *uuid.UUID `json:"recovery_case_id,omitempty"`
}

type SyncReservation struct {
	ACK      *SyncACK
	Evidence bool
}

type ForceRecoveryInput struct {
	PreviousSessionID uuid.UUID `json:"previous_session_id"`
	DeviceID          uuid.UUID `json:"device_id"`
	DeviceName        string    `json:"device_name"`
	Reason            string    `json:"reason"`
	CashCountCents    int64     `json:"cash_count_cents"`
}

type RecoveryDifferenceInput struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	AmountCents *int64    `json:"amount_cents,omitempty"`
	Description string    `json:"description"`
}

type RecoveryControlsInput struct {
	JournalStatus           string `json:"journal_status"`
	GapsDocumented          bool   `json:"gaps_documented"`
	DigitalPaymentsReviewed bool   `json:"digital_payments_reviewed"`
	InventoryCounted        bool   `json:"inventory_counted"`
	OwnerApproved           bool   `json:"owner_approved"`
}

type RecoveryDifference struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	AmountCents *int64     `json:"amount_cents,omitempty"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type RecoveryDetails struct {
	RecoveryCase
	JournalStatus           string               `json:"journal_status"`
	GapsDocumented          bool                 `json:"gaps_documented"`
	DigitalPaymentsReviewed bool                 `json:"digital_payments_reviewed"`
	InventoryCounted        bool                 `json:"inventory_counted"`
	OwnerApproved           bool                 `json:"owner_approved"`
	Differences             []RecoveryDifference `json:"differences"`
}

type RecoveryReconstructionInput struct {
	ReconciliationOperationID uuid.UUID       `json:"reconciliation_operation_id"`
	SourceOperationID         *uuid.UUID      `json:"source_operation_id,omitempty"`
	Certainty                 string          `json:"certainty"`
	Evidence                  string          `json:"evidence"`
	Sale                      CreateSaleInput `json:"sale"`
}

type RecoveryCase struct {
	ID                     uuid.UUID     `json:"id"`
	PreviousSessionID      uuid.UUID     `json:"previous_session_id"`
	ReplacementSessionID   uuid.UUID     `json:"replacement_session_id"`
	AffectedShiftID        *uuid.UUID    `json:"affected_shift_id,omitempty"`
	Status                 string        `json:"status"`
	Reason                 string        `json:"reason"`
	LastACKSequence        int64         `json:"last_ack_sequence"`
	LastACKHash            string        `json:"last_ack_hash"`
	CashCountCents         int64         `json:"cash_count_cents"`
	OpenedAt               time.Time     `json:"opened_at"`
	PreviousFencingVersion int64         `json:"previous_fencing_version"`
	AffectedShiftVersion   int64         `json:"affected_shift_version"`
	ReplacementSession     DeviceSession `json:"replacement_session"`
}

type SaleItem struct {
	ID             uuid.UUID          `json:"id"`
	LineNumber     int                `json:"line_number"`
	ProductID      uuid.UUID          `json:"product_id"`
	ProductName    string             `json:"product_name"`
	SaleMode       string             `json:"sale_mode"`
	BaseUnit       string             `json:"base_unit"`
	Quantity       inventory.Quantity `json:"quantity"`
	UnitPriceCents int64              `json:"unit_price_cents"`
	PriceUnit      string             `json:"price_unit"`
	SubtotalCents  int64              `json:"subtotal_cents"`
	CatalogVersion int64              `json:"catalog_version"`
}

type SalePayment struct {
	ID                 uuid.UUID     `json:"id"`
	Method             PaymentMethod `json:"method"`
	AmountCents        int64         `json:"amount_cents"`
	Status             PaymentStatus `json:"status"`
	ReceivedCents      *int64        `json:"received_cents,omitempty"`
	ChangeCents        *int64        `json:"change_cents,omitempty"`
	ExternalReference  string        `json:"external_reference,omitempty"`
	ConfirmedBy        *uuid.UUID    `json:"confirmed_by,omitempty"`
	ConfirmedAt        *time.Time    `json:"confirmed_at,omitempty"`
	VerificationMethod string        `json:"verification_method,omitempty"`
}

type InventoryIncident struct {
	ID              uuid.UUID          `json:"id"`
	ProductID       uuid.UUID          `json:"product_id"`
	BalanceQuantity inventory.Quantity `json:"balance_quantity"`
	Status          string             `json:"status"`
}

type Sale struct {
	ID                 uuid.UUID           `json:"id"`
	CashShiftID        uuid.UUID           `json:"cash_shift_id"`
	DeviceSessionID    uuid.UUID           `json:"device_session_id"`
	FencingVersion     int64               `json:"fencing_version"`
	ActorID            uuid.UUID           `json:"actor_id"`
	Status             string              `json:"status"`
	PaymentState       string              `json:"payment_state"`
	CatalogVersion     int64               `json:"catalog_version"`
	SubtotalCents      int64               `json:"subtotal_cents"`
	TotalCents         int64               `json:"total_cents"`
	CompletedAt        time.Time           `json:"completed_at"`
	CashShiftVersion   int64               `json:"cash_shift_version"`
	Items              []SaleItem          `json:"items"`
	Payments           []SalePayment       `json:"payments"`
	InventoryIncidents []InventoryIncident `json:"inventory_incidents"`
	Replayed           bool                `json:"replayed"`
}

type SaleList struct {
	Items  []Sale `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type SaleFilter struct {
	Limit  int
	Offset int
}
