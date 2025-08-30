package main

import (
	"be03/pkg/ocr"
	"fmt"
	"log"
)

func main() {
	amt, conf, found, err := ocr.ExtractAmountFromImage("public/keu/399482392.png")
	if err != nil {
		log.Printf("ocr error: %v", err)
		return
	}
	fmt.Printf("amt=%d conf=%.4f found=%q\n", amt, conf, found)
}
