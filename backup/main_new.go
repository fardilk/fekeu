// Package main is the legacy entrypoint kept for backwards compatibility.
// The actual application logic lives in cmd/api/main.go.
// This file simply delegates to the new structure.
//
// To run the API server:
//
//	go run ./cmd/api
//
// Or build:
//
//	go build -o bin/api ./cmd/api
package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	// Delegate to cmd/api by running it as a subprocess
	cmd := exec.Command("go", "run", "./cmd/api")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
