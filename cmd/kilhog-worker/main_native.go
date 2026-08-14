//go:build !(js && wasm)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "kilhog-worker must be built with GOOS=js GOARCH=wasm (use: make build-wasm)")
	os.Exit(1)
}
