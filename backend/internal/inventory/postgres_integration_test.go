//go:build integration

package inventory

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"bakery-pos/backend/internal/database"

	"github.com/google/uuid"
)

func TestInventoryLedgerIsExactIdempotentAndImmutable(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE audit_events, sessions, inventory_movements, inventory_balances, price_change_items, price_change_batches, product_prices, product_barcodes, products, categories, users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	actorID := uuid.New()
	categoryID := uuid.New()
	unitProductID := uuid.New()
	weightProductID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, name, username, password_hash, role) VALUES ($1, 'Owner Test', 'inventory-owner', 'not-used', 'owner')`, actorID); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO categories (id, name) VALUES ($1, 'Inventario')`, categoryID); err != nil {
		t.Fatalf("insert category: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO products (id, category_id, name, product_type, sale_mode, base_unit, sellable)
		VALUES ($1, $3, 'Gaseosa', 'resale', 'unit', 'unit', false),
		       ($2, $3, 'Harina', 'ingredient', 'weight', 'g', false)`, unitProductID, weightProductID, categoryID); err != nil {
		t.Fatalf("insert products: %v", err)
	}

	service := NewService(NewPostgresRepository(pool))
	threeUnits, _ := ParseQuantity("3")
	movementID := uuid.New()
	input := RecordMovementInput{
		ID:            movementID,
		ProductID:     unitProductID,
		QuantityDelta: threeUnits,
		MovementType:  MovementTypePurchase,
		ReferenceType: "purchase",
		ReferenceID:   uuidPointer(uuid.New()),
	}
	created, err := service.RecordMovement(ctx, input, actorID, "request-create")
	if err != nil {
		t.Fatalf("record movement: %v", err)
	}
	if created.Replayed || created.Balance.Quantity.String() != "3" {
		t.Fatalf("created result = %#v", created)
	}

	replayed, err := service.RecordMovement(ctx, input, actorID, "request-retry")
	if err != nil {
		t.Fatalf("replay movement: %v", err)
	}
	if !replayed.Replayed || replayed.Balance.Quantity.String() != "3" {
		t.Fatalf("replayed result = %#v", replayed)
	}

	differentQuantity, _ := ParseQuantity("4")
	input.QuantityDelta = differentQuantity
	if _, err := service.RecordMovement(ctx, input, actorID, "request-conflict"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrConflict", err)
	}

	fractionalUnit, _ := ParseQuantity("0.5")
	input.ID = uuid.New()
	input.QuantityDelta = fractionalUnit
	if _, err := service.RecordMovement(ctx, input, actorID, "request-fraction"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("fractional unit error = %v, want ErrInvalid", err)
	}

	weightedIn, _ := ParseQuantity("350.125")
	weightedOut, _ := ParseQuantity("-50.125")
	for index, quantity := range []Quantity{weightedIn, weightedOut} {
		_, err := service.RecordMovement(ctx, RecordMovementInput{
			ID:            uuid.New(),
			ProductID:     weightProductID,
			QuantityDelta: quantity,
			MovementType:  MovementTypeProduction,
			ReferenceType: "production_batch",
			ReferenceID:   uuidPointer(uuid.New()),
		}, actorID, "request-weight-"+string(rune('0'+index)))
		if err != nil {
			t.Fatalf("record weighted movement %d: %v", index, err)
		}
	}
	reconstructed, err := service.ReconstructBalance(ctx, weightProductID)
	if err != nil {
		t.Fatalf("reconstruct balance: %v", err)
	}
	if reconstructed.String() != "300" {
		t.Fatalf("reconstructed = %s, want 300", reconstructed.String())
	}
	balances, err := service.ListBalances(ctx, BalanceFilter{Limit: 200})
	if err != nil {
		t.Fatalf("list balances: %v", err)
	}
	if balances.Total != 2 {
		t.Fatalf("balance total = %d, want 2", balances.Total)
	}
	movements, err := service.ListMovements(ctx, MovementFilter{ProductID: &weightProductID})
	if err != nil {
		t.Fatalf("list movements: %v", err)
	}
	if movements.Total != 2 {
		t.Fatalf("movement total = %d, want 2", movements.Total)
	}

	if _, err := pool.Exec(ctx, `UPDATE inventory_movements SET reason = 'mutated' WHERE id = $1`, movementID); err == nil {
		t.Fatal("updating an inventory movement should fail")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM inventory_movements WHERE id = $1`, movementID); err == nil {
		t.Fatal("deleting an inventory movement should fail")
	}
	if _, err := pool.Exec(ctx, `UPDATE inventory_balances SET quantity = 100 WHERE product_id = $1`, unitProductID); err == nil {
		t.Fatal("updating an inventory balance without a movement should fail")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO inventory_movements (
			id, product_id, quantity_delta, movement_type, reference_type,
			actor_id, request_id, occurred_at
		) VALUES ($1, $2, 0.5, 'purchase', 'constraint_test', $3, 'request-db-fraction', now())`,
		uuid.New(), unitProductID, actorID); err == nil {
		t.Fatal("database should reject fractional movements for unit products")
	}

	var movementCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM inventory_movements WHERE id = $1`, movementID).Scan(&movementCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE entity_type = 'inventory_movement' AND entity_id = $1`, movementID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if movementCount != 1 || auditCount != 1 {
		t.Fatalf("movement count = %d, audit count = %d", movementCount, auditCount)
	}
}
