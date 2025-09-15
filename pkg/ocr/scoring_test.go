package ocr

import "testing"

func TestBestAmountTotalPriority(t *testing.T) {
	// Rp50.000 larger, but TOTAL Rp40.000 should win due to TOTAL boost.
	matches := []string{"Rp50.000", "TOTAL Rp40.000"}
	amt, raw, ok := BestAmountFromMatches(matches)
	if !ok {
		t.Fatalf("no amount chosen")
	}
	if amt != 40000 {
		t.Fatalf("expected 40000 (TOTAL) got %d raw=%s", amt, raw)
	}
}
