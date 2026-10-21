package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readPages(t *testing.T, dir string) map[string]string {
	t.Helper()
	pages := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pages[entry.Name()] = string(body)
	}
	return pages
}

func TestReferenceIndependentOfDataDirectory(t *testing.T) {
	var first map[string]string
	for i := 0; i < 2; i++ {
		data := t.TempDir()
		t.Setenv("XDG_DATA_HOME", data)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(data, "config"))
		dir := t.TempDir()
		if err := run(false, dir); err != nil {
			t.Fatal(err)
		}
		pages := readPages(t, dir)
		if i == 0 {
			first = pages
		} else if !reflect.DeepEqual(first, pages) {
			t.Fatal("reference depends on XDG_DATA_HOME")
		}
		for name, body := range pages {
			if strings.Contains(body, data) {
				t.Errorf("%s contains local data path", name)
			}
		}
	}
	if !strings.Contains(first["index.md"], "--data-dir` Directory for local receipts") {
		t.Fatal("global flag usage is missing from reference")
	}
}

func TestCheckReportsDriftWithoutWriting(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "reference")
	handwritten := filepath.Join(parent, "intro.md")
	if err := os.WriteFile(handwritten, []byte("handwritten"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(true, dir); err == nil {
		t.Fatal("missing reference accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("check created reference directory")
	}
	if err := run(false, dir); err != nil {
		t.Fatal(err)
	}
	if err := run(true, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.md", "old.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "tabslip_serve.md")); err != nil {
		t.Fatal(err)
	}
	before := readPages(t, dir)
	if err := run(true, dir); err == nil || !strings.Contains(err.Error(), "orphaned: [old.md]") || !strings.Contains(err.Error(), "stale: [index.md]") || !strings.Contains(err.Error(), "missing: [tabslip_serve.md]") {
		t.Fatalf("unexpected drift result: %v", err)
	}
	if !reflect.DeepEqual(before, readPages(t, dir)) {
		t.Fatal("check changed reference files")
	}
	if err := run(false, dir); err != nil {
		t.Fatal(err)
	}
	if err := run(true, dir); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(handwritten); err != nil || string(body) != "handwritten" {
		t.Fatal("generation touched handwritten page")
	}
}

func TestTypedFlagsAndSharedLinks(t *testing.T) {
	pages := generate()
	for name, page := range pages {
		if name != "index.md" && !strings.Contains(page, "[global flags](index.md#global-flags)") {
			t.Errorf("%s has no shared flags link", name)
		}
	}
	for _, want := range []string{"`--port` Port to listen on. Type: `int`. Default: `4545`", "`--keep-content` Keep request and answer content locally. Type: `bool`. Default: `false`", "$XDG_CONFIG_HOME/tabslip/config.yaml", "%LocalAppData%", "~/.local/share/tabslip"} {
		if !strings.Contains(pages["index.md"], want) {
			t.Errorf("missing global flag detail: %s", want)
		}
	}
	if !strings.Contains(pages["tabslip_receipts.md"], "Type: `int`. Default: `20`") {
		t.Fatal("local limit metadata missing")
	}
	if !strings.Contains(pages["tabslip_key_export.md"], "Type: `string`. Default: unset (empty)") {
		t.Fatal("nested flag default missing")
	}
}
