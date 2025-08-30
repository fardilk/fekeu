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
