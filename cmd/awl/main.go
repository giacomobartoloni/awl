package main

import (
	"os"

	"github.com/giacomobartoloni/awl/internal/awl"
)

func main() { os.Exit(awl.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
