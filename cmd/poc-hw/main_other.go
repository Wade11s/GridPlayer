//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "poc-hw: macOS/arm64 only")
	os.Exit(2)
}
