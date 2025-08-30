package main

import (
	"flag"
	"fmt"
	"os"

	"be03/process/report"
)

func main() {
	username := flag.String("username", "fardiluser", "username to report for")
	month := flag.String("month", "2025-08", "month to report (YYYY-MM)")
	list := flag.Bool("list", false, "list matching rows")
	flag.Parse()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DB_DSN not set; export DB_DSN and retry")
		os.Exit(2)
	}

	report.RunReport(*username, *month, *list)
}
