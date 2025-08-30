package main

import (
	"fmt"
	"log"
	"os"

	"be03/pkg/ocr"

	"github.com/disintegration/imaging"
)

func main() {
	in := "public/keu/399482392.png"
	img, err := imaging.Open(in)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	proc := imaging.Sharpen(img, 2.0)
	proc = imaging.AdjustContrast(proc, 30)
	tmp := "/tmp/399482392.retry.png"
	if err := imaging.Save(proc, tmp); err != nil {
		log.Fatalf("save tmp: %v", err)
	}
	amt, conf, found, err := ocr.ExtractAmountFromImage(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		log.Fatalf("ocr err: %v", err)
	}
	fmt.Printf("after-preproc amt=%d conf=%.4f found=%q\n", amt, conf, found)
}
