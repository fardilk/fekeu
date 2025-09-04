package ocr

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract/v2"
)

// ExtractAmountFromImage performs light preprocessing + Tesseract OCR and attempts
// to extract a transfer/total amount. Returns amount in whole currency units (e.g. 4010000).
// If no amount is found returns (0,0,nil).
func ExtractAmountFromImage(path string) (int64, float64, string, error) {
	img, err := imaging.Open(path)
	if err != nil {
		return 0, 0, "", fmt.Errorf("open image: %w", err)
	}

	gray := imaging.Grayscale(img)

	h := gray.Bounds().Dy()
	if h < 800 {
		gray = imaging.Resize(gray, 0, 1200, imaging.Lanczos)
	}

	// write preprocessed image to system temp directory to avoid polluting watched dirs
	tmpFile, err := os.CreateTemp("", "ocr-*.png")
	tmp := path
	if err == nil {
		tmp = tmpFile.Name()
		_ = tmpFile.Close()
		if err := imaging.Save(gray, tmp); err != nil {
			// fallback to original path if temp save fails
			tmp = path
		}
	}

	client := gosseract.NewClient()
	defer client.Close()
	_ = client.SetLanguage("eng")
	_ = client.SetWhitelist("0123456789RpRp.,:()/- ")
	client.SetImage(tmp)
	text, err := client.Text()
	if tmp != path {
		_ = os.Remove(tmp)
	}
	if err != nil {
		return 0, 0, "", fmt.Errorf("ocr error: %w", err)
	}

	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	patterns := []string{
		`(?i)(?:jumlah(?:\s+transfer)?|total(?:\s+bayar)?|total pembayaran|transfer)[:\s]*Rp?[\s]*([0-9\.,]+)`,
		`(?i)Rp[\s]*([0-9\.,]+)`,
		`([0-9]{1,3}(?:[.,][0-9]{3})+)`,
		`([0-9]{5,})`,
	}

	var found string
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if m := re.FindStringSubmatch(text); len(m) >= 2 {
			found = m[1]
			break
		}
	}

	if found == "" {
		return 0, 0, "", nil
	}

	// Normalize the matched substring into whole currency units.
	// Handle formats like:
	//  - "53.000" -> 53000
	//  - "53.000,00" -> 53000
	//  - "Rp 53.000,00" -> 53000
	//  - "53,000.00" -> 53000 (US-style)
	centsRE := regexp.MustCompile(`[.,]\d{2}$`)
	foundTrim := strings.TrimSpace(found)

	onlyDigits := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
	}

	var digits string
	if centsRE.MatchString(foundTrim) {
		// If there is a trailing .00 or ,00 we treat that as the decimal part and strip it.
		// Determine which separator looks like the decimal separator by checking the last occurrence.
		lastDot := strings.LastIndex(foundTrim, ".")
		lastComma := strings.LastIndex(foundTrim, ",")
		if lastComma > lastDot {
			// comma appears last: treat comma as decimal separator, remove thousand separators like '.' and spaces
			integerPart := foundTrim[:lastComma]
			// remove dots, spaces and non-digits
			digits = onlyDigits(integerPart)
		} else if lastDot > lastComma {
			// dot appears last: treat dot as decimal separator (US style), remove commas as thousand separators
			integerPart := foundTrim[:lastDot]
			digits = onlyDigits(integerPart)
		} else {
			// unexpected: fallback
			digits = onlyDigits(foundTrim)
		}
	} else {
		// No explicit cents detected. Remove any thousand separators ('.' or ',') and parse remaining digits.
		// e.g. "53.000" or "53,000" -> 53000
		digits = onlyDigits(foundTrim)
	}

	if digits == "" {
		return 0, 0, "", fmt.Errorf("no digits extracted from %q", found)
	}

	amtInt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse amount %q: %w", digits, err)
	}

	conf := float64(len(found)) / float64(len(text)+1)
	if conf > 1 {
		conf = 1
	}
	if amtInt < 0 {
		amtInt = -amtInt
	}

	return amtInt, conf, found, nil
}

