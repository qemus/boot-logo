package main

import (
	"fmt"
	"io"
	"os"
)

var (
	Version = "0.0"
	stdout  io.Writer = os.Stdout
	stderr  io.Writer = os.Stderr
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
