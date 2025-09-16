package ocr

import (
	"regexp"
	"strconv"
	"strings"
)

// fuzzyCurrencyAmount tries to reconstruct an amount near an Rp marker allowing OCR confusions.
func fuzzyCurrencyAmount(text string) (int64, string) {
	low := strings.ToLower(text)
	idx := strings.Index(low, "rp")
	if idx == -1 { return 0, "" }
	window := low[idx:]
	if len(window) > 120 { window = window[:120] }
	window = strings.Map(func(r rune) rune {
		switch r {
		case 'o','d': return '0'
		case 's': return '5'
		default: return r
		}
	}, window)
	re := regexp.MustCompile(`rp\s*([0-9oOdD]{1,3}(?:[.,][0-9oOdD]{3})+|[0-9oOdD]{5,9})`)
	m := re.FindStringSubmatch(window)
	if len(m) < 2 { return 0, "" }
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case 'o','O','d','D': return '0'
		default: return r
		}
	}, m[1])
	digits := onlyDigits(cleaned)
	if len(digits) < 3 || len(digits) > 9 { return 0, "" }
	amt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || amt <= 0 { return 0, "" }
	return amt, "Rp" + formatGrouping(digits)
}

// scanCurrencyNumbers tolerant scan of Rp amounts.
func scanCurrencyNumbers(text string) []string {
	low := strings.ToLower(text)
	repl := strings.NewReplacer("o","0","O","0","d","0","D","0","s","5")
	low = repl.Replace(low)
	re := regexp.MustCompile(`rp\s*([0-9]{1,3}(?:[.,][0-9]{3})+|[0-9]{5,9})`)
	ms := re.FindAllStringSubmatch(low, -1)
	out := []string{}
	seen := map[string]struct{}{}
	for _, m := range ms {
		if len(m) >= 2 {
			digits := onlyDigits(m[1])
			if digits == "" || len(digits) > 9 { continue }
			amt, err := strconv.ParseInt(digits, 10, 64)
			if err != nil || amt <= 0 { continue }
			norm := "Rp" + formatGrouping(digits)
			if _, ok := seen[norm]; !ok { out = append(out, norm); seen[norm] = struct{}{} }
		}
	}
	return out
}

// detectFlexibleCurrency finds spaced or noisy Rp digit sequences.
func detectFlexibleCurrency(text string) (int64, string) {
	low := strings.ToLower(text)
	rebuilt := strings.Join(strings.Fields(low), " ")
	re := regexp.MustCompile(`rp\s*([0-9\s.,]{5,15})`)
	m := re.FindStringSubmatch(rebuilt)
	if len(m) < 2 { return 0, "" }
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case 'o','O','d','D': return '0'
		case ' ': return -1
		default: return r
		}
	}, m[1])
	digits := onlyDigits(cleaned)
	if len(digits) < 5 || len(digits) > 9 { return 0, "" }
	amt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || amt <= 0 { return 0, "" }
	return amt, "Rp" + formatGrouping(digits)
}

// inferZeroAmountFromPattern infers Rp + digit + zeros pattern when spaced.
func inferZeroAmountFromPattern(text string) (int64, string) {
	low := strings.ToLower(text)
	idx := strings.Index(low, "rp")
	if idx == -1 { return 0, "" }
	window := low[idx:]
	if len(window) > 80 { window = window[:80] }
	window = strings.Join(strings.Fields(window), " ")
	re := regexp.MustCompile(`rp\s*([1-9])([0\s.,]{3,8})`)
	m := re.FindStringSubmatch(window)
	if len(m) < 3 { return 0, "" }
	lead, tail := m[1], m[2]
	zeros := strings.Count(tail, "0")
	if zeros < 3 { return 0, "" }
	if zeros > 6 { zeros = 6 }
	digits := lead + strings.Repeat("0", zeros)
	amt, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || amt <= 0 { return 0, "" }
	return amt, "Rp" + formatGrouping(digits)
}

// inferStandaloneZeroAmount guesses amount when Rp lost but clear zero block.
func inferStandaloneZeroAmount(text string) (int64, string) {
	norm := strings.ToLower(text)
	norm = strings.Join(strings.Fields(norm), " ")
	re := regexp.MustCompile(`(?:^|\s)([1-9])([0\s.,idrl]{4,12})(?:\s|$)`)
	ms := re.FindAllStringSubmatch(norm, -1)
	bestAmt := int64(0)
	bestRaw := ""
	for _, m := range ms {
		if len(m) < 3 { continue }
		lead, tail := m[1], m[2]
		zeros := strings.Count(tail, "0")
		if zeros < 4 { continue }
		if zeros > 6 { zeros = 6 }
		digits := lead + strings.Repeat("0", zeros)
		amt, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || amt <= 0 { continue }
		if !strings.HasSuffix(digits, "000") { continue }
		if amt > bestAmt { bestAmt = amt; bestRaw = "Rp" + formatGrouping(digits) + "?" }
	}
	if bestAmt > 0 { return bestAmt, bestRaw }
	return 0, ""
}
