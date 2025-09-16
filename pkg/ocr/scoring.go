package ocr

import "strings"

// BestAmountFromMatches selects the best amount using scoring priorities.
func BestAmountFromMatches(matches []string) (int64, string, bool) {
	type cand struct {
		amt   int64
		raw   string
		score int
	}
	scoreFor := func(raw string, amt int64) int {
		s := 0
		low := strings.ToLower(raw)
		if strings.Contains(low, "rp") || strings.Contains(low, "idr") {
			s += 10
		}
		if strings.Contains(low, "total") {
			s += 8
		} // boost TOTAL context
		if strings.Contains(raw, ".") || strings.Contains(raw, ",") {
			s += 5
		}
		if strings.HasSuffix(raw, ",00") || strings.HasSuffix(raw, ".00") {
			s += 3
		}
		if len(onlyDigits(raw)) >= 4 {
			s += 1
		}
		return s
	}
	cands := []cand{}
	for _, m := range matches {
		amt, err := ParseAmountFromMatch(m)
		if err != nil || amt <= 0 {
			continue
		}
		sc := scoreFor(m, amt)
		cands = append(cands, cand{amt: amt, raw: m, score: sc})
	}
	if len(cands) == 0 {
		return 0, "", false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		replace := false
		if c.score > best.score {
			replace = true
		} else if c.score == best.score {
			if c.amt > best.amt {
				replace = true
			} else if c.amt == best.amt {
				if len(c.raw) > len(best.raw) {
					replace = true
				} else if len(c.raw) == len(best.raw) && c.raw < best.raw {
					replace = true
				}
			}
		}
		if replace {
			best = c
		}
	}
	return best.amt, best.raw, true
}
