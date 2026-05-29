// Command siphon is the CLI entry point. All real work lives in
// internal/cli; this file exists only to satisfy the Go executable layout.
package main

import (
	"os"

	"github.com/nixrajput/siphon/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
