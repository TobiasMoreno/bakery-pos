package catalog

import "testing"

func TestCalculateIncreasedPriceRoundsHalfUp(t *testing.T) {
	percentage, _, err := parsePercentage("12.50")
	if err != nil {
		t.Fatal(err)
	}
	got, err := calculateIncreasedPrice(101, percentage)
	if err != nil {
		t.Fatal(err)
	}
	if got != 114 {
		t.Fatalf("price = %d, want 114", got)
	}
}

func TestParsePercentageRejectsFloatLikeAmbiguity(t *testing.T) {
	for _, value := range []string{"0", "-1", "1.234", "1e2", "1000.01"} {
		if _, _, err := parsePercentage(value); err == nil {
			t.Fatalf("parsePercentage(%q) unexpectedly succeeded", value)
		}
	}
}
