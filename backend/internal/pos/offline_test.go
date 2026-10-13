package pos

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

func TestOfflineOperationRejectsChangedPayload(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, "test-signing-key-with-at-least-32-characters")
	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	op := validOfflineOperation(t, service, now)
	var payload CreateSaleInput
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Items[0].Quantity = mustQuantity(t, "2")
	op.Payload, _ = json.Marshal(payload)

	if _, err := service.validateOfflineOperation(op); !errors.Is(err, ErrSyncConflict) {
		t.Fatalf("validateOfflineOperation() error = %v, want ErrSyncConflict", err)
	}
}

func TestOfflineOperationRejectsConfirmedDigitalPayment(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, "test-signing-key-with-at-least-32-characters")
	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	op := validOfflineOperation(t, service, now)
	var payload CreateSaleInput
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Payments[0] = PaymentInput{Method: PaymentTransfer, AmountCents: 100, Status: PaymentConfirmed, VerificationMethod: "manual"}
	op.Payload, _ = json.Marshal(payload)
	hash, err := canonicalRawPayloadHash(op.Payload)
	if err != nil {
		t.Fatal(err)
	}
	op.PayloadHash = hash

	if _, err := service.validateOfflineOperation(op); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validateOfflineOperation() error = %v, want ErrInvalid", err)
	}
}

func TestOfflineGrantAllowsSyncAfterExpiryWhenOperationWasCreatedInTime(t *testing.T) {
	issued := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, "test-signing-key-with-at-least-32-characters")
	service.now = func() time.Time { return issued.Add(48 * time.Hour) }
	op := validOfflineOperation(t, service, issued)

	if _, err := service.validateOfflineOperation(op); err != nil {
		t.Fatalf("validateOfflineOperation() error = %v", err)
	}
}

func validOfflineOperation(t *testing.T, service *Service, issued time.Time) OfflineOperation {
	t.Helper()
	userID, deviceID, sessionID, shiftID, operationID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	grant := OfflineAuthorization{UserID: userID, DeviceID: deviceID, DeviceSessionID: sessionID, FencingVersion: 3, CashShiftID: shiftID, ShiftVersion: 1, CatalogVersion: 5, IssuedAt: issued, ExpiresAt: issued.Add(24 * time.Hour)}
	body, err := json.Marshal(grant)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, service.signingKey)
	_, _ = mac.Write([]byte(encoded))
	token := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	payload := CreateSaleInput{OperationID: operationID, DeviceSessionID: sessionID, FencingVersion: 3, CashShiftID: shiftID, ShiftVersion: 1, CatalogVersion: 5, Items: []SaleItemInput{{ProductID: uuid.New(), Quantity: mustQuantity(t, "1")}}, Payments: []PaymentInput{{Method: PaymentCash, AmountCents: 100, Status: PaymentConfirmed, ReceivedCents: int64Pointer(100)}}}
	hash, err := canonicalPayloadHash(payload)
	if err != nil {
		t.Fatal(err)
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return OfflineOperation{OperationID: operationID, OperationType: "sale", DeviceID: deviceID, DeviceSessionID: sessionID, FencingVersion: 3, Sequence: 1, CreatedAtLocal: issued.Add(time.Hour), Timezone: "America/Argentina/Buenos_Aires", PayloadHash: hash, Authorization: token, Payload: rawPayload}
}

func int64Pointer(value int64) *int64 { return &value }

func mustQuantity(t *testing.T, value string) inventory.Quantity {
	t.Helper()
	quantity, err := inventory.ParseQuantity(value)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}
