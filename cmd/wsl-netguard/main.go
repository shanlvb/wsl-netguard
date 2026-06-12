package main

import (
	"io"
	"os"

	"github.com/shanlvb/wsl-netguard/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return cli.Run(args, stdout, stderr)
}
