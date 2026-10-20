package main

import (
	"os"

	"github.com/alhafoudh/sharryctl/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
