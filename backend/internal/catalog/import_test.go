package catalog

import "testing"

func TestParseImportCSVNormalizesMoneyAndBooleans(t *testing.T) {
	csvData := "name,category,product_type,sale_mode,base_unit,sellable,track_lots,barcode,price_ars\n" +
		"Pan de campo,Panificados,produced,weight,g,si,no,779000000001,9500.00\n"
	preview, err := parseImportCSV(csvData)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ErrorCount != 0 || preview.ValidCount != 1 {
		t.Fatalf("preview = %#v", preview)
	}
	if preview.Rows[0].PriceCents == nil || *preview.Rows[0].PriceCents != 950000 {
		t.Fatalf("price = %v, want 950000", preview.Rows[0].PriceCents)
	}
}

func TestParseImportCSVReportsDuplicateRows(t *testing.T) {
	csvData := "name,category,product_type,sale_mode,base_unit,sellable,track_lots,barcode,price_ars\n" +
		"Gaseosa,Bebidas,resale,unit,unit,true,false,7791,1200\n" +
		"Gaseosa,Bebidas,resale,unit,unit,true,false,7791,1200\n"
	preview, err := parseImportCSV(csvData)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ErrorCount != 1 || len(preview.Rows[1].Errors) < 2 {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestParseARSToCentsRejectsMoreThanTwoDecimals(t *testing.T) {
	if _, err := parseARSToCents("10.001"); err == nil {
		t.Fatal("expected invalid money error")
	}
}
