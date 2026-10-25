package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Aufruf: pipeline-go <input> <output> <log.csv>")
		os.Exit(1)
	}

	exitCode := Run(os.Args[1], os.Args[2], os.Args[3], os.Stdout, os.Stderr)
	os.Exit(exitCode)
}
