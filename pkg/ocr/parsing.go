package ocr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParseAmountFromMatch normalizes a matched substring into an integer amount (whole currency units).
// It removes a trailing decimal part of exactly two digits (e.g., 10.000,00 -> 10000).
func ParseAmountFromMatch(found string) (int64, error) {
	centsRE := regexp.MustCompile(`[.,]\d{2}$`)
	foundTrim := strings.TrimSpace(found)
	if foundTrim == "" {
		return 0, fmt.Errorf("empty")
	}
	onlyDigitsLocal := func(s string) string { return onlyDigits(s) }
	var digits string
	if centsRE.MatchString(foundTrim) {
		lastDot := strings.LastIndex(foundTrim, ".")
		lastComma := strings.LastIndex(foundTrim, ",")
		if lastComma > lastDot {
			integerPart := foundTrim[:lastComma]
			digits = onlyDigitsLocal(integerPart)
		} else if lastDot > lastComma {
			integerPart := foundTrim[:lastDot]
			digits = onlyDigitsLocal(integerPart)
		} else {
			digits = onlyDigitsLocal(foundTrim)
		}
	} else {
		digits = onlyDigitsLocal(foundTrim)
	}
	if digits == "" {
		return 0, fmt.Errorf("no digits extracted from %q", found)
	}
	amtInt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount %q: %w", digits, err)
	}
	if amtInt < 0 {
		amtInt = -amtInt
	}
	return amtInt, nil
}
