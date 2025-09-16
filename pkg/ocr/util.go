package ocr

import "strings"

// snippet returns a shortened version of text (ASCII only) for logging.
func snippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// normalizeOCRText collapses whitespace and replaces newlines/tabs.
func normalizeOCRText(t string) string {
	t = strings.ReplaceAll(t, "\n", " ")
	t = strings.ReplaceAll(t, "\t", " ")
	return strings.Join(strings.Fields(t), " ")
}

// formatGrouping adds dot separators every 3 digits.
func formatGrouping(ds string) string {
	n := len(ds)
	if n <= 3 {
		return ds
	}
	var parts []string
	for n > 3 {
		parts = append([]string{ds[n-3:]}, parts...)
		ds = ds[:n-3]
		n = len(ds)
	}
	parts = append([]string{ds}, parts...)
	return strings.Join(parts, ".")
}
