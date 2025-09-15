OCR Module Structure

Files:
- ocr.go: Public entry points (ExtractAmountFromImage, FindAllMatches) and ribu helper.
- preprocess.go: Image preprocessing primitives (binarize, adaptiveThreshold, dilate).
- passes.go: Orchestrates multi-pass Tesseract OCR producing variant texts.
- parsing.go: ParseAmountFromMatch logic (decimal stripping).
- plausibility.go: Heuristics for plausible amount detection.
- scoring.go: BestAmountFromMatches scoring (currency, TOTAL boost, formatting).
- inference.go: Fuzzy / flexible pattern and zero-block inference helpers.
- util.go: Small generic helpers (snippet, normalizeOCRText, formatGrouping).
- errors.go: ErrNoAmount sentinel.

Selection rules encoded:
1. Prefer lines with currency markers (Rp/IDR) and TOTAL context.
2. Strip trailing decimal fractions (",00" / ".00") to whole units.
3. If multiple remain, choose highest score then largest amount.
4. Fallback patterns: 'ribu' (thousand), zero-block inference when no direct markers.
5. If none found, return ErrNoAmount.

Tests cover: decimal stripping, TOTAL prioritization, ErrNoAmount on blank image.
