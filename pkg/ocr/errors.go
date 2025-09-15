package ocr

import "errors"

// ErrNoAmount is returned when no plausible monetary amount can be extracted.
var ErrNoAmount = errors.New("no amount detected")
