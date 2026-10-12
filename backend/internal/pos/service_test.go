package pos

import (
	"errors"
	"testing"

	"bakery-pos/backend/internal/inventory"
)

func TestCalculateSubtotalWeightUsesHalfUpRounding(t *testing.T) {
	quantity, err := inventory.ParseQuantity("350")
	if err != nil {
		t.Fatal(err)
	}
	subtotal, err := CalculateSubtotal(950000, "weight", quantity)
	if err != nil {
		t.Fatal(err)
	}
	if subtotal != 332500 {
		t.Fatalf("expected 332500 cents, got %d", subtotal)
	}
}

func TestCalculateSubtotalRoundsFractionalCentHalfUp(t *testing.T) {
	quantity, err := inventory.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	subtotal, err := CalculateSubtotal(500, "weight", quantity)
	if err != nil {
		t.Fatal(err)
	}
	if subtotal != 1 {
		t.Fatalf("expected half cent to round to 1 cent, got %d", subtotal)
	}
}

func TestCalculateSubtotalRejectsFractionalUnit(t *testing.T) {
	quantity, err := inventory.ParseQuantity("1.5")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CalculateSubtotal(100, "unit", quantity); err == nil {
		t.Fatal("expected fractional unit to be rejected")
	}
}

func TestValidatePaymentRequiresEvidenceForConfirmedDigital(t *testing.T) {
	err := validatePayment(PaymentInput{Method: PaymentTransfer, AmountCents: 100, Status: PaymentConfirmed})
	if err == nil {
		t.Fatal("expected confirmed digital payment without evidence to fail")
	}
}

func TestValidatePaymentAllowsReportedDigital(t *testing.T) {
	err := validatePayment(PaymentInput{Method: PaymentQR, AmountCents: 100, Status: PaymentReported})
	if err != nil {
		t.Fatalf("expected reported payment to be valid: %v", err)
	}
}

func TestValidatePaymentRejectsPendingPaymentForCompletedSale(t *testing.T) {
	err := validatePayment(PaymentInput{Method: PaymentTransfer, AmountCents: 100, Status: PaymentPending})
	if !errors.Is(err, ErrPaymentIncomplete) {
		t.Fatalf("error = %v, want ErrPaymentIncomplete", err)
	}
}
