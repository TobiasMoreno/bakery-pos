//go:build integration

package pos

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"testing"
	"time"

	"bakery-pos/backend/internal/database"
	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
)

var testDatabaseURL = flag.String("test-database-url", "", "PostgreSQL URL used by integration tests; TEST_DATABASE_URL remains supported")

func TestSaleCommitsCashInventoryIncidentAuditAndIdempotency(t *testing.T) {
	databaseURL := *testDatabaseURL
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
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
	grant, err := service.IssueOfflineAuthorization(ctx, OfflineAuthorizationRequest{
		DeviceSessionID: device.ID, FencingVersion: device.FencingVersion, CashShiftID: shift.ID,
		ShiftVersion: mixedSale.CashShiftVersion, CatalogVersion: catalogVersion,
	}, actorID)
	if err != nil {
		t.Fatalf("issue offline authorization: %v", err)
	}
	offlineReceived := int64(95000)
	offlinePayload := CreateSaleInput{
		OperationID: uuid.New(), DeviceSessionID: device.ID, FencingVersion: device.FencingVersion,
		CashShiftID: shift.ID, ShiftVersion: mixedSale.CashShiftVersion, CatalogVersion: catalogVersion,
		Items:    []SaleItemInput{{ProductID: productID, Quantity: mixedQuantity}},
		Payments: []PaymentInput{{Method: PaymentCash, AmountCents: 95000, Status: PaymentConfirmed, ReceivedCents: &offlineReceived}},
	}
	offlineHash, err := canonicalPayloadHash(offlinePayload)
	if err != nil {
		t.Fatal(err)
	}
	offlinePayloadJSON, err := json.Marshal(offlinePayload)
	if err != nil {
		t.Fatal(err)
	}
	offlineOperation := OfflineOperation{
		OperationID: offlinePayload.OperationID, OperationType: "sale", DeviceID: device.DeviceID, DeviceSessionID: device.ID,
		FencingVersion: device.FencingVersion, Sequence: 1, CreatedAtLocal: grant.IssuedAt.Add(time.Second),
		Timezone: "America/Argentina/Buenos_Aires", PayloadHash: offlineHash, Authorization: grant.Token, Payload: offlinePayloadJSON,
	}
	firstSync, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{offlineOperation}}, "request-sync")
	if err != nil || len(firstSync.ACKs) != 1 || firstSync.ACKs[0].SaleID == nil {
		t.Fatalf("first sync = %#v, error = %v", firstSync, err)
	}
	replayedSync, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{offlineOperation}}, "request-sync-retry")
	if err != nil || len(replayedSync.ACKs) != 1 || !replayedSync.ACKs[0].Replayed || replayedSync.ACKs[0].SaleID == nil || *replayedSync.ACKs[0].SaleID != *firstSync.ACKs[0].SaleID {
		t.Fatalf("replayed sync = %#v, error = %v", replayedSync, err)
	}

	current, err := service.CurrentShift(ctx)
	if err != nil {
		t.Fatalf("current shift: %v", err)
	}
	if current == nil || current.ExpectedCashCents != 3477500 || current.Version != 4 {
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
	if currentValue.ExpectedCashCents != 2877500 || currentValue.Version != 6 {
		t.Fatalf("cash formula result = %#v", currentValue)
	}
	closePayload := OfflineCloseShiftInput{CashShiftID: shift.ID, ExpectedVersion: currentValue.Version, CountedCashCents: 2850000}
	closeJSON, _ := json.Marshal(closePayload)
	closeHash, _ := canonicalRawPayloadHash(closeJSON)
	closeOperation := OfflineOperation{
		OperationID: uuid.New(), OperationType: "shift_close", DeviceID: device.DeviceID, DeviceSessionID: device.ID,
		FencingVersion: device.FencingVersion, Sequence: 2, CreatedAtLocal: grant.IssuedAt.Add(2 * time.Second),
		Timezone: "America/Argentina/Buenos_Aires", PreviousHash: offlineHash, PayloadHash: closeHash, Authorization: grant.Token, Payload: closeJSON,
	}
	closeResult, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{closeOperation}}, "request-offline-close")
	if err != nil || len(closeResult.ACKs) != 1 || closeResult.ACKs[0].ShiftID == nil {
		t.Fatalf("offline close sync = %#v, error = %v", closeResult, err)
	}
	closed, err := service.CurrentShift(ctx)
	if err != nil || closed != nil {
		t.Fatalf("current shift after offline close = %#v, error = %v", closed, err)
	}
	closedShift, err := getShift(ctx, pool, shift.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if closedShift.ExpectedCashCents != 2877500 || closedShift.DifferenceCents == nil || *closedShift.DifferenceCents != -27500 {
		t.Fatalf("unexpected closed shift: %#v", closedShift)
	}
	newShiftID := uuid.New()
	openPayload := OfflineOpenShiftInput{ID: newShiftID, PreviousCashShiftID: shift.ID, DeviceSessionID: device.ID, FencingVersion: device.FencingVersion, OpeningCashCents: 2850000}
	openJSON, _ := json.Marshal(openPayload)
	openHash, _ := canonicalRawPayloadHash(openJSON)
	openOperation := OfflineOperation{
		OperationID: uuid.New(), OperationType: "shift_open", DeviceID: device.DeviceID, DeviceSessionID: device.ID,
		FencingVersion: device.FencingVersion, Sequence: 3, CreatedAtLocal: grant.IssuedAt.Add(3 * time.Second),
		Timezone: "America/Argentina/Buenos_Aires", PreviousHash: closeHash, PayloadHash: openHash, Authorization: grant.Token, Payload: openJSON,
	}
	openResult, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{openOperation}}, "request-offline-open")
	if err != nil || len(openResult.ACKs) != 1 || openResult.ACKs[0].ShiftID == nil || *openResult.ACKs[0].ShiftID != newShiftID {
		t.Fatalf("offline open sync = %#v, error = %v", openResult, err)
	}
	replayedClose, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{closeOperation}}, "request-offline-close-replay")
	if err != nil || !replayedClose.ACKs[0].Replayed {
		t.Fatalf("offline close replay = %#v, error = %v", replayedClose, err)
	}
	recovery, err := service.ForceRecovery(ctx, ForceRecoveryInput{
		PreviousSessionID: device.ID, DeviceID: uuid.New(), DeviceName: "Mostrador reemplazo",
		Reason: "Dispositivo anterior inutilizable", CashCountCents: 2850000,
	}, actorID, "request-force-recovery")
	if err != nil || recovery.AffectedShiftID == nil || recovery.AffectedShiftVersion != 2 {
		t.Fatalf("force recovery = %#v, error = %v", recovery, err)
	}
	differenceID := uuid.New()
	details, err := service.AddRecoveryDifference(ctx, recovery.ID, RecoveryDifferenceInput{
		ID: differenceID, Kind: "sequence_gap", Description: "Se documentó el tramo posterior al último ACK.",
	}, actorID, "request-recovery-difference")
	if err != nil || len(details.Differences) != 1 {
		t.Fatalf("recovery differences = %#v, error = %v", details, err)
	}
	_, err = service.UpdateRecoveryControls(ctx, recovery.ID, RecoveryControlsInput{
		JournalStatus: "imported", GapsDocumented: true, DigitalPaymentsReviewed: true, InventoryCounted: true, OwnerApproved: true,
	}, actorID, "request-recovery-controls-before-resolution")
	if err != nil {
		t.Fatalf("prepare recovery controls: %v", err)
	}
	if _, err = service.CloseRecovery(ctx, recovery.ID, actorID, "request-premature-recovery-close"); !errors.Is(err, ErrConflict) {
		t.Fatalf("close recovery with open difference error = %v, want ErrConflict", err)
	}
	details, err = service.ResolveRecoveryDifference(ctx, recovery.ID, differenceID, actorID, "request-resolve-recovery-difference")
	if err != nil || details.Differences[0].ResolvedAt == nil {
		t.Fatalf("resolved recovery difference = %#v, error = %v", details, err)
	}
	sourceOperationID := uuid.New()
	reconstructionID := uuid.New()
	reconstructionInput := RecoveryReconstructionInput{
		ReconciliationOperationID: reconstructionID, SourceOperationID: &sourceOperationID, Certainty: "exact", Evidence: "Comprobante físico conservado",
		Sale: CreateSaleInput{
			DeviceSessionID: device.ID, FencingVersion: device.FencingVersion, CashShiftID: newShiftID,
			ShiftVersion: recovery.AffectedShiftVersion, CatalogVersion: catalogVersion,
			Items:    []SaleItemInput{{ProductID: productID, Quantity: mixedQuantity}},
			Payments: []PaymentInput{{Method: PaymentCash, AmountCents: 95000, Status: PaymentConfirmed, ReceivedCents: int64Pointer(95000)}},
		},
	}
	type reconstructionResult struct {
		sale Sale
		err  error
	}
	concurrentResults := make(chan reconstructionResult, 2)
	startReconstruction := make(chan struct{})
	for _, concurrentRequestID := range []string{"request-reconstruct-sale-a", "request-reconstruct-sale-b"} {
		go func(requestID string) {
			<-startReconstruction
			sale, err := service.ReconstructRecoverySale(ctx, recovery.ID, reconstructionInput, actorID, requestID)
			concurrentResults <- reconstructionResult{sale: sale, err: err}
		}(concurrentRequestID)
	}
	close(startReconstruction)
	var reconstructed Sale
	for range 2 {
		result := <-concurrentResults
		if result.err == nil {
			if result.sale.ID != reconstructionID {
				t.Fatalf("concurrent reconstructed sale = %#v", result.sale)
			}
			reconstructed = result.sale
		} else if !errors.Is(result.err, ErrConflict) {
			t.Fatalf("concurrent reconstruction error = %v, want nil or ErrConflict", result.err)
		}
	}
	if reconstructed.ID != reconstructionID {
		t.Fatal("expected one concurrent reconstruction to succeed")
	}
	replayedReconstruction, err := service.ReconstructRecoverySale(ctx, recovery.ID, reconstructionInput, actorID, "request-reconstruct-sale-retry")
	if err != nil || !replayedReconstruction.Replayed || replayedReconstruction.ID != reconstructed.ID {
		t.Fatalf("replayed reconstruction = %#v, error = %v", replayedReconstruction, err)
	}
	failedReconstructionID := uuid.New()
	failedSourceID := uuid.New()
	failedReconstruction := reconstructionInput
	failedReconstruction.ReconciliationOperationID = failedReconstructionID
	failedReconstruction.SourceOperationID = &failedSourceID
	failedReconstruction.Sale.ShiftVersion = reconstructed.CashShiftVersion
	failedReconstruction.Sale.Payments = []PaymentInput{{Method: PaymentCash, AmountCents: 94000, Status: PaymentConfirmed, ReceivedCents: int64Pointer(94000)}}
	if _, err := service.ReconstructRecoverySale(ctx, recovery.ID, failedReconstruction, actorID, "request-failed-reconstruction"); !errors.Is(err, ErrPaymentIncomplete) {
		t.Fatalf("failed reconstruction error = %v, want ErrPaymentIncomplete", err)
	}
	for table, id := range map[string]uuid.UUID{"sales": failedReconstructionID, "recovery_reconstructions": failedReconstructionID} {
		column := "id"
		if table == "recovery_reconstructions" {
			column = "reconciliation_operation_id"
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE `+column+`=$1`, id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("atomic reconstruction rollback %s count = %d, error = %v", table, count, err)
		}
	}
	evidencePayload := CreateSaleInput{
		OperationID: sourceOperationID, DeviceSessionID: device.ID, FencingVersion: device.FencingVersion,
		CashShiftID: newShiftID, ShiftVersion: recovery.AffectedShiftVersion + 1, CatalogVersion: catalogVersion,
		Items:    []SaleItemInput{{ProductID: productID, Quantity: mixedQuantity}},
		Payments: []PaymentInput{{Method: PaymentCash, AmountCents: 95000, Status: PaymentConfirmed, ReceivedCents: int64Pointer(95000)}},
	}
	evidenceJSON, _ := json.Marshal(evidencePayload)
	evidenceHash, _ := canonicalRawPayloadHash(evidenceJSON)
	evidenceOperation := OfflineOperation{
		OperationID: sourceOperationID, OperationType: "sale", DeviceID: device.DeviceID, DeviceSessionID: device.ID,
		FencingVersion: device.FencingVersion, Sequence: 4, CreatedAtLocal: grant.IssuedAt.Add(4 * time.Second),
		Timezone: "America/Argentina/Buenos_Aires", PreviousHash: openHash, PayloadHash: evidenceHash, Authorization: grant.Token, Payload: evidenceJSON,
	}
	evidenceResult, err := service.SyncOperations(ctx, SyncBatchInput{Operations: []OfflineOperation{evidenceOperation}}, "request-recovery-evidence")
	if err != nil || len(evidenceResult.ACKs) != 1 || evidenceResult.ACKs[0].SaleID == nil || *evidenceResult.ACKs[0].SaleID != reconstructed.ID {
		t.Fatalf("matched recovery evidence = %#v, error = %v", evidenceResult, err)
	}
	details, err = service.UpdateRecoveryControls(ctx, recovery.ID, RecoveryControlsInput{
		JournalStatus: "imported", GapsDocumented: true, DigitalPaymentsReviewed: true, InventoryCounted: true, OwnerApproved: true,
	}, actorID, "request-recovery-controls")
	if err != nil || !details.OwnerApproved {
		t.Fatalf("recovery controls = %#v, error = %v", details, err)
	}
	details, err = service.CloseRecovery(ctx, recovery.ID, actorID, "request-recovery-close")
	if err != nil || details.Status != "resolved" {
		t.Fatalf("close recovery = %#v, error = %v", details, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sales WHERE id = $1`, sale.ID); err == nil {
		t.Fatal("expected sale deletion to be rejected")
	}
	for table, want := range map[string]int{"sales": 4, "sale_items": 4, "sale_payments": 5, "cash_movements": 6, "inventory_movements": 4, "inventory_incidents": 4, "sync_operations": 4, "recovery_differences": 1, "recovery_reconstructions": 1} {
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}
