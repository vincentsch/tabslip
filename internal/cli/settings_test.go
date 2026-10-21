package cli

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/rungrad/testutil"
)

func TestSelectedConfigFeedback(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.yaml")
	missing := filepath.Join(dir, "missing.yaml")
	if err := os.WriteFile(valid, []byte("port: 4598\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// All doctor upstream checks stay local; no provider traffic or credentials.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer fake.Close()
	old := doctorTransport
	doctorTransport = func() *http.Transport {
		return &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fake.URL, "http://"))
		}}
	}
	defer func() { doctorTransport = old }()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strings.Split(free.Addr().String(), ":")[1]
	free.Close()
	for _, tc := range []struct {
		name, env, flag string
		warn            bool
		port            string
	}{
		{"missing environment", missing, "", true, "4545"},
		{"missing flag overrides valid environment", valid, missing, true, "4545"},
		{"valid flag overrides missing environment", missing, valid, false, "4598"},
		{"valid environment", valid, "", false, "4598"},
		{"implicit absence", "", "", false, "4545"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TABSLIP_CONFIG", tc.env)
			for _, command := range []string{"", "serve", "doctor"} {
				for _, machine := range []bool{false, true} {
					args := []string{}
					if command != "" {
						args = append(args, command)
					}
					if tc.flag != "" {
						args = append(args, "--config", tc.flag)
					}
					if machine {
						args = append(args, "--json")
					}
					args = append(args, "--data-dir", filepath.Join(dir, "unused-data"))
					if command == "doctor" {
						args = append(args, "--port", port)
					} else {
						args = append(args, "--dry-run")
					}
					r := testutil.RunWith(NewApp(), testutil.Options{UserConfigDir: func() (string, error) { return dir, nil }}, args...)
					if r.Exit != 0 {
						t.Fatalf("%v: %+v", args, r)
					}
					if machine && !json.Valid([]byte(r.Stdout)) {
						t.Fatalf("invalid stdout: %+v", r)
					}
					if command == "doctor" {
						if r.Stderr != "" {
							t.Fatalf("duplicate doctor warning: %+v", r)
						}
						if machine {
							var checks []check
							if err := json.Unmarshal([]byte(r.Stdout), &checks); err != nil {
								t.Fatal(err)
							}
							want := "ok"
							if tc.warn {
								want = "warn"
							}
							if checks[0].Name != "config" || checks[0].Status != want || (tc.warn && checks[0].Fix == "") {
								t.Fatal(checks)
							}
						} else if tc.warn && !strings.Contains(r.Stdout, "[warn] config:") {
							t.Fatal(r)
						}
					} else {
						if !strings.Contains(r.Stdout, tc.port) {
							t.Fatal(r)
						}
						if tc.warn {
							if !strings.Contains(r.Stderr, "missing.yaml") || !strings.Contains(r.Stderr, "using defaults") || !strings.Contains(r.Stderr, "create the selected YAML") || strings.Count(strings.TrimSpace(r.Stderr), "\n") != 0 {
								t.Fatal(r)
							}
							if machine {
								var warning map[string]string
								if json.Unmarshal([]byte(r.Stderr), &warning) != nil || warning["warning"] == "" {
									t.Fatal(r)
								}
							} else if !strings.HasPrefix(r.Stderr, "Warning: ") {
								t.Fatal(r)
							}
						} else if r.Stderr != "" {
							t.Fatal(r)
						}
					}
				}
			}
		})
	}
	brokenData := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(brokenData, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	r := testutil.Run(NewApp(), "doctor", "--config", missing, "--data-dir", brokenData, "--port", port, "--json")
	if r.Exit != 2 || r.Stderr != "" || !strings.Contains(r.Stdout, `"status": "warn"`) || !strings.Contains(r.Stdout, `"status": "fail"`) {
		t.Fatal(r)
	}
	for _, path := range []string{missing, filepath.Join(dir, "unused-data"), filepath.Join(dir, "tabslip")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("created %s", path)
		}
	}
}

func TestReadAndDemoIgnoreSelectedSettings(t *testing.T) {
	dir, _ := seedReads(t)
	for _, malformed := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "selected.yaml")
		if malformed {
			if err := os.WriteFile(path, []byte("port: [broken"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("TABSLIP_CONFIG", path)
		for _, args := range [][]string{{"receipts", "--data-dir", dir, "--json"}, {"demo", "--dry-run", "--json"}} {
			r := testutil.Run(NewApp(), args...)
			if r.Exit != 0 || r.Stderr != "" || !json.Valid([]byte(r.Stdout)) {
				t.Fatal(r)
			}
		}
	}
}

func TestMissingConfigRedactsHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	r := testutil.Run(NewApp(), "serve", "--config", filepath.Join(home, "absent.yaml"), "--dry-run", "--json")
	if strings.Contains(r.Stderr, home) || !strings.Contains(r.Stderr, "absent.yaml") {
		t.Fatal(r)
	}
}
