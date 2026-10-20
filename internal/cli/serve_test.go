package cli

import (
	"encoding/json"
	"fmt"
	"github.com/vincentsch/rungrad/testutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestDryRunConfigPrecedenceAndUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("port: 4598\nkeep_content: true\n"), 0600)
	t.Setenv("TOLLGLASS_CONFIG", path)
	data := filepath.Join(dir, "data")
	for _, args := range [][]string{{"--dry-run", "--json", "--data-dir", data}, {"serve", "--dry-run", "--json", "--data-dir", data}} {
		r := testutil.Run(NewApp(), args...)
		if r.Exit != 0 || !strings.Contains(r.Stdout, "4598") || !strings.Contains(r.Stdout, `"content": "true"`) {
			t.Fatalf("config: %+v", r)
		}
	}
	r := testutil.Run(NewApp(), "serve", "--dry-run", "--json", "--port", "4599", "--keep-content=false", "--data-dir", data)
	if r.Exit != 0 || !strings.Contains(r.Stdout, "4599") || !strings.Contains(r.Stdout, `"content": "false"`) {
		t.Fatalf("flags: %+v", r)
	}
	if _, e := os.Stat(data); !os.IsNotExist(e) {
		t.Fatal("dry-run created data")
	}
	os.WriteFile(path, []byte("port: 70000\n"), 0600)
	r = testutil.Run(NewApp(), "serve", "--dry-run", "--json", "--port", "4599")
	if r.Exit != 0 {
		t.Fatalf("flag did not override file setting: %+v", r)
	}
	missing := filepath.Join(dir, "missing.yaml")
	r = testutil.Run(NewApp(), "serve", "--dry-run", "--json", "--config", missing)
	if r.Exit != 0 || !strings.Contains(r.Stdout, "4545") {
		t.Fatal("explicit config precedence")
	}
	t.Setenv("TOLLGLASS_CONFIG", "")
	os.MkdirAll(filepath.Join(dir, "tollglass"), 0700)
	os.WriteFile(filepath.Join(dir, "tollglass", "config.yaml"), []byte("port: 4597"), 0600)
	r = testutil.RunWith(NewApp(), testutil.Options{
		UserConfigDir: func() (string, error) { return dir, nil },
	}, "--dry-run", "--json")
	if r.Exit != 0 || !strings.Contains(r.Stdout, "4597") {
		t.Fatalf("OS config: %+v", r)
	}
	os.WriteFile(path, []byte("port: [broken"), 0600)
	r = testutil.Run(NewApp(), "serve", "--config", path, "--dry-run")
	if r.Exit != 1 || !strings.Contains(r.Stderr, "malformed") {
		t.Fatal("malformed YAML accepted")
	}
	r = testutil.Run(NewApp(), "this-subcommand-does-not-exist")
	if r.Exit != 1 {
		t.Fatal("unknown command started server")
	}
	r = testutil.Run(NewApp(), "--version")
	if r.Exit != 0 || !strings.Contains(r.Stdout, "commit") || !strings.Contains(r.Stdout, "go1.") {
		t.Fatal("version metadata")
	}
}
func TestBusyPort(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	p := ln.Addr().(*net.TCPAddr).Port
	r := testutil.Run(NewApp(), "serve", "--port", strconv.Itoa(p), "--data-dir", filepath.Join(t.TempDir(), "data"))
	if r.Exit != 2 || !strings.Contains(r.Stderr, "in use by another program") || !strings.Contains(r.Stderr, "--port") {
		t.Fatalf("busy port: %+v", r)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tool": "tollglass"})
	}))
	defer fake.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(fake.URL, "http://"))
	n, _ := strconv.Atoi(port)
	if !strings.Contains(busyMessage(n), "already running at "+fake.URL) {
		t.Fatal("existing Tollglass not identified")
	}
}
func TestHelpAndManifest(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"serve", "--help"}, {"__rungrad_manifest"}} {
		r := testutil.Run(NewApp(), args...)
		if r.Exit != 0 || !strings.Contains(r.Stdout, "tollglass") {
			t.Fatalf("help/manifest: %+v", r)
		}
	}
}

func TestCustomUpstreamsCannotShadowLocalPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := filepath.Join(dir, "data")
	for _, name := range []string{"setup", "spend", "r", "events", "settings"} {
		t.Run(name, func(t *testing.T) {
			body := fmt.Sprintf("upstreams:\n  - name: %s\n    base_url: http://127.0.0.1:4598\n    api: openai\n", name)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			r := testutil.Run(NewApp(), "serve", "--config", path, "--data-dir", data, "--dry-run")
			if r.Exit != 1 || !strings.Contains(r.Stderr, "reserved for the local page") || !strings.Contains(r.Stderr, name) {
				t.Fatalf("route collision accepted or unclear diagnostic: %+v", r)
			}
		})
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatal("invalid config created instance state")
	}
}

func TestAddressInUseErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"address in use", &net.OpError{Op: "listen", Err: &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}}, true},
		{"Windows socket address in use", &net.OpError{Op: "listen", Err: &os.SyscallError{Syscall: "bind", Err: syscall.Errno(10048)}}, runtime.GOOS == "windows"},
		{"permission denied", syscall.EACCES, false},
		{"other error", fmt.Errorf("cannot bind"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAddressInUse(tc.err); got != tc.want {
				t.Fatalf("isAddressInUse=%t, want %t", got, tc.want)
			}
		})
	}
}

func TestDemoDryRunIgnoresNormalConfigAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	os.WriteFile(path, []byte("port: [broken"), 0600)
	t.Setenv("TOLLGLASS_CONFIG", path)
	data := filepath.Join(dir, "demo-data")
	r := testutil.Run(NewApp(), "demo", "--dry-run", "--json", "--data-dir", data)
	if r.Exit != 0 || !strings.Contains(r.Stdout, "eight simulated calls") {
		t.Fatalf("demo dry run: %+v", r)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatal("dry run created state")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "port: [broken" {
		t.Fatal("normal config changed")
	}
}

func TestBrowserOpenIsExplicitAndUsesConfiguredURL(t *testing.T) {
	original := browserStart
	defer func() { browserStart = original }()
	var command string
	var args []string
	browserStart = func(name string, a ...string) error { command = name; args = a; return nil }
	r := testutil.Run(NewApp(), "serve", "--open", "--dry-run", "--json", "--port", "4599", "--data-dir", filepath.Join(t.TempDir(), "data"))
	if r.Exit != 0 || command != "" {
		t.Fatal("dry-run launched browser", r)
	}
	if err := openBrowser("http://127.0.0.1:4599"); err != nil || command == "" || len(args) == 0 || args[len(args)-1] != "http://127.0.0.1:4599" {
		t.Fatal("configured URL lost", command, args, err)
	}
}
