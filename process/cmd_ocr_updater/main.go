package main

import (
	"flag"
	"fmt"
	"os"

	ocrupdater "be03/process/ocr_updater"
)

func main() {
	dir := flag.String("dir", "public/keu", "directory to scan for images")
	dry := flag.Bool("dry-run", true, "dry-run: don't write to DB")
	minConf := flag.Float64("min-conf", 0.12, "minimum OCR confidence to accept")
	flag.Parse()

	if os.Getenv("DB_DSN") == "" {
		fmt.Fprintln(os.Stderr, "DB_DSN not set; export and retry")
		os.Exit(2)
	}

	if err := ocrupdater.Run(*dir, *dry, *minConf); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
}
