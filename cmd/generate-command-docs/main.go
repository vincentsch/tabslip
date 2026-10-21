// Command generate-command-docs writes or checks the generated command reference.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vincentsch/rungrad/docsgen"
	"github.com/vincentsch/tabslip/internal/cli"
)

// Extend rungrad's reference with typed defaults from the same command tree.
func generate() map[string]string {
	app := cli.NewApp()
	pages := docsgen.Generate(app)
	root := app.Root()
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
		if page, ok := pages[path]; ok {
			cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { page = describeFlag(page, f) })
			pages[path] = strings.TrimRight(page, "\n") + "\n\n## Shared flags\n\nSee [global flags](index.md#global-flags) for shared options, types and defaults.\n"
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) { pages["index.md"] = describeFlag(pages["index.md"], f) })
	return pages
}

func describeFlag(page string, f *pflag.Flag) string {
	prefix := "- `--" + f.Name + "` " + f.Usage
	def := "`" + f.DefValue + "`"
	if f.DefValue == "" {
		def = "unset (empty)"
	}
	switch f.Name {
	case "config":
		def = "`TABSLIP_CONFIG`, then the OS config directory: Linux `$XDG_CONFIG_HOME/tabslip/config.yaml` or `~/.config/tabslip/config.yaml`; macOS `~/Library/Application Support/tabslip/config.yaml`; Windows `%AppData%\\tabslip\\config.yaml`"
	case "data-dir":
		def = "Linux `$XDG_DATA_HOME/tabslip` or `~/.local/share/tabslip`; macOS `~/Library/Application Support/tabslip`; Windows `%LocalAppData%\\tabslip`"
	}
	return strings.ReplaceAll(page, prefix+"\n", prefix+". Type: `"+f.Value.Type()+"`. Default: "+def+".\n")
}

func run(check bool, dir string) error {
	want := generate()
	result := docsgen.CheckResult{}
	for path, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			result.Missing = append(result.Missing, path)
		case err != nil:
			return err
		case string(got) != content:
			result.Stale = append(result.Stale, path)
		}
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := want[rel]; !ok {
			result.Orphaned = append(result.Orphaned, rel)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	sort.Strings(result.Missing)
	sort.Strings(result.Stale)
	sort.Strings(result.Orphaned)
	if check {
		if !result.OK() {
			return fmt.Errorf("command reference is stale (missing: %v; stale: %v; orphaned: %v); run make docs", result.Missing, result.Stale, result.Orphaned)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for path, content := range want {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0644); err != nil {
			return err
		}
	}
	for _, path := range result.Orphaned {
		if err := os.Remove(filepath.Join(dir, path)); err != nil {
			return err
		}
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
