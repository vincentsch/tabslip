// Command generate-command-docs writes or checks the generated command reference.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vincentsch/rungrad/docsgen"
	"github.com/vincentsch/tabslip/internal/cli"
)

func run(check bool, dir string) error {
	app := cli.NewApp()
	if !check {
		return docsgen.Write(app, dir)
	}
	result, err := docsgen.Check(app, dir)
	if err != nil {
		return err
	}
	if !result.OK() {
		return fmt.Errorf("command reference is stale (missing: %v; stale: %v; orphaned: %v); run make docs", result.Missing, result.Stale, result.Orphaned)
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "check reference files without changing them")
	flag.Parse()
	if err := run(*check, "docs/reference"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
