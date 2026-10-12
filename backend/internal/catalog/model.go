package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("catalog item not found")
	ErrConflict = errors.New("catalog conflict")
	ErrInvalid  = errors.New("invalid catalog data")
)

type ProductType string

const (
	ProductTypeIngredient ProductType = "ingredient"
	ProductTypeProduced   ProductType = "produced"
	ProductTypeResale     ProductType = "resale"
)

type SaleMode string

const (
	SaleModeUnit   SaleMode = "unit"
	SaleModeWeight SaleMode = "weight"
)

type BaseUnit string

const (
	BaseUnitUnit BaseUnit = "unit"
	BaseUnitGram BaseUnit = "g"
	BaseUnitML   BaseUnit = "ml"
)

type PriceUnit string

const (
	PriceUnitItem     PriceUnit = "unit"
	PriceUnitKilogram PriceUnit = "kg"
)

type Category struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
	ID                uuid.UUID   `json:"id"`
	CategoryID        uuid.UUID   `json:"category_id"`
	CategoryName      string      `json:"category_name"`
	Name              string      `json:"name"`
	ProductType       ProductType `json:"product_type"`
	SaleMode          SaleMode    `json:"sale_mode"`
	BaseUnit          BaseUnit    `json:"base_unit"`
	Sellable          bool        `json:"sellable"`
	TrackLots         bool        `json:"track_lots"`
	Active            bool        `json:"active"`
	Barcode           *string     `json:"barcode"`
	CurrentPriceCents *int64      `json:"current_price_cents"`
	PriceUnit         *PriceUnit  `json:"price_unit"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type Price struct {
	ID            uuid.UUID  `json:"id"`
	ProductID     uuid.UUID  `json:"product_id"`
	AmountCents   int64      `json:"amount_cents"`
	PriceUnit     PriceUnit  `json:"price_unit"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	ActorID       uuid.UUID  `json:"actor_id"`
}

type CreateProductInput struct {
	CategoryID  uuid.UUID   `json:"category_id"`
	Name        string      `json:"name"`
	ProductType ProductType `json:"product_type"`
	SaleMode    SaleMode    `json:"sale_mode"`
	BaseUnit    BaseUnit    `json:"base_unit"`
	Sellable    bool        `json:"sellable"`
	TrackLots   bool        `json:"track_lots"`
	Barcode     string      `json:"barcode"`
	PriceCents  *int64      `json:"price_cents"`
}

type UpdateProductInput struct {
	CategoryID  *uuid.UUID   `json:"category_id"`
	Name        *string      `json:"name"`
	ProductType *ProductType `json:"product_type"`
	SaleMode    *SaleMode    `json:"sale_mode"`
	BaseUnit    *BaseUnit    `json:"base_unit"`
	Sellable    *bool        `json:"sellable"`
	TrackLots   *bool        `json:"track_lots"`
	Active      *bool        `json:"active"`
	Barcode     *string      `json:"barcode"`
	PriceCents  *int64       `json:"price_cents"`
}

type ProductFilter struct {
	Query  string
	Active *bool
	Limit  int
	Offset int
}

type ProductList struct {
	Items  []Product `json:"items"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

type Snapshot struct {
	Version     int64     `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	Products    []Product `json:"products"`
}

type BulkPriceRequest struct {
	Percentage string      `json:"percentage"`
	ProductIDs []uuid.UUID `json:"product_ids"`
}

type BulkPriceCommitRequest struct {
	BulkPriceRequest
	PreviewHash string `json:"preview_hash"`
}

type BulkPriceItem struct {
	ProductID     uuid.UUID `json:"product_id"`
	ProductName   string    `json:"product_name"`
	OldPriceCents int64     `json:"old_price_cents"`
	NewPriceCents int64     `json:"new_price_cents"`
	PriceUnit     PriceUnit `json:"price_unit"`
}

type BulkPricePreview struct {
	Percentage  string          `json:"percentage"`
	PreviewHash string          `json:"preview_hash"`
	Items       []BulkPriceItem `json:"items"`
}

type ImportPreviewRequest struct {
	CSV string `json:"csv"`
}

type ImportCommitRequest struct {
	CSV         string `json:"csv"`
	PreviewHash string `json:"preview_hash"`
}

type ImportRow struct {
	RowNumber   int         `json:"row_number"`
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	ProductType ProductType `json:"product_type"`
	SaleMode    SaleMode    `json:"sale_mode"`
	BaseUnit    BaseUnit    `json:"base_unit"`
	Sellable    bool        `json:"sellable"`
	TrackLots   bool        `json:"track_lots"`
	Barcode     string      `json:"barcode"`
	PriceCents  *int64      `json:"price_cents"`
	Errors      []string    `json:"errors"`
}

type ImportPreview struct {
	PreviewHash string      `json:"preview_hash"`
	ValidCount  int         `json:"valid_count"`
	ErrorCount  int         `json:"error_count"`
	Rows        []ImportRow `json:"rows"`
}

type ImportResult struct {
	ImportedCount  int   `json:"imported_count"`
	CatalogVersion int64 `json:"catalog_version"`
}
