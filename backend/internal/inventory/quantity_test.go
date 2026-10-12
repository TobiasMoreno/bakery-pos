package inventory

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseQuantityKeepsThreeDecimalPrecision(t *testing.T) {
	testCases := map[string]string{
		"0":                   "0",
		"1":                   "1",
		"001.230":             "1.23",
		"-350.125":            "-350.125",
		"999999999999999.999": "999999999999999.999",
	}
	for input, expected := range testCases {
		t.Run(input, func(t *testing.T) {
			quantity, err := ParseQuantity(input)
			if err != nil {
				t.Fatalf("ParseQuantity() error = %v", err)
			}
			if quantity.String() != expected {
				t.Fatalf("String() = %q, want %q", quantity.String(), expected)
			}
		})
	}
}

func TestParseQuantityRejectsInvalidOrOverPreciseValues(t *testing.T) {
	for _, input := range []string{"", ".5", "1.", "1.0001", "1e3", "NaN", "1000000000000000"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseQuantity(input); !errors.Is(err, ErrInvalidQuantity) {
				t.Fatalf("error = %v, want ErrInvalidQuantity", err)
			}
		})
	}
}

func TestQuantityJSONUsesAString(t *testing.T) {
	quantity, err := ParseQuantity("12.5")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(quantity)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"12.5"` {
		t.Fatalf("JSON = %s", encoded)
	}

	var decoded Quantity
	if err := json.Unmarshal([]byte(`"-2.125"`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.String() != "-2.125" {
		t.Fatalf("decoded = %s", decoded.String())
	}
	if err := json.Unmarshal([]byte(`1.25`), &decoded); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("numeric JSON error = %v, want ErrInvalidQuantity", err)
	}
}
