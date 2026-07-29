package main

import (
	"os"

	"github.com/kilhog-io/kilhog/cmd/pogig/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
