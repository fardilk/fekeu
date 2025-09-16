package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"be03/pkg/ocr"
)

func main() {
	img := flag.String("img", "tmp/test.png", "image file to run OCR on")
	flag.Parse()
	p, _ := filepath.Abs(*img)
	fmt.Printf("Running OCR on %s\n", p)
	matches, isLikelyNonAmount, err := ocr.FindAllMatches(p)
	if err != nil {
		log.Fatalf("FindAllMatches error: %v", err)
	}
	fmt.Printf("isLikelyNonAmount=%v, matches=%v\n", isLikelyNonAmount, matches)
	amt, conf, found, err := ocr.ExtractAmountFromImage(p)
	if err != nil {
		log.Fatalf("ExtractAmountFromImage error: %v", err)
	}
	fmt.Printf("ExtractAmountFromImage -> amount=%d conf=%.2f found=%q\n", amt, conf, found)
}