// FindAllMatches returns all numeric-like substrings that look like amounts found in the image text.
// It returns a slice of the raw matched substrings (un-normalized) in the order found.
// FindAllMatches returns all numeric-like substrings that look like amounts found in the image text.
// It also returns a boolean `isLikelyNonAmount` which is true when the image/text looks like a
// logo / non-amount image (very little text and no digits), so callers can surface a different
// user-facing message.
func FindAllMatches(path string) ([]string, bool, error) {
	img, err := imaging.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("open image: %w", err)
	}
	gray := imaging.Grayscale(img)
	h := gray.Bounds().Dy()
	if h < 800 {
		gray = imaging.Resize(gray, 0, 1200, imaging.Lanczos)
	}
	tmpFile, err := os.CreateTemp("", "ocr-*.png")
	tmp := path
	if err == nil {
		tmp = tmpFile.Name()
		_ = tmpFile.Close()
		if err := imaging.Save(gray, tmp); err != nil {
			tmp = path
		}
	}

	client := gosseract.NewClient()
	defer client.Close()
	_ = client.SetLanguage("eng")
	_ = client.SetWhitelist("0123456789RpRp.,:()/- ")
	client.SetImage(tmp)
	text, err := client.Text()
	if tmp != path {
		_ = os.Remove(tmp)
	}
	if err != nil {
		return nil, false, fmt.Errorf("ocr error: %w", err)
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	// Heuristic: if OCR produced very little text and there are no digits at all,
	// this is likely a logo/graphic or non-receipt image. We treat this as a
	// non-amount scenario so caller can show a clearer message.
	trimmed := strings.TrimSpace(text)
	digitCount := 0
	letterCount := 0
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digitCount++
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letterCount++
		}
	}
	isLikelyNonAmount := false
	if digitCount == 0 && len(trimmed) > 0 && len(trimmed) < 40 {
		// small amount of text and no digits -> likely logo
		isLikelyNonAmount = true
	}

	patterns := []string{
		`(?i)(?:jumlah(?:\s+transfer)?|total(?:\s+bayar)?|total pembayaran|transfer)[:\s]*Rp?[\s]*([0-9\.,]+)`,
		`(?i)Rp[\s]*([0-9\.,]+)`,
		`([0-9]{1,3}(?:[.,][0-9]{3})+)`,
		`([0-9]{5,})`,
	}

	var out []string
	seen := map[string]struct{}{}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		ms := re.FindAllStringSubmatch(text, -1)
		for _, m := range ms {
			if len(m) >= 2 {
				s := strings.TrimSpace(m[1])
				if s == "" {
					continue
				}
				if _, ok := seen[s]; ok {
					continue
				}
				// Apply plausibility filter to avoid picking up phone numbers,
				// RRNs and transaction ids that are numeric but not amounts.
				if !isPlausibleAmount(s) {
					// skip unlikely amount-looking matches
					seen[s] = struct{}{}
					continue
				}
				out = append(out, s)
				seen[s] = struct{}{}
			}
		}
	}
	return out, isLikelyNonAmount, nil
}

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
	// If it explicitly contains a currency marker, accept.
	if strings.Contains(low, "rp") || strings.Contains(low, "idr") {
		return true
	}
	// If it contains grouping separators '.' or ',' assume it's formatted as an amount
	// (e.g., 12.000 or 1,200,000). Accept unless it clearly looks like a date/phone.
	if strings.Contains(s, ".") || strings.Contains(s, ",") {
		// ensure there are at least 3 digits total
		digits := onlyDigits(s)
		if len(digits) >= 3 && digits[0] != '0' {
			return true
		}
		return false
	}
	// Pure digit cases: apply length and leading-zero heuristics.
	digits := onlyDigits(s)
	if digits == "" {
		return false
	}
	// Reject leading zeros (likely phone numbers or padded ids)
	if digits[0] == '0' {
		return false
	}
	// Reject very long digit-only strings (transaction ids / RRNs / phones)
	if len(digits) > 7 { // >7 digits likely not a typical payment amount
		return false
	}
	// Minimum reasonable amount is 2 digits (e.g., 10)
	if len(digits) < 2 {
		return false
	}
	return true
}

// onlyDigits helper used by isPlausibleAmount
func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// ParseAmountFromMatch normalizes a matched substring into an integer amount (whole currency units).
func ParseAmountFromMatch(found string) (int64, error) {
	centsRE := regexp.MustCompile(`[.,]\d{2}$`)
	foundTrim := strings.TrimSpace(found)
	onlyDigits := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
	}
	var digits string
	if centsRE.MatchString(foundTrim) {
		lastDot := strings.LastIndex(foundTrim, ".")
		lastComma := strings.LastIndex(foundTrim, ",")
		if lastComma > lastDot {
			integerPart := foundTrim[:lastComma]
			digits = onlyDigits(integerPart)
		} else if lastDot > lastComma {
			integerPart := foundTrim[:lastDot]
			digits = onlyDigits(integerPart)
		} else {
			digits = onlyDigits(foundTrim)
		}
	} else {
		digits = onlyDigits(foundTrim)
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
