package catalog

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var moneyPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,2})?$`)

var requiredImportHeaders = []string{
	"name",
	"category",
	"product_type",
	"sale_mode",
	"base_unit",
	"sellable",
	"track_lots",
	"barcode",
	"price_ars",
}

func parseImportCSV(csvData string) (ImportPreview, error) {
	if strings.TrimSpace(csvData) == "" {
		return ImportPreview{}, fmt.Errorf("%w: CSV is empty", ErrInvalid)
	}
	reader := csv.NewReader(strings.NewReader(csvData))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return ImportPreview{}, fmt.Errorf("%w: cannot read CSV header", ErrInvalid)
	}
	indexes := make(map[string]int, len(header))
	for index, value := range header {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
		if _, exists := indexes[name]; exists {
			return ImportPreview{}, fmt.Errorf("%w: duplicated CSV header %q", ErrInvalid, name)
		}
		indexes[name] = index
	}
	for _, required := range requiredImportHeaders {
		if _, ok := indexes[required]; !ok {
			return ImportPreview{}, fmt.Errorf("%w: missing CSV header %q", ErrInvalid, required)
		}
	}

	rows := make([]ImportRow, 0)
	seenNames := make(map[string]int)
	seenBarcodes := make(map[string]int)
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ImportPreview{}, fmt.Errorf("%w: invalid CSV at row %d", ErrInvalid, rowNumber)
		}
		if len(rows) >= 5000 {
			return ImportPreview{}, fmt.Errorf("%w: CSV cannot contain more than 5000 rows", ErrInvalid)
		}
		if blankCSVRecord(record) {
			continue
		}
		row := parseImportRow(record, indexes, rowNumber)
		nameKey := strings.ToLower(row.Name)
		if previous, exists := seenNames[nameKey]; nameKey != "" && exists {
			row.Errors = append(row.Errors, fmt.Sprintf("nombre duplicado con la fila %d", previous))
		} else if nameKey != "" {
			seenNames[nameKey] = rowNumber
		}
		if previous, exists := seenBarcodes[row.Barcode]; row.Barcode != "" && exists {
			row.Errors = append(row.Errors, fmt.Sprintf("código duplicado con la fila %d", previous))
		} else if row.Barcode != "" {
			seenBarcodes[row.Barcode] = rowNumber
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return ImportPreview{}, fmt.Errorf("%w: CSV has no product rows", ErrInvalid)
	}
	preview := ImportPreview{Rows: rows}
	countImportRows(&preview)
	hash, err := hashImportRows(rows)
	if err != nil {
		return ImportPreview{}, err
	}
	preview.PreviewHash = hash
	return preview, nil
}

func parseImportRow(record []string, indexes map[string]int, rowNumber int) ImportRow {
	value := func(name string) string {
		index := indexes[name]
		if index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	row := ImportRow{
		RowNumber:   rowNumber,
		Name:        normalizeName(value("name")),
		Category:    normalizeName(value("category")),
		ProductType: ProductType(strings.ToLower(value("product_type"))),
		SaleMode:    SaleMode(strings.ToLower(value("sale_mode"))),
		BaseUnit:    BaseUnit(strings.ToLower(value("base_unit"))),
		Barcode:     value("barcode"),
		Errors:      make([]string, 0),
	}
	if length := utf8.RuneCountInString(row.Name); length < 1 || length > 120 {
		row.Errors = append(row.Errors, "name debe tener entre 1 y 120 caracteres")
	}
	if length := utf8.RuneCountInString(row.Category); length < 1 || length > 80 {
		row.Errors = append(row.Errors, "category debe tener entre 1 y 80 caracteres")
	}
	if !validProductType(row.ProductType) {
		row.Errors = append(row.Errors, "product_type debe ser ingredient, produced o resale")
	}
	if !validSaleMode(row.SaleMode) {
		row.Errors = append(row.Errors, "sale_mode debe ser unit o weight")
	}
	if !validBaseUnit(row.BaseUnit) {
		row.Errors = append(row.Errors, "base_unit debe ser unit, g o ml")
	}
	var ok bool
	row.Sellable, ok = parseCSVBool(value("sellable"))
	if !ok {
		row.Errors = append(row.Errors, "sellable debe ser true/false o si/no")
	}
	row.TrackLots, ok = parseCSVBool(value("track_lots"))
	if !ok {
		row.Errors = append(row.Errors, "track_lots debe ser true/false o si/no")
	}
	if utf8.RuneCountInString(row.Barcode) > 64 {
		row.Errors = append(row.Errors, "barcode no puede superar 64 caracteres")
	}
	priceValue := value("price_ars")
	if priceValue != "" {
		price, err := parseARSToCents(priceValue)
		if err != nil {
			row.Errors = append(row.Errors, "price_ars debe ser un importe positivo con hasta 2 decimales")
		} else {
			row.PriceCents = &price
		}
	}
	validationInput := CreateProductInput{
		CategoryID:  uuid.New(),
		Name:        row.Name,
		ProductType: row.ProductType,
		SaleMode:    row.SaleMode,
		BaseUnit:    row.BaseUnit,
		Sellable:    row.Sellable,
		TrackLots:   row.TrackLots,
		Barcode:     row.Barcode,
		PriceCents:  row.PriceCents,
	}
	if err := validateProductInput(validationInput); err != nil {
		row.Errors = append(row.Errors, strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": "))
	}
	row.Errors = slices.Compact(row.Errors)
	return row
}

func (r *PostgresRepository) MarkImportConflicts(ctx context.Context, preview *ImportPreview) error {
	names := make([]string, 0, len(preview.Rows))
	barcodes := make([]string, 0, len(preview.Rows))
	for _, row := range preview.Rows {
		names = append(names, strings.ToLower(row.Name))
		if row.Barcode != "" {
			barcodes = append(barcodes, row.Barcode)
		}
	}
	existingNames := make(map[string]struct{})
	rows, err := r.pool.Query(ctx, `SELECT lower(name) FROM products WHERE lower(name) = ANY($1::text[])`, names)
	if err != nil {
		return fmt.Errorf("find import name conflicts: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan import name conflict: %w", err)
		}
		existingNames[name] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate import name conflicts: %w", err)
	}
	existingBarcodes := make(map[string]struct{})
	rows, err = r.pool.Query(ctx, `SELECT code FROM product_barcodes WHERE code = ANY($1::text[])`, barcodes)
	if err != nil {
		return fmt.Errorf("find import barcode conflicts: %w", err)
	}
	for rows.Next() {
		var barcode string
		if err := rows.Scan(&barcode); err != nil {
			rows.Close()
			return fmt.Errorf("scan import barcode conflict: %w", err)
		}
		existingBarcodes[barcode] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate import barcode conflicts: %w", err)
	}
	for index := range preview.Rows {
		row := &preview.Rows[index]
		if _, exists := existingNames[strings.ToLower(row.Name)]; exists {
			row.Errors = append(row.Errors, "ya existe un producto con ese nombre")
		}
		if _, exists := existingBarcodes[row.Barcode]; row.Barcode != "" && exists {
			row.Errors = append(row.Errors, "ya existe un producto con ese código")
		}
	}
	return nil
}

func (r *PostgresRepository) CommitImport(ctx context.Context, rows []ImportRow, actorID uuid.UUID, requestID string) (ImportResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin catalog import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := r.now().UTC()
	categories := make(map[string]uuid.UUID)
	for _, row := range rows {
		key := strings.ToLower(row.Category)
		if _, exists := categories[key]; exists {
			continue
		}
		var categoryID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM categories WHERE lower(name) = $1 FOR UPDATE`, key).Scan(&categoryID)
		if errors.Is(err, pgx.ErrNoRows) {
			categoryID = uuid.New()
			if _, err := tx.Exec(ctx, `INSERT INTO categories (id, name, active, created_at, updated_at) VALUES ($1, $2, TRUE, $3, $3)`, categoryID, row.Category, now); err != nil {
				return ImportResult{}, mapWriteError("insert import category", err)
			}
		} else if err != nil {
			return ImportResult{}, fmt.Errorf("find import category: %w", err)
		}
		categories[key] = categoryID
	}

	batchID := uuid.New()
	priceBatchCreated := false
	for _, row := range rows {
		productID := uuid.New()
		const insertProduct = `
			INSERT INTO products (
				id, category_id, name, product_type, sale_mode, base_unit,
				sellable, track_lots, active, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9, $9)`
		if _, err := tx.Exec(ctx, insertProduct, productID, categories[strings.ToLower(row.Category)], row.Name, row.ProductType, row.SaleMode, row.BaseUnit, row.Sellable, row.TrackLots, now); err != nil {
			return ImportResult{}, mapWriteError("insert imported product", err)
		}
		if row.Barcode != "" {
			if err := replaceBarcode(ctx, tx, productID, row.Barcode, now); err != nil {
				return ImportResult{}, err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_balances (product_id, quantity, updated_at) VALUES ($1, 0, $2)`, productID, now); err != nil {
			return ImportResult{}, fmt.Errorf("create imported inventory balance: %w", err)
		}
		if row.PriceCents != nil {
			if !priceBatchCreated {
				if _, err := tx.Exec(ctx, `INSERT INTO price_change_batches (id, kind, actor_id, request_id, created_at) VALUES ($1, 'import', $2, $3, $4)`, batchID, actorID, requestID, now); err != nil {
					return ImportResult{}, fmt.Errorf("insert import price batch: %w", err)
				}
				priceBatchCreated = true
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO product_prices (id, product_id, amount_cents, price_unit, effective_from, actor_id, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $5)`, uuid.New(), productID, *row.PriceCents, priceUnitFor(row.SaleMode), now, actorID); err != nil {
				return ImportResult{}, mapWriteError("insert imported price", err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO price_change_items (batch_id, product_id, new_amount_cents) VALUES ($1, $2, $3)`, batchID, productID, *row.PriceCents); err != nil {
				return ImportResult{}, fmt.Errorf("insert imported price item: %w", err)
			}
		}
	}
	if err := bumpCatalogVersion(ctx, tx, now); err != nil {
		return ImportResult{}, err
	}
	importID := uuid.New()
	if err := insertAudit(ctx, tx, actorID, "catalog.imported", "catalog_import", importID, requestID, map[string]any{"count": len(rows)}); err != nil {
		return ImportResult{}, err
	}
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM catalog_state WHERE singleton = TRUE`).Scan(&version); err != nil {
		return ImportResult{}, fmt.Errorf("read imported catalog version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, mapWriteError("commit catalog import", err)
	}
	return ImportResult{ImportedCount: len(rows), CatalogVersion: version}, nil
}

func parseCSVBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "si", "sí", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	default:
		return false, false
	}
}

func parseARSToCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if !moneyPattern.MatchString(value) {
		return 0, ErrInvalid
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok || amount.Sign() <= 0 {
		return 0, ErrInvalid
	}
	amount.Mul(amount, big.NewRat(100, 1))
	if !amount.IsInt() || !amount.Num().IsInt64() {
		return 0, ErrInvalid
	}
	return amount.Num().Int64(), nil
}

func hashImportRows(rows []ImportRow) (string, error) {
	type canonicalRow struct {
		Name        string      `json:"name"`
		Category    string      `json:"category"`
		ProductType ProductType `json:"product_type"`
		SaleMode    SaleMode    `json:"sale_mode"`
		BaseUnit    BaseUnit    `json:"base_unit"`
		Sellable    bool        `json:"sellable"`
		TrackLots   bool        `json:"track_lots"`
		Barcode     string      `json:"barcode"`
		PriceCents  *int64      `json:"price_cents"`
	}
	canonical := make([]canonicalRow, 0, len(rows))
	for _, row := range rows {
		canonical = append(canonical, canonicalRow{
			Name: row.Name, Category: row.Category, ProductType: row.ProductType,
			SaleMode: row.SaleMode, BaseUnit: row.BaseUnit, Sellable: row.Sellable,
			TrackLots: row.TrackLots, Barcode: row.Barcode, PriceCents: row.PriceCents,
		})
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode import preview: %w", err)
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

func samePreviewHash(expected, received string) bool {
	expected = strings.TrimSpace(expected)
	received = strings.TrimSpace(received)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(received)) == 1
}

func countImportRows(preview *ImportPreview) {
	preview.ValidCount = 0
	preview.ErrorCount = 0
	for _, row := range preview.Rows {
		if len(row.Errors) == 0 {
			preview.ValidCount++
		} else {
			preview.ErrorCount++
		}
	}
}

func blankCSVRecord(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
