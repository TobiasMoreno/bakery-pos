package catalog

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var percentagePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,2})?$`)

func (r *PostgresRepository) PreviewBulkPrices(ctx context.Context, input BulkPriceRequest) (BulkPricePreview, error) {
	percentage, normalized, err := parsePercentage(input.Percentage)
	if err != nil {
		return BulkPricePreview{}, err
	}
	items, err := loadBulkPriceItems(ctx, r.pool, input.ProductIDs, percentage, false)
	if err != nil {
		return BulkPricePreview{}, err
	}
	if len(items) == 0 {
		return BulkPricePreview{}, fmt.Errorf("%w: no active sellable products matched", ErrInvalid)
	}
	return buildBulkPreview(normalized, items)
}

func (r *PostgresRepository) CommitBulkPrices(ctx context.Context, input BulkPriceCommitRequest, actorID uuid.UUID, requestID string) (BulkPricePreview, error) {
	percentage, normalized, err := parsePercentage(input.Percentage)
	if err != nil {
		return BulkPricePreview{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BulkPricePreview{}, fmt.Errorf("begin bulk price update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	items, err := loadBulkPriceItems(ctx, tx, input.ProductIDs, percentage, true)
	if err != nil {
		return BulkPricePreview{}, err
	}
	if len(items) == 0 {
		return BulkPricePreview{}, fmt.Errorf("%w: no active sellable products matched", ErrInvalid)
	}
	preview, err := buildBulkPreview(normalized, items)
	if err != nil {
		return BulkPricePreview{}, err
	}
	if subtle.ConstantTimeCompare([]byte(preview.PreviewHash), []byte(strings.TrimSpace(input.PreviewHash))) != 1 {
		return BulkPricePreview{}, fmt.Errorf("%w: catalog prices changed after preview", ErrConflict)
	}

	now := r.now().UTC()
	batchID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO price_change_batches (id, kind, actor_id, request_id, created_at) VALUES ($1, 'bulk', $2, $3, $4)`, batchID, actorID, requestID, now); err != nil {
		return BulkPricePreview{}, fmt.Errorf("insert bulk price batch: %w", err)
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `UPDATE product_prices SET effective_to = $2 WHERE product_id = $1 AND effective_to IS NULL`, item.ProductID, now); err != nil {
			return BulkPricePreview{}, fmt.Errorf("close bulk price: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO product_prices (id, product_id, amount_cents, price_unit, effective_from, actor_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $5)`, uuid.New(), item.ProductID, item.NewPriceCents, item.PriceUnit, now, actorID); err != nil {
			return BulkPricePreview{}, mapWriteError("insert bulk price", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO price_change_items (batch_id, product_id, old_amount_cents, new_amount_cents)
			VALUES ($1, $2, $3, $4)`, batchID, item.ProductID, item.OldPriceCents, item.NewPriceCents); err != nil {
			return BulkPricePreview{}, fmt.Errorf("insert bulk price item: %w", err)
		}
	}
	if err := bumpCatalogVersion(ctx, tx, now); err != nil {
		return BulkPricePreview{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "pricing.bulk_updated", "price_change_batch", batchID, requestID, preview); err != nil {
		return BulkPricePreview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BulkPricePreview{}, fmt.Errorf("commit bulk price update: %w", err)
	}
	return preview, nil
}

func parsePercentage(value string) (*big.Rat, string, error) {
	normalized := strings.TrimSpace(value)
	if !percentagePattern.MatchString(normalized) {
		return nil, "", fmt.Errorf("%w: percentage must be a positive decimal with at most two decimals", ErrInvalid)
	}
	percentage, ok := new(big.Rat).SetString(normalized)
	if !ok || percentage.Sign() <= 0 || percentage.Cmp(big.NewRat(1000, 1)) > 0 {
		return nil, "", fmt.Errorf("%w: percentage must be greater than 0 and at most 1000", ErrInvalid)
	}
	return percentage, normalized, nil
}

func calculateIncreasedPrice(current int64, percentage *big.Rat) (int64, error) {
	factor := new(big.Rat).Add(big.NewRat(100, 1), percentage)
	value := new(big.Rat).Mul(big.NewRat(current, 1), factor)
	value.Quo(value, big.NewRat(100, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value.Num(), value.Denom(), remainder)
	twiceRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if twiceRemainder.Cmp(value.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() || quotient.Sign() <= 0 {
		return 0, fmt.Errorf("%w: calculated price is out of range", ErrInvalid)
	}
	return quotient.Int64(), nil
}

func loadBulkPriceItems(ctx context.Context, queryer rowsQueryer, productIDs []uuid.UUID, percentage *big.Rat, lock bool) ([]BulkPriceItem, error) {
	query := `
		SELECT p.id, p.name, pp.amount_cents, pp.price_unit
		FROM products p
		JOIN product_prices pp ON pp.product_id = p.id AND pp.effective_to IS NULL
		WHERE p.active = TRUE AND p.sellable = TRUE
		  AND (COALESCE(cardinality($1::uuid[]), 0) = 0 OR p.id = ANY($1::uuid[]))
		ORDER BY p.id`
	if lock {
		query += ` FOR UPDATE OF p, pp`
	}
	rows, err := queryer.Query(ctx, query, productIDs)
	if err != nil {
		return nil, fmt.Errorf("load bulk prices: %w", err)
	}
	defer rows.Close()
	items := make([]BulkPriceItem, 0)
	for rows.Next() {
		var item BulkPriceItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.OldPriceCents, &item.PriceUnit); err != nil {
			return nil, fmt.Errorf("scan bulk price: %w", err)
		}
		item.NewPriceCents, err = calculateIncreasedPrice(item.OldPriceCents, percentage)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bulk prices: %w", err)
	}
	return items, nil
}

func buildBulkPreview(percentage string, items []BulkPriceItem) (BulkPricePreview, error) {
	payload, err := json.Marshal(struct {
		Percentage string          `json:"percentage"`
		Items      []BulkPriceItem `json:"items"`
	}{Percentage: percentage, Items: items})
	if err != nil {
		return BulkPricePreview{}, fmt.Errorf("encode bulk price preview: %w", err)
	}
	hash := sha256.Sum256(payload)
	return BulkPricePreview{
		Percentage:  percentage,
		PreviewHash: hex.EncodeToString(hash[:]),
		Items:       items,
	}, nil
}
