package ocr

import "testing"

func TestParseAmountStripDecimals(t *testing.T) {
	amt, err := ParseAmountFromMatch("10.000,00")
	if err != nil || amt != 10000 {
		t.Fatalf("expected 10000 got %d err=%v", amt, err)
	}
	amt2, err2 := ParseAmountFromMatch("7,500.00")
	if err2 != nil || amt2 != 7500 {
		t.Fatalf("expected 7500 got %d err=%v", amt2, err2)
	}
}
