//go:build integration

package catalog

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"bakery-pos/backend/internal/database"

	"github.com/google/uuid"
)

func TestCatalogPersistsPriceHistoryAndSnapshot(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, name, username, password_hash, role) VALUES ($1, 'Owner Test', 'owner-test', 'not-used', 'owner')`, actorID); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	repository := NewPostgresRepository(pool)
	service := NewService(repository)
	category, err := service.CreateCategory(ctx, "Panificados", actorID, "request-category")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	initialPrice := int64(950000)
	product, err := service.CreateProduct(ctx, CreateProductInput{
		CategoryID:  category.ID,
		Name:        "Pan de campo",
		ProductType: ProductTypeProduced,
		SaleMode:    SaleModeWeight,
		BaseUnit:    BaseUnitGram,
		Sellable:    true,
		Barcode:     "779000000001",
		PriceCents:  &initialPrice,
	}, actorID, "request-product")
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	newPrice := int64(1025000)
	updated, err := service.UpdateProduct(ctx, product.ID, UpdateProductInput{PriceCents: &newPrice}, actorID, "request-price")
	if err != nil {
		t.Fatalf("update product: %v", err)
	}
	if updated.CurrentPriceCents == nil || *updated.CurrentPriceCents != newPrice {
		t.Fatalf("current price = %v, want %d", updated.CurrentPriceCents, newPrice)
	}
	history, err := service.PriceHistory(ctx, product.ID)
	if err != nil {
		t.Fatalf("price history: %v", err)
	}
	if len(history) != 2 || history[0].EffectiveTo != nil || history[1].EffectiveTo == nil {
		t.Fatalf("unexpected price history: %#v", history)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Version <= 1 || len(snapshot.Products) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	bulkPreview, err := service.PreviewBulkPrices(ctx, BulkPriceRequest{Percentage: "10.00"})
	if err != nil {
		t.Fatalf("preview bulk prices: %v", err)
	}
	if len(bulkPreview.Items) != 1 || bulkPreview.Items[0].NewPriceCents != 1127500 {
		t.Fatalf("bulk preview = %#v", bulkPreview)
	}
	bulkResult, err := service.CommitBulkPrices(ctx, BulkPriceCommitRequest{
		BulkPriceRequest: BulkPriceRequest{Percentage: "10.00"},
		PreviewHash:      bulkPreview.PreviewHash,
	}, actorID, "request-bulk")
	if err != nil {
		t.Fatalf("commit bulk prices: %v", err)
	}
	if bulkResult.Items[0].NewPriceCents != 1127500 {
		t.Fatalf("bulk result = %#v", bulkResult)
	}
	csvData := "name,category,product_type,sale_mode,base_unit,sellable,track_lots,barcode,price_ars\n" +
		"Harina,Insumos,ingredient,weight,g,false,false,,\n" +
		"Factura,Panificados,produced,unit,unit,true,false,779000000002,850.50\n"
	importPreview, err := service.PreviewImport(ctx, csvData)
	if err != nil {
		t.Fatalf("preview import: %v", err)
	}
	if importPreview.ErrorCount != 0 || importPreview.ValidCount != 2 {
		t.Fatalf("import preview = %#v", importPreview)
	}
	importResult, err := service.CommitImport(ctx, ImportCommitRequest{CSV: csvData, PreviewHash: importPreview.PreviewHash}, actorID, "request-import")
	if err != nil {
		t.Fatalf("commit import: %v", err)
	}
	if importResult.ImportedCount != 2 {
		t.Fatalf("import result = %#v", importResult)
	}

	otherPrice := int64(100000)
	_, err = service.CreateProduct(ctx, CreateProductInput{
		CategoryID:  category.ID,
		Name:        "Otro pan",
		ProductType: ProductTypeProduced,
		SaleMode:    SaleModeUnit,
		BaseUnit:    BaseUnitUnit,
		Sellable:    true,
		Barcode:     "779000000001",
		PriceCents:  &otherPrice,
	}, actorID, "request-conflict")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate barcode error = %v, want ErrConflict", err)
	}
}
