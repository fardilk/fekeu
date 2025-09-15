package ocr

import (
	"fmt"
	"log"
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
	variants, err := runAllOCRPasses(path)
	if err != nil {
		return 0, 0, "", fmt.Errorf("ocr passes: %w", err)
	}
	matches, _, err := FindAllMatches(path)
	if err != nil {
		return 0, 0, "", err
	}
	text := variants["text"]
	textDigits := variants["textDigits"]
	textOrig := variants["textOrig"]
	allText := variants["aggregate"]

	// Attempt inference of amount made of a leading digit + zeros (possibly spaced) when Rp context exists.
	if infAmt, infRaw := inferZeroAmountFromPattern(allText); infAmt > 0 {
		matches = append(matches, infRaw)
	}

	// Flexible spaced currency detection (e.g., "Rp6 0 0 . 0 0 0")
	if flexAmt, flexRaw := detectFlexibleCurrency(allText); flexAmt > 0 {
		matches = append(matches, flexRaw)
	}

	// Extra direct scan: try to capture a currency-marked amount line from raw OCR text
	// even if FindAllMatches missed it (e.g. formatting noise). This looks for Rp followed
	// by grouped or plain digits up to 9 digits.
	var directCurrency string
	if idx := strings.Index(strings.ToLower(text), "rp"); idx != -1 {
		seg := text[idx:]
		reRp := regexp.MustCompile(`(?i)^rp\s*([0-9]{1,3}(?:[.,][0-9]{3})+|[0-9]{3,9})`)
		if m := reRp.FindStringSubmatch(strings.ToLower(seg)); len(m) >= 2 {
			cand := m[1]
			if !strings.HasPrefix(cand, "rp") {
				cand = "Rp" + cand
			}
			// ensure not already in matches
			dup := false
			for _, ex := range matches {
				if strings.EqualFold(ex, cand) {
					dup = true
					break
				}
			}
			if !dup {
				matches = append(matches, cand)
				directCurrency = cand
			}
		}
	}
	// Robust scan across combined texts for any currency number patterns not already captured.
	extraCurrency := scanCurrencyNumbers(allText)
	for _, ec := range extraCurrency {
		already := false
		for _, m := range matches {
			if strings.EqualFold(m, ec) {
				already = true
				break
			}
		}
		if !already {
			matches = append(matches, ec)
		}
	}

	if len(matches) == 0 {
		// Before returning, attempt a 'ribu' (thousand) pattern extraction e.g. "400 ribu" or "400ribu".
		if amt, raw := extractRibu(text); amt > 0 {
			return amt, 0.5, raw, nil
		}
		// New: attempt zero-block inference without explicit Rp when other signals (e.g. many zeros) present.
		if zAmt, zRaw := inferStandaloneZeroAmount(allText); zAmt > 0 {
			log.Printf("OCR fallback zero-block inferred %d raw=%s", zAmt, zRaw)
			return zAmt, 0.35, zRaw, nil
		} else {
			log.Printf("OCR fallback zero-block inference failed; text snippet=%q", snippet(allText, 140))
		}
		return 0, 0, "", ErrNoAmount
	}
	if amt, raw, ok := BestAmountFromMatches(matches); ok {
		// Fuzzy reconstruction: attempt to parse an amount near an Rp marker even if OCR mangled digits.
		if fAmt, fRaw := fuzzyCurrencyAmount(text + " " + textDigits + " " + textOrig); fAmt > 0 {
			// Prefer fuzzy if original raw lacks currency hints OR fuzzy differs materially.
			rawLow := strings.ToLower(raw)
			if !(strings.Contains(rawLow, "rp") || strings.Contains(rawLow, "idr")) || fAmt != amt {
				amt = fAmt
				raw = fRaw
			}
		}
		fAmtLog, fRawLog := fuzzyCurrencyAmount(text + " " + textDigits + " " + textOrig)
		if fAmtLog > 0 {
			log.Printf("OCR debug: raw_text_snippet=%q candidates=%v directAdded=%s fuzzy_recon=%d/%s chosen_raw=%s chosen_amt=%d", snippet(text, 160), matches, directCurrency, fAmtLog, fRawLog, raw, amt)
		} else {
			log.Printf("OCR debug: raw_text_snippet=%q candidates=%v directAdded=%s fuzzy_recon=none chosen_raw=%s chosen_amt=%d", snippet(text, 160), matches, directCurrency, raw, amt)
		}
		// Confidence proxy based on substring length vs OCR text size
		conf := float64(len(raw)) / float64(len(text)+1)
		if conf > 1 {
			conf = 1
		}
		if amt < 0 {
			amt = -amt
		}
		// Boost confidence if explicit currency or trailing .00/.00 detected
		lowRaw := strings.ToLower(raw)
		if strings.Contains(lowRaw, "rp") || strings.Contains(lowRaw, "idr") || strings.HasSuffix(lowRaw, ",00") || strings.HasSuffix(lowRaw, ".00") {
			if conf < 0.85 {
				conf = 0.85
			}
		}

		// Heuristic: when the OCR text contains a currency context (Rp/IDR),
		// but the chosen raw match has no separators or currency hints itself,
		// and it's very close to a clean thousand boundary (e.g. 250903),
		// floor to the nearest thousand. This addresses common OCR artifacts
		// where separators/decimals are misread as stray digits.
		lowText := strings.ToLower(text)
		hasCurrencyCtx := strings.Contains(lowText, "rp") || strings.Contains(lowText, "idr")
		rawLow := strings.ToLower(raw)
		rawHasHints := strings.Contains(rawLow, "rp") || strings.Contains(rawLow, "idr") || strings.Contains(raw, ".") || strings.Contains(raw, ",")
		if hasCurrencyCtx && !rawHasHints && amt >= 1000 {
			rem := amt % 1000
			// Tighter threshold to avoid flooring legitimate 6-digit grouped values misread.
			if rem <= 20 || rem >= 980 {
				amt = amt - rem
			}
		}
		return amt, conf, raw, nil
	}
	// Fallback: attempt 'ribu' pattern if numeric matches didn't yield a best amount.
	if amt, raw := extractRibu(text); amt > 0 {
		return amt, 0.4, raw, nil
	}
	return 0, 0, "", ErrNoAmount
}

