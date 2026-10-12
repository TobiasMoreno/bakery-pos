package catalog

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateProductInputAcceptsWeightedProduct(t *testing.T) {
	price := int64(950000)
	input := CreateProductInput{
		CategoryID:  uuid.New(),
		Name:        "Pan de campo",
		ProductType: ProductTypeProduced,
		SaleMode:    SaleModeWeight,
		BaseUnit:    BaseUnitGram,
		Sellable:    true,
		PriceCents:  &price,
	}
	if err := validateProductInput(input); err != nil {
		t.Fatalf("validateProductInput() error = %v", err)
	}
}

func TestValidateProductInputRejectsFractionalUnitModel(t *testing.T) {
	price := int64(120000)
	err := validateProductInput(CreateProductInput{
		CategoryID:  uuid.New(),
		Name:        "Gaseosa",
		ProductType: ProductTypeResale,
		SaleMode:    SaleModeUnit,
		BaseUnit:    BaseUnitGram,
		Sellable:    true,
		PriceCents:  &price,
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestValidateProductInputRequiresPriceForSellableProduct(t *testing.T) {
	err := validateProductInput(CreateProductInput{
		CategoryID:  uuid.New(),
		Name:        "Medialuna",
		ProductType: ProductTypeProduced,
		SaleMode:    SaleModeUnit,
		BaseUnit:    BaseUnitUnit,
		Sellable:    true,
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestMergeProductDisablingSaleClearsPrice(t *testing.T) {
	price := int64(120000)
	barcode := "779000000001"
	current := Product{
		CategoryID:        uuid.New(),
		Name:              "Gaseosa",
		ProductType:       ProductTypeResale,
		SaleMode:          SaleModeUnit,
		BaseUnit:          BaseUnitUnit,
		Sellable:          true,
		Barcode:           &barcode,
		CurrentPriceCents: &price,
	}
	sellable := false
	merged := mergeProduct(current, UpdateProductInput{Sellable: &sellable})
	if merged.PriceCents != nil {
		t.Fatalf("PriceCents = %v, want nil", *merged.PriceCents)
	}
	if merged.Barcode != barcode {
		t.Fatalf("Barcode = %q, want %q", merged.Barcode, barcode)
	}
}
