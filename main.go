package main

import (
	"os"

	"github.com/amphoze/asaman/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