// extractRibu finds patterns like "400 ribu", "400ribu", "400 RIBU" meaning 400 * 1000.
// Returns (amount, raw) or (0, "") if not found / invalid.
func extractRibu(text string) (int64, string) {
	low := strings.ToLower(text)
	// Accept optional space, punctuation, or colon between number and 'ribu'
	re := regexp.MustCompile(`(?i)\b([1-9][0-9]{0,3})\s*[,.:;-]?\s*ribu\b`)
	m := re.FindStringSubmatch(low)
	if len(m) >= 2 {
		digits := m[1]
		n, err := strconv.ParseInt(digits, 10, 64)
		if err == nil && n > 0 {
			// Guard against unrealistic scaling (limit to 9,999 ribu => 9,999,000)
			if n <= 9999 {
				return n * 1000, m[0]
			}
		}
	}
	// Also handle concatenated form like '400ribu' that may lack boundary due to OCR noise.
	re2 := regexp.MustCompile(`(?i)\b([1-9][0-9]{0,3})ribu\b`)
	m2 := re2.FindStringSubmatch(low)
	if len(m2) >= 2 {
		digits := m2[1]
		n, err := strconv.ParseInt(digits, 10, 64)
		if err == nil && n > 0 && n <= 9999 {
			return n * 1000, m2[0]
		}
	}
	return 0, ""
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
	_ = client.SetWhitelist("0123456789RpIDRidri.,:()/- ")
	client.SetImage(tmp)
	text, err := client.Text()
	if tmp != path {
		_ = os.Remove(tmp)
	}
	if err != nil {
		return nil, false, fmt.Errorf("ocr error: %w", err)
	}
	// Preserve the raw OCR text before normalization for later flexible detection/inference.
	originalText := text
	text = normalizeOCRText(text)
	log.Printf("OCR RAW %s snippet=%q", path, snippet(text, 180))

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
		`(?i)(?:jumlah(?:\s+transfer)?|total(?:\s+bayar)?|total pembayaran|transfer)[:\s]*(?:Rp|IDR)?[\s]*([0-9\.,]+)`,
		`(?i)Rp[\s]*([0-9\.,]+)`,
		`(?i)IDR[\s]*([0-9\.,]+)`,
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
				// If the full match contained a currency marker (Rp/IDR) but the captured group
				// stripped it away, re-associate the marker so downstream scoring logic can
				// prioritize it. This fixes cases like "Rp600.000" vs a raw long digit match
				// (e.g. transaction id fragment) where previously only "600.000" (no Rp) was
				// scored, reducing its priority.
				if s != "" {
					full := strings.ToLower(m[0])
					lowS := strings.ToLower(s)
					if (strings.Contains(full, "rp") || strings.Contains(full, "idr")) && !strings.Contains(lowS, "rp") && !strings.Contains(lowS, "idr") {
						// Prepend a normalized marker (choose Rp) to retain context; formatting characters
						// (spaces) are not critical for parsing but presence of marker boosts score.
						s = "Rp" + s
					}
				}
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
			}
		}
	}

	// Additional pass: find numbers that follow a nearby "Rp" or "IDR" marker that
	// the main patterns missed. This helps when OCR inserts non-printable chars or
	// when the primary regex failed due to spacing/artifacts.
	lowText := strings.ToLower(text)
	for _, marker := range []string{"rp", "idr"} {
		searchIdx := 0
		for {
			i := strings.Index(lowText[searchIdx:], marker)
			if i == -1 {
				break
			}
			pos := searchIdx + i
			tail := text[pos+len(marker):]
			reNum := regexp.MustCompile(`\s*[:\-\s]*\s*([0-9]{1,3}(?:[.,][0-9]{3})+|[0-9]{3,7})`)
			m := reNum.FindStringSubmatch(tail)
			if len(m) >= 2 {
				candidate := strings.TrimSpace(m[1])
				candWithMarker := "Rp" + candidate
				if _, ok := seen[candWithMarker]; !ok && isPlausibleAmount(candWithMarker) {
					out = append(out, candWithMarker)
					seen[candWithMarker] = struct{}{}
				}
			}
			searchIdx = pos + len(marker)
		}
	}
	// Final extra scan for reconstructed currency patterns
	extra := scanCurrencyNumbers(text)
	for _, ec := range extra {
		if _, ok := seen[ec]; !ok {
			out = append(out, ec)
			seen[ec] = struct{}{}
		}
	}

	// Flexible currency pattern detection over combined raw + normalized text.
	if flexAmt, flexRaw := detectFlexibleCurrency(originalText + " " + text); flexAmt > 0 {
		if _, ok := seen[flexRaw]; !ok {
			out = append(out, flexRaw)
			seen[flexRaw] = struct{}{}
		}
	}
	// Zero-pattern inference (e.g., infer 600000 from context like '600 000').
	if infAmt, infRaw := inferZeroAmountFromPattern(originalText + " " + text); infAmt > 0 {
		if _, ok := seen[infRaw]; !ok {
			out = append(out, infRaw)
			seen[infRaw] = struct{}{}
		}
	}
	return out, isLikelyNonAmount, nil
}

