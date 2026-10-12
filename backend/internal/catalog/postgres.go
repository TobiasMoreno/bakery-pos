package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, now: time.Now}
}

func (r *PostgresRepository) ListCategories(ctx context.Context) ([]Category, error) {
	const query = `
		SELECT id, name, active, created_at, updated_at
		FROM categories
		ORDER BY active DESC, lower(name), id`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]Category, 0)
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name, &category.Active, &category.CreatedAt, &category.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return categories, nil
}

func (r *PostgresRepository) CreateCategory(ctx context.Context, name string, actorID uuid.UUID, requestID string) (Category, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Category{}, fmt.Errorf("begin create category: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := r.now().UTC()
	category := Category{ID: uuid.New(), Name: name, Active: true, CreatedAt: now, UpdatedAt: now}
	const insert = `
		INSERT INTO categories (id, name, active, created_at, updated_at)
		VALUES ($1, $2, TRUE, $3, $3)`
	if _, err := tx.Exec(ctx, insert, category.ID, category.Name, now); err != nil {
		return Category{}, mapWriteError("insert category", err)
	}
	if err := bumpCatalogVersion(ctx, tx, now); err != nil {
		return Category{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "catalog.category_created", "category", category.ID, requestID, map[string]any{"name": name}); err != nil {
		return Category{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Category{}, fmt.Errorf("commit create category: %w", err)
	}
	return category, nil
}

func (r *PostgresRepository) ListProducts(ctx context.Context, filter ProductFilter) ([]Product, int, error) {
	const countQuery = `
		SELECT count(*)
		FROM products p
		LEFT JOIN product_barcodes b ON b.product_id = p.id
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%' OR b.code = $1)
		  AND ($2::boolean IS NULL OR p.active = $2)`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, filter.Query, filter.Active).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	const listQuery = productSelect + `
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%' OR b.code = $1)
		  AND ($2::boolean IS NULL OR p.active = $2)
		ORDER BY p.active DESC, lower(p.name), p.id
		LIMIT $3 OFFSET $4`
	products, err := queryProducts(ctx, r.pool, listQuery, filter.Query, filter.Active, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (r *PostgresRepository) GetProduct(ctx context.Context, id uuid.UUID) (Product, error) {
	const query = productSelect + ` WHERE p.id = $1`
	product, err := scanProduct(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("get product: %w", err)
	}
	return product, nil
}

func (r *PostgresRepository) CreateProduct(ctx context.Context, input CreateProductInput, actorID uuid.UUID, requestID string) (Product, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin create product: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireActiveCategory(ctx, tx, input.CategoryID); err != nil {
		return Product{}, err
	}
	now := r.now().UTC()
	productID := uuid.New()
	const insertProduct = `
		INSERT INTO products (
			id, category_id, name, product_type, sale_mode, base_unit,
			sellable, track_lots, active, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9, $9)`
	if _, err := tx.Exec(
		ctx,
		insertProduct,
		productID,
		input.CategoryID,
		input.Name,
		input.ProductType,
		input.SaleMode,
		input.BaseUnit,
		input.Sellable,
		input.TrackLots,
		now,
	); err != nil {
		return Product{}, mapWriteError("insert product", err)
	}
	if input.Barcode != "" {
		if err := replaceBarcode(ctx, tx, productID, input.Barcode, now); err != nil {
			return Product{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO inventory_balances (product_id, quantity, updated_at) VALUES ($1, 0, $2)`, productID, now); err != nil {
		return Product{}, fmt.Errorf("create inventory balance: %w", err)
	}
	if input.PriceCents != nil {
		if err := createPrice(ctx, tx, productID, nil, *input.PriceCents, priceUnitFor(input.SaleMode), actorID, requestID, "manual", now); err != nil {
			return Product{}, err
		}
	}
	if err := bumpCatalogVersion(ctx, tx, now); err != nil {
		return Product{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "catalog.product_created", "product", productID, requestID, input); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, fmt.Errorf("commit create product: %w", err)
	}
	return r.GetProduct(ctx, productID)
}

func (r *PostgresRepository) UpdateProduct(ctx context.Context, id uuid.UUID, patch UpdateProductInput, actorID uuid.UUID, requestID string) (Product, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin update product: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockedQuery = productSelect + ` WHERE p.id = $1 FOR UPDATE OF p`
	current, err := scanProduct(tx.QueryRow(ctx, lockedQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("lock product: %w", err)
	}

	input := mergeProduct(current, patch)
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	if err := requireActiveCategory(ctx, tx, input.CategoryID); err != nil {
		return Product{}, err
	}
	active := current.Active
	if patch.Active != nil {
		active = *patch.Active
	}
	now := r.now().UTC()
	const updateProduct = `
		UPDATE products
		SET category_id = $2, name = $3, product_type = $4, sale_mode = $5,
		    base_unit = $6, sellable = $7, track_lots = $8, active = $9,
		    updated_at = $10
		WHERE id = $1`
	if _, err := tx.Exec(
		ctx,
		updateProduct,
		id,
		input.CategoryID,
		input.Name,
		input.ProductType,
		input.SaleMode,
		input.BaseUnit,
		input.Sellable,
		input.TrackLots,
		active,
		now,
	); err != nil {
		return Product{}, mapWriteError("update product", err)
	}
	if patch.Barcode != nil && !sameStringPointer(current.Barcode, inputBarcodePointer(input.Barcode)) {
		if err := replaceBarcode(ctx, tx, id, input.Barcode, now); err != nil {
			return Product{}, err
		}
	}

	newPrice := input.PriceCents
	newPriceUnit := priceUnitFor(input.SaleMode)
	priceChanged := !sameInt64Pointer(current.CurrentPriceCents, newPrice)
	if !priceChanged && newPrice != nil && current.PriceUnit != nil {
		priceChanged = *current.PriceUnit != newPriceUnit
	}
	if priceChanged {
		if _, err := tx.Exec(ctx, `UPDATE product_prices SET effective_to = $2 WHERE product_id = $1 AND effective_to IS NULL`, id, now); err != nil {
			return Product{}, fmt.Errorf("close current price: %w", err)
		}
		if newPrice != nil {
			if err := createPrice(ctx, tx, id, current.CurrentPriceCents, *newPrice, newPriceUnit, actorID, requestID, "manual", now); err != nil {
				return Product{}, err
			}
		}
	}
	if err := bumpCatalogVersion(ctx, tx, now); err != nil {
		return Product{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "catalog.product_updated", "product", id, requestID, map[string]any{"before": current, "patch": patch}); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Product{}, fmt.Errorf("commit update product: %w", err)
	}
	return r.GetProduct(ctx, id)
}

func (r *PostgresRepository) PriceHistory(ctx context.Context, productID uuid.UUID) ([]Price, error) {
	if _, err := r.GetProduct(ctx, productID); err != nil {
		return nil, err
	}
	const query = `
		SELECT id, product_id, amount_cents, price_unit, effective_from, effective_to, actor_id
		FROM product_prices
		WHERE product_id = $1
		ORDER BY effective_from DESC, id DESC`
	rows, err := r.pool.Query(ctx, query, productID)
	if err != nil {
		return nil, fmt.Errorf("list price history: %w", err)
	}
	defer rows.Close()
	prices := make([]Price, 0)
	for rows.Next() {
		var price Price
		if err := rows.Scan(&price.ID, &price.ProductID, &price.AmountCents, &price.PriceUnit, &price.EffectiveFrom, &price.EffectiveTo, &price.ActorID); err != nil {
			return nil, fmt.Errorf("scan price: %w", err)
		}
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prices: %w", err)
	}
	return prices, nil
}

func (r *PostgresRepository) Snapshot(ctx context.Context) (Snapshot, error) {
	var snapshot Snapshot
	if err := r.pool.QueryRow(ctx, `SELECT version, updated_at FROM catalog_state WHERE singleton = TRUE`).Scan(&snapshot.Version, &snapshot.GeneratedAt); err != nil {
		return Snapshot{}, fmt.Errorf("get catalog version: %w", err)
	}
	active := true
	const query = productSelect + `
		WHERE p.active = $1 AND p.sellable = TRUE
		ORDER BY lower(p.name), p.id`
	products, err := queryProducts(ctx, r.pool, query, active)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Products = products
	return snapshot, nil
}

const productSelect = `
	SELECT p.id, p.category_id, c.name, p.name, p.product_type, p.sale_mode,
	       p.base_unit, p.sellable, p.track_lots, p.active,
	       b.code, pp.amount_cents, pp.price_unit, p.created_at, p.updated_at
	FROM products p
	JOIN categories c ON c.id = p.category_id
	LEFT JOIN product_barcodes b ON b.product_id = p.id
	LEFT JOIN product_prices pp ON pp.product_id = p.id AND pp.effective_to IS NULL`

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func queryProducts(ctx context.Context, queryer rowsQueryer, query string, args ...any) ([]Product, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	products := make([]Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

func scanProduct(row rowScanner) (Product, error) {
	var product Product
	var barcode sql.NullString
	var price sql.NullInt64
	var priceUnit sql.NullString
	err := row.Scan(
		&product.ID,
		&product.CategoryID,
		&product.CategoryName,
		&product.Name,
		&product.ProductType,
		&product.SaleMode,
		&product.BaseUnit,
		&product.Sellable,
		&product.TrackLots,
		&product.Active,
		&barcode,
		&price,
		&priceUnit,
		&product.CreatedAt,
		&product.UpdatedAt,
	)
	if err != nil {
		return Product{}, err
	}
	if barcode.Valid {
		product.Barcode = &barcode.String
	}
	if price.Valid {
		product.CurrentPriceCents = &price.Int64
	}
	if priceUnit.Valid {
		value := PriceUnit(priceUnit.String)
		product.PriceUnit = &value
	}
	return product, nil
}

func mergeProduct(current Product, patch UpdateProductInput) CreateProductInput {
	input := CreateProductInput{
		CategoryID:  current.CategoryID,
		Name:        current.Name,
		ProductType: current.ProductType,
		SaleMode:    current.SaleMode,
		BaseUnit:    current.BaseUnit,
		Sellable:    current.Sellable,
		TrackLots:   current.TrackLots,
		PriceCents:  current.CurrentPriceCents,
	}
	if current.Barcode != nil {
		input.Barcode = *current.Barcode
	}
	if patch.CategoryID != nil {
		input.CategoryID = *patch.CategoryID
	}
	if patch.Name != nil {
		input.Name = *patch.Name
	}
	if patch.ProductType != nil {
		input.ProductType = *patch.ProductType
	}
	if patch.SaleMode != nil {
		input.SaleMode = *patch.SaleMode
	}
	if patch.BaseUnit != nil {
		input.BaseUnit = *patch.BaseUnit
	}
	if patch.Sellable != nil {
		input.Sellable = *patch.Sellable
	}
	if patch.TrackLots != nil {
		input.TrackLots = *patch.TrackLots
	}
	if patch.Barcode != nil {
		input.Barcode = *patch.Barcode
	}
	if patch.PriceCents != nil {
		input.PriceCents = patch.PriceCents
	}
	if !input.Sellable {
		input.PriceCents = nil
	}
	return input
}

func requireActiveCategory(ctx context.Context, tx pgx.Tx, categoryID uuid.UUID) error {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT active FROM categories WHERE id = $1`, categoryID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: category does not exist", ErrInvalid)
	} else if err != nil {
		return fmt.Errorf("find category: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: category is inactive", ErrInvalid)
	}
	return nil
}

func replaceBarcode(ctx context.Context, tx pgx.Tx, productID uuid.UUID, barcode string, now time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM product_barcodes WHERE product_id = $1`, productID); err != nil {
		return fmt.Errorf("remove barcode: %w", err)
	}
	if barcode == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO product_barcodes (id, product_id, code, created_at) VALUES ($1, $2, $3, $4)`, uuid.New(), productID, barcode, now); err != nil {
		return mapWriteError("insert barcode", err)
	}
	return nil
}

func createPrice(
	ctx context.Context,
	tx pgx.Tx,
	productID uuid.UUID,
	oldPrice *int64,
	newPrice int64,
	unit PriceUnit,
	actorID uuid.UUID,
	requestID string,
	kind string,
	now time.Time,
) error {
	priceID := uuid.New()
	const insertPrice = `
		INSERT INTO product_prices (id, product_id, amount_cents, price_unit, effective_from, actor_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $5)`
	if _, err := tx.Exec(ctx, insertPrice, priceID, productID, newPrice, unit, now, actorID); err != nil {
		return mapWriteError("insert price", err)
	}
	batchID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO price_change_batches (id, kind, actor_id, request_id, created_at) VALUES ($1, $2, $3, $4, $5)`, batchID, kind, actorID, requestID, now); err != nil {
		return fmt.Errorf("insert price batch: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO price_change_items (batch_id, product_id, old_amount_cents, new_amount_cents) VALUES ($1, $2, $3, $4)`, batchID, productID, oldPrice, newPrice); err != nil {
		return fmt.Errorf("insert price change: %w", err)
	}
	return nil
}

func bumpCatalogVersion(ctx context.Context, tx pgx.Tx, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE catalog_state SET version = version + 1, updated_at = $1 WHERE singleton = TRUE`, now); err != nil {
		return fmt.Errorf("bump catalog version: %w", err)
	}
	return nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, requestID string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode audit data: %w", err)
	}
	const query = `
		INSERT INTO audit_events (id, actor_id, action, entity_type, entity_id, request_id, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`
	if _, err := tx.Exec(ctx, query, uuid.New(), actorID, action, entityType, entityID, requestID, string(payload)); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func mapWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: duplicate catalog value", ErrConflict)
		case "23503", "23514":
			return fmt.Errorf("%w: catalog constraint failed", ErrInvalid)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func sameInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameStringPointer(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func inputBarcodePointer(barcode string) *string {
	if barcode == "" {
		return nil
	}
	return &barcode
}
