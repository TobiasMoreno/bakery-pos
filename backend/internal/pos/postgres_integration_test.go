//go:build integration

package pos

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"bakery-pos/backend/internal/database"
	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

func TestSaleCommitsCashInventoryIncidentAuditAndIdempotency(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `TRUNCATE audit_events, sessions, inventory_incidents, cash_movements,
		sale_payments, sale_items, sales, cash_shifts, pos_device_sessions, inventory_movements,
		inventory_balances, price_change_items, price_change_batches, product_prices, product_barcodes,
		products, categories, users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	actorID := uuid.New()
	categoryID := uuid.New()
	productID := uuid.New()
	priceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, name, username, password_hash, role) VALUES ($1, 'Owner Test', 'pos-owner', 'unused', 'owner')`, actorID); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO categories (id, name) VALUES ($1, 'Panificados')`, categoryID); err != nil {
		t.Fatalf("insert category: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO products (id, category_id, name, product_type, sale_mode, base_unit, sellable, track_lots)
		VALUES ($1, $2, 'Pan de campo', 'produced', 'weight', 'g', TRUE, FALSE)`, productID, categoryID); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO inventory_balances (product_id, quantity) VALUES ($1, 0)`, productID); err != nil {
		t.Fatalf("insert balance: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_prices (id, product_id, amount_cents, price_unit, effective_from, actor_id)
		VALUES ($1, $2, 950000, 'kg', now(), $3)`, priceID, productID, actorID); err != nil {
		t.Fatalf("insert price: %v", err)
	}
	var catalogVersion int64
	if err := pool.QueryRow(ctx, `SELECT version FROM catalog_state WHERE singleton = TRUE`).Scan(&catalogVersion); err != nil {
		t.Fatalf("catalog version: %v", err)
	}

	service := NewService(NewPostgresRepository(pool))
	device, err := service.AcquireDevice(ctx, AcquireDeviceInput{DeviceID: uuid.New(), DeviceName: "Tablet test"}, actorID, "request-device")
	if err != nil {
		t.Fatalf("acquire device: %v", err)
	}
	if _, err := service.AcquireDevice(ctx, AcquireDeviceInput{DeviceID: uuid.New(), DeviceName: "Segundo POS"}, actorID, "request-second-device"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second device error = %v, want ErrConflict", err)
	}
	shift, err := service.OpenShift(ctx, OpenShiftInput{
		DeviceSessionID: device.ID, FencingVersion: device.FencingVersion, OpeningCashCents: 3000000,
	}, actorID, "request-shift")
	if err != nil {
		t.Fatalf("open shift: %v", err)
	}
	quantity, err := inventory.ParseQuantity("350")
	if err != nil {
		t.Fatal(err)
	}
	received := int64(400000)
	input := CreateSaleInput{
		OperationID: uuid.New(), DeviceSessionID: device.ID, FencingVersion: device.FencingVersion,
		CashShiftID: shift.ID, ShiftVersion: shift.Version, CatalogVersion: catalogVersion,
		Items:    []SaleItemInput{{ProductID: productID, Quantity: quantity}},
		Payments: []PaymentInput{{Method: PaymentCash, AmountCents: 332500, Status: PaymentConfirmed, ReceivedCents: &received}},
	}
	sale, err := service.CreateSale(ctx, input, actorID, "request-sale")
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if sale.TotalCents != 332500 || sale.PaymentState != "settled" || sale.CashShiftVersion != 2 {
		t.Fatalf("unexpected sale: %#v", sale)
	}
	if len(sale.InventoryIncidents) != 1 || sale.InventoryIncidents[0].BalanceQuantity.String() != "-350" {
		t.Fatalf("unexpected stock incidents: %#v", sale.InventoryIncidents)
	}
	if sale.Payments[0].ChangeCents == nil || *sale.Payments[0].ChangeCents != 67500 {
		t.Fatalf("unexpected cash change: %#v", sale.Payments[0])
	}

	replayed, err := service.CreateSale(ctx, input, actorID, "request-sale-retry")
	if err != nil {
		t.Fatalf("replay sale: %v", err)
	}
	if !replayed.Replayed || replayed.ID != sale.ID {
		t.Fatalf("unexpected replay: %#v", replayed)
	}
	input.Payments[0].AmountCents++
	if _, err := service.CreateSale(ctx, input, actorID, "request-sale-conflict"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed replay error = %v, want ErrConflict", err)
	}
	mixedQuantity, err := inventory.ParseQuantity("100")
	if err != nil {
		t.Fatal(err)
	}
	mixedReceived := int64(60000)
	mixedSale, err := service.CreateSale(ctx, CreateSaleInput{
		OperationID: uuid.New(), DeviceSessionID: device.ID, FencingVersion: device.FencingVersion,
		CashShiftID: shift.ID, ShiftVersion: sale.CashShiftVersion, CatalogVersion: catalogVersion,
		Items: []SaleItemInput{{ProductID: productID, Quantity: mixedQuantity}},
		Payments: []PaymentInput{
			{Method: PaymentCash, AmountCents: 50000, Status: PaymentConfirmed, ReceivedCents: &mixedReceived},
			{Method: PaymentTransfer, AmountCents: 45000, Status: PaymentReported, ExternalReference: "transfer-test"},
		},
	}, actorID, "request-mixed-sale")
	if err != nil {
		t.Fatalf("create mixed sale: %v", err)
	}
	if mixedSale.PaymentState != "unverified" || mixedSale.Payments[0].ChangeCents == nil || *mixedSale.Payments[0].ChangeCents != 10000 {
		t.Fatalf("unexpected mixed payment result: %#v", mixedSale)
	}

	current, err := service.CurrentShift(ctx)
	if err != nil {
		t.Fatalf("current shift: %v", err)
	}
	if current == nil || current.ExpectedCashCents != 3382500 || current.Version != 3 {
		t.Fatalf("unexpected current shift: %#v", current)
	}
	currentValue := *current
	currentValue, err = service.RecordCashMovement(ctx, RecordCashMovementInput{
		ID: uuid.New(), CashShiftID: shift.ID, ExpectedVersion: currentValue.Version,
		MovementType: CashMovementExpense, AmountCents: 200000, Reason: "Compra urgente",
	}, actorID, "request-expense")
	if err != nil {
		t.Fatalf("record expense: %v", err)
	}
	currentValue, err = service.RecordCashMovement(ctx, RecordCashMovementInput{
		ID: uuid.New(), CashShiftID: shift.ID, ExpectedVersion: currentValue.Version,
		MovementType: CashMovementWithdrawal, AmountCents: 400000, Reason: "Retiro parcial", Recipient: "Dueño",
	}, actorID, "request-withdrawal")
	if err != nil {
		t.Fatalf("record withdrawal: %v", err)
	}
	if currentValue.ExpectedCashCents != 2782500 || currentValue.Version != 5 {
		t.Fatalf("cash formula result = %#v", currentValue)
	}
	closed, err := service.CloseShift(ctx, CloseShiftInput{
		CashShiftID: shift.ID, ExpectedVersion: currentValue.Version, CountedCashCents: 2750000,
	}, actorID, "request-close")
	if err != nil {
		t.Fatalf("close shift: %v", err)
	}
	if closed.ExpectedCashCents != 2782500 || closed.DifferenceCents == nil || *closed.DifferenceCents != -32500 {
		t.Fatalf("unexpected closed shift: %#v", closed)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sales WHERE id = $1`, sale.ID); err == nil {
		t.Fatal("expected sale deletion to be rejected")
	}
	for table, want := range map[string]int{"sales": 2, "sale_items": 2, "sale_payments": 3, "cash_movements": 4, "inventory_movements": 2, "inventory_incidents": 2} {
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}