// isPlausibleAmount applies lightweight heuristics to decide whether a
// matched numeric substring likely represents a monetary amount rather than
// a phone number, transaction id, or RRN. The heuristics are intentionally
// conservative: prefer strings that include currency hints or grouping
// separators, and reject very long digit-only strings or those starting with 0.
// isPlausibleAmount and onlyDigits moved to plausibility.go

// ParseAmountFromMatch normalizes a matched substring into an integer amount (whole currency units).
// ParseAmountFromMatch moved to parsing.go

// BestAmountFromMatches moved to scoring.go

// snippet returns a shortened version of text (ASCII only) for logging.
// snippet, normalizeOCRText moved to util.go

// fuzzyCurrencyAmount tries to reconstruct an amount like 600000 or 600.000
// near an Rp marker even if OCR produced confusing characters (O->0, etc).
// Returns (amount, raw) else (0, "").
// fuzzyCurrencyAmount moved to inference.go

// scanCurrencyNumbers finds all Rp/IDR amounts by tolerant scanning (ignoring noise) and returns normalized list (with Rp prefix).
// scanCurrencyNumbers moved to inference.go

// detectFlexibleCurrency detects patterns like "Rp6 0 0 . 0 0 0" or "Rp 6 0 0 0 0 0".
// detectFlexibleCurrency moved to inference.go

// inferZeroAmountFromPattern tries to infer an amount like 600000 from patterns such as
// "rp 6 0 0 0 0 0" or "rp 6 0 0 . 0 0 0" where OCR separated digits.
// inferZeroAmountFromPattern moved to inference.go

// formatGrouping adds dot separators every 3 digits for logging/raw presentation.
// formatGrouping moved to util.go

// inferStandaloneZeroAmount attempts to infer an amount like 600000 when OCR loses the 'Rp'
// marker entirely but there is a clear pattern of one leading non-zero digit followed by
// >=4 zeros (possibly spaced or punctuated) and NO other plausible matches were found.
// It is deliberately conservative: refuses patterns embedded inside longer digit runs,
// and caps length at 7 digits to avoid picking large ids.
// inferStandaloneZeroAmount moved to inference.go
