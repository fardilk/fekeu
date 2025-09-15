package ocr

import "strings"

// isPlausibleAmount applies lightweight heuristics to decide whether a
// matched numeric substring likely represents a monetary amount rather than
// a phone number, transaction id, or RRN. The heuristics are intentionally
// conservative: prefer strings that include currency hints or grouping
// separators, and reject very long digit-only strings or those starting with 0.
func isPlausibleAmount(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "rp") || strings.Contains(low, "idr") {
		return true
	}
	if strings.Contains(s, ".") || strings.Contains(s, ",") {
		d := onlyDigits(s)
		if len(d) >= 3 && d[0] != '0' {
			return true
		}
		return false
	}
	d := onlyDigits(s)
	if d == "" {
		return false
	}
	if d[0] == '0' {
		return false
	}
	if len(d) > 7 {
		return false
	}
	if len(d) >= 5 { // reject irregular mid-size ids like 250903
		if !(strings.HasSuffix(d, "000") || strings.HasSuffix(d, "500")) {
			return false
		}
	}
	if len(d) < 2 {
		return false
	}
	return true
}

// onlyDigits extracts decimal digits from a string.
func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
