package main

import (
	"be03/pkg/ocr"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	path := flag.String("path", "", "image path")
	flag.Parse()
	if *path == "" {
		log.Fatal("--path is required")
	}
	// Re-run OCR raw for debugging
	rawText, _ := os.ReadFile(*path) // placeholder (not reading image text) but keep structure
	matches, nonAmount, err := ocr.FindAllMatches(*path)
	if err != nil {
		log.Fatalf("ocr error: %v", err)
	}
	fmt.Printf("nonAmount=%v matches=%v\n", nonAmount, matches)
	_ = rawText // silence unused for now
	fmt.Printf("NOTE: raw text printed from library via server logs if upload path used.\n")
	if len(matches) > 0 {
		amt, raw, ok := ocr.BestAmountFromMatches(matches)
		fmt.Printf("best ok=%v amt=%d raw=%q\n", ok, amt, raw)
	}
	// crude manual fuzzy fallback: look for 'rp' tokens inside FindAllMatches text logic is already there.
	fmt.Println(strings.Repeat("-", 50))
}
