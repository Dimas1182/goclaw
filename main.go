package main

import (
	"os"

	"github.com/smallnest/goclaw/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
