package main

import (
	"be03/pkg/ocr"
	"fmt"
	"os"
)

func main() {
	p := "public/processed/3749238.png"
	if len(os.Args) > 1 {
		p = os.Args[1]
	}
	matches, nonAmount, err := ocr.FindAllMatches(p)
	fmt.Printf("FindAllMatches err=%v nonAmount=%v\n", err, nonAmount)
	fmt.Printf("matches=%#v\n", matches)
	amt, conf, found, err := ocr.ExtractAmountFromImage(p)
	fmt.Printf("ExtractAmountFromImage err=%v\n", err)
	fmt.Printf("amt=%d conf=%.3f found=%q\n", amt, conf, found)
}
