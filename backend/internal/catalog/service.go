package catalog

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Service struct {
	repository *PostgresRepository
}

func NewService(repository *PostgresRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	return s.repository.ListCategories(ctx)
}

func (s *Service) CreateCategory(ctx context.Context, name string, actorID uuid.UUID, requestID string) (Category, error) {
	name = normalizeName(name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 80 {
		return Category{}, fmt.Errorf("%w: category name must contain between 1 and 80 characters", ErrInvalid)
	}
	return s.repository.CreateCategory(ctx, name, actorID, requestID)
}

func (s *Service) ListProducts(ctx context.Context, filter ProductFilter) (ProductList, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	items, total, err := s.repository.ListProducts(ctx, filter)
	if err != nil {
		return ProductList{}, err
	}
	return ProductList{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (Product, error) {
	return s.repository.GetProduct(ctx, id)
}

func (s *Service) CreateProduct(ctx context.Context, input CreateProductInput, actorID uuid.UUID, requestID string) (Product, error) {
	input.Name = normalizeName(input.Name)
	input.Barcode = strings.TrimSpace(input.Barcode)
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	return s.repository.CreateProduct(ctx, input, actorID, requestID)
}

func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, input UpdateProductInput, actorID uuid.UUID, requestID string) (Product, error) {
	if input.Name != nil {
		name := normalizeName(*input.Name)
		input.Name = &name
	}
	if input.Barcode != nil {
		barcode := strings.TrimSpace(*input.Barcode)
		input.Barcode = &barcode
	}
	return s.repository.UpdateProduct(ctx, id, input, actorID, requestID)
}

func (s *Service) PriceHistory(ctx context.Context, productID uuid.UUID) ([]Price, error) {
	return s.repository.PriceHistory(ctx, productID)
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	return s.repository.Snapshot(ctx)
}

func (s *Service) PreviewBulkPrices(ctx context.Context, input BulkPriceRequest) (BulkPricePreview, error) {
	return s.repository.PreviewBulkPrices(ctx, input)
}

func (s *Service) CommitBulkPrices(ctx context.Context, input BulkPriceCommitRequest, actorID uuid.UUID, requestID string) (BulkPricePreview, error) {
	if strings.TrimSpace(input.PreviewHash) == "" {
		return BulkPricePreview{}, fmt.Errorf("%w: preview hash is required", ErrInvalid)
	}
	return s.repository.CommitBulkPrices(ctx, input, actorID, requestID)
}

func (s *Service) PreviewImport(ctx context.Context, csvData string) (ImportPreview, error) {
	preview, err := parseImportCSV(csvData)
	if err != nil {
		return ImportPreview{}, err
	}
	if err := s.repository.MarkImportConflicts(ctx, &preview); err != nil {
		return ImportPreview{}, err
	}
	countImportRows(&preview)
	return preview, nil
}

func (s *Service) CommitImport(ctx context.Context, input ImportCommitRequest, actorID uuid.UUID, requestID string) (ImportResult, error) {
	preview, err := parseImportCSV(input.CSV)
	if err != nil {
		return ImportResult{}, err
	}
	if preview.ErrorCount > 0 {
		return ImportResult{}, fmt.Errorf("%w: import contains invalid rows", ErrInvalid)
	}
	if !samePreviewHash(preview.PreviewHash, input.PreviewHash) {
		return ImportResult{}, fmt.Errorf("%w: CSV changed after preview", ErrConflict)
	}
	return s.repository.CommitImport(ctx, preview.Rows, actorID, requestID)
}

func normalizeName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateProductInput(input CreateProductInput) error {
	nameLength := utf8.RuneCountInString(input.Name)
	if nameLength < 1 || nameLength > 120 {
		return fmt.Errorf("%w: product name must contain between 1 and 120 characters", ErrInvalid)
	}
	if input.CategoryID == uuid.Nil {
		return fmt.Errorf("%w: category is required", ErrInvalid)
	}
	if !validProductType(input.ProductType) {
		return fmt.Errorf("%w: unknown product type", ErrInvalid)
	}
	if !validSaleMode(input.SaleMode) || !validBaseUnit(input.BaseUnit) {
		return fmt.Errorf("%w: unknown sale mode or base unit", ErrInvalid)
	}
	if input.Sellable {
		if input.SaleMode == SaleModeUnit && input.BaseUnit != BaseUnitUnit {
			return fmt.Errorf("%w: products sold by unit must use unit as base", ErrInvalid)
		}
		if input.SaleMode == SaleModeWeight && input.BaseUnit != BaseUnitGram {
			return fmt.Errorf("%w: products sold by weight must use grams as base", ErrInvalid)
		}
		if input.PriceCents == nil || *input.PriceCents <= 0 {
			return fmt.Errorf("%w: sellable products require a positive price", ErrInvalid)
		}
	} else if input.PriceCents != nil {
		return fmt.Errorf("%w: non-sellable products cannot have a sale price", ErrInvalid)
	}
	if utf8.RuneCountInString(input.Barcode) > 64 {
		return fmt.Errorf("%w: barcode is too long", ErrInvalid)
	}
	return nil
}

func validProductType(value ProductType) bool {
	return value == ProductTypeIngredient || value == ProductTypeProduced || value == ProductTypeResale
}

func validSaleMode(value SaleMode) bool {
	return value == SaleModeUnit || value == SaleModeWeight
}

func validBaseUnit(value BaseUnit) bool {
	return value == BaseUnitUnit || value == BaseUnitGram || value == BaseUnitML
}

func priceUnitFor(mode SaleMode) PriceUnit {
	if mode == SaleModeWeight {
		return PriceUnitKilogram
	}
	return PriceUnitItem
}
