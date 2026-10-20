package main

import (
	"github.com/vincentsch/tollglass/internal/cli"
	"os"
)

func main() { os.Exit(cli.NewApp().Run(os.Args[1:], os.Stdout, os.Stderr)) }
