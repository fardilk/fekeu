package main

import (
	"flag"
	"fmt"
	"log"

	"be03/pkg/ocr"
)

func main() {
	f := flag.String("file", "", "image file to OCR")
	flag.Parse()
	if *f == "" {
		log.Fatalf("-file required")
	}
	amt, conf, found, err := ocr.ExtractAmountFromImage(*f)
	if err != nil {
		log.Fatalf("ocr error: %v", err)
	}
	fmt.Printf("amt=%d conf=%.4f found=%q\n", amt, conf, found)
}
