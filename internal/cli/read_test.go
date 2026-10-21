package cli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/rungrad/testutil"
	"github.com/vincentsch/tabslip/internal/config"
	"github.com/vincentsch/tabslip/internal/keys"
	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/store"
	"github.com/vincentsch/tabslip/internal/upstream"
)

func ptr[T any](v T) *T { return &v }
func seedReads(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := receipt.New(db.KeyID, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	r.ID = "rcpt_01JB8Y3Q6V9W2N4K8M0P5R7S1T"
	r.Request.Upstream = "openrouter"
	r.Request.API = "openai.chat"
	r.Request.Tag = "acme-shop"
	r.Request.Model = ptr("vendor/requested-long-model-name/with-more-segments-and-words")
	r.Response.Model = ptr("<script>alert(1)</script>")
	r.Response.Provider = ptr("Known provider")
	r.Response.Status = ptr(200)
	r.Usage.Input = ptr(int64(12))
	r.Time.Total = 123
	r.Find("price_unknown", "observed", "The cost of this call is unknown.")
	db.Save(r, &store.Content{Request: []byte("PRIVATE PROMPT"), Response: []byte("PRIVATE ANSWER")})
	db.Flush()
	b, _ := receipt.Sign(r, db.Key)
	var e receipt.Envelope
	json.Unmarshal(b, &e)
	p, _ := base64.StdEncoding.DecodeString(e.Payload)
	s := &receipt.Supplement{Type: "tabslip.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(p), Source: "openrouter.generation", Cost: ptr("0.100000000000000001"), Currency: "USD", DataRegion: ptr("eu"), Attempts: []receipt.Attempt{{Provider: ptr("Known provider"), Status: ptr(200)}}}
	db.SaveSupplement(s)
	db.Flush()
	other := receipt.New(db.KeyID, time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	other.Request.Model = ptr("equal")
	other.Response.Model = ptr("equal")
	other.Request.Upstream = "openai"
	db.Save(other, nil)
	db.Flush()
	db.Close()
	return dir, r.ID
}
func runRead(t *testing.T, dir string, args ...string) testutil.Result {
	t.Helper()
	return testutil.Run(NewApp(), append(args, "--data-dir", dir, "--config", filepath.Join(dir, "absent.yaml"))...)
}
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch %s\ngot:\n%s", name, got)
	}
}
func TestReadFiltersAndGoldenOutput(t *testing.T) {
	dir, id := seedReads(t)
	for _, m := range []string{"vendor/requested-long-model-name/with-more-segments-and-words", "<script>alert(1)</script>"} {
		r := runRead(t, dir, "receipts", "--model", m, "--tag", "acme-shop", "--upstream", "openrouter", "--since", "2026-10-20", "--until", "2026-10-21", "--json")
		if r.Exit != 0 {
			t.Fatal(r)
		}
		golden(t, "receipts.json", r.Stdout)
		var summaries []store.Summary
		if json.Unmarshal([]byte(r.Stdout), &summaries) != nil || len(summaries) != 1 || summaries[0].Provider == nil || *summaries[0].Provider != "Known provider" || len(summaries[0].Findings) != 0 {
			t.Fatal(r)
		}
	}
	r := runRead(t, dir, "show", id)
	if r.Exit != 0 {
		t.Fatal(r)
	}
	golden(t, "show.txt", regexp.MustCompile(`ts_[0-9a-f]{16}`).ReplaceAllString(r.Stdout, "ts_6f1c2a9b04d3e871"))
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if len([]rune(line)) > 44 {
			t.Fatalf("receipt too wide: %q", line)
		}
	}
	r = runRead(t, dir, "receipts", "--model", "equal", "--json")
	var rows []store.Summary
	json.Unmarshal([]byte(r.Stdout), &rows)
	if len(rows) != 1 || rows[0].Cost != nil {
		t.Fatal(r)
	}
	for _, args := range [][]string{{"receipts", "--since", "bad"}, {"receipts", "--limit", "-1"}, {"receipts", "--since", "2026-10-21", "--until", "2026-10-20"}} {
		if r = runRead(t, dir, args...); r.Exit != 1 {
			t.Fatal(r)
		}
	}
	if r = runRead(t, dir, "show", "absent"); r.Exit != 5 {
		t.Fatal(r)
	}
}
func TestExportFormatsAndPreviews(t *testing.T) {
	dir, id := seedReads(t)
	for _, format := range []string{"bundle", "jsonl", "csv", "html"} {
		out := filepath.Join(dir, "calls."+format)
		r := runRead(t, dir, "export", "--format", format, "--out", out, "--dry-run")
		if r.Exit != 0 || !strings.Contains(r.Stdout, "Would write") {
			t.Fatal(r)
		}
		for _, term := range []string{"created ", "updated ", "deleted ", "removed ", "successfully"} {
			if strings.Contains(strings.ToLower(r.Stdout), term) {
				t.Fatal(r)
			}
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("dry run wrote output")
		}
		r = runRead(t, dir, "export", "--format", format, "--out", out, "--json")
		if r.Exit != 0 || !json.Valid([]byte(r.Stdout)) {
			t.Fatal(r)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("PRIVATE")) {
			t.Fatal("export leaked content")
		}
		switch format {
		case "bundle":
			var bundle receipt.Bundle
			json.Unmarshal(b, &bundle)
			if len(bundle.Receipts) != 2 || len(bundle.Supplements) != 1 {
				t.Fatal(string(b))
			}
			for _, args := range [][]string{{"verify", out}, {"verify", out, "--key", filepath.Join(dir, "key.pub")}, {"verify", out, "--key-id", bundle.KeyID}} {
				v := runRead(t, dir, append(args, "--json")...)
				if v.Exit != 0 || !json.Valid([]byte(v.Stdout)) || v.Stderr != "" {
					t.Fatal(v)
				}
			}
			v := runRead(t, dir, "verify", out, "--key-id", "ts_different", "--json")
			if v.Exit != 11 || !strings.Contains(v.Stdout, "signer mismatch") {
				t.Fatal(v)
			}
			v = runRead(t, t.TempDir(), "verify", out)
			if v.Exit != 11 || !strings.Contains(v.Stdout, "Not confirmed") {
				t.Fatal(v)
			}
		case "jsonl":
			lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
			if len(lines) != 2 {
				t.Fatal(string(b))
			}
			for _, l := range lines {
				var m map[string]json.RawMessage
				if json.Unmarshal(l, &m) != nil || len(m) != 2 {
					t.Fatal(string(l))
				}
			}
		case "csv":
			rows, e := csv.NewReader(bytes.NewReader(b)).ReadAll()
			if e != nil || len(rows) != 3 || strings.Join(rows[0], ",") != strings.Join(csvColumns, ",") || rows[1][0] != id || rows[1][11] != "0.100000000000000001" || rows[1][10] != "" {
				t.Fatal(string(b))
			}
		case "html":
			if bytes.Contains(b, []byte("<script>")) || !bytes.Contains(b, []byte("&lt;script&gt;")) || !bytes.Contains(b, []byte("0.100000000000000001")) || !bytes.Contains(b, []byte("@media print")) {
				t.Fatal(string(b))
			}
		}
		before := string(b)
		r = runRead(t, dir, "export", "--format", format, "--out", out)
		after, _ := os.ReadFile(out)
		if r.Exit != 2 || string(after) != before {
			t.Fatal("overwrote output", r)
		}
	}
	r := runRead(t, dir, "key", "export", "--out", filepath.Join(dir, "export.pub"), "--dry-run")
	if r.Exit != 0 {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(dir, "export.pub")); !os.IsNotExist(err) {
		t.Fatal("key dry run wrote")
	}
}
func TestReadonlyNeverInitializesState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	for _, args := range [][]string{{"receipts"}, {"show", "missing"}, {"key"}, {"key", "export", "--out", filepath.Join(t.TempDir(), "key.pub")}, {"prices"}} {
		runRead(t, dir, args...)
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("read command initialized state", args)
		}
	}
}
func TestVerifyInvalidAndSignerPrecedence(t *testing.T) {
	dir, _ := seedReads(t)
	out := filepath.Join(dir, "bundle.json")
	runRead(t, dir, "export", "--out", out)
	b, _ := os.ReadFile(out)
	var original receipt.Bundle
	json.Unmarshal(b, &original)
	_, wrong, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(wrong.Public())
	wrongPath := filepath.Join(dir, "wrong.pub")
	os.WriteFile(wrongPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0600)
	for _, tc := range []struct {
		name   string
		modify func(*receipt.Bundle)
	}{{"signature", func(b *receipt.Bundle) {
		var e receipt.Envelope
		json.Unmarshal(b.Receipts[0], &e)
		p, _ := base64.StdEncoding.DecodeString(e.Payload)
		p = bytes.Replace(p, []byte("Known provider"), []byte("Other provider"), 1)
		e.Payload = base64.StdEncoding.EncodeToString(p)
		b.Receipts[0], _ = json.Marshal(e)
	}}, {"malformed payload", func(b *receipt.Bundle) {
		var e receipt.Envelope
		json.Unmarshal(b.Receipts[0], &e)
		e.Payload = base64.StdEncoding.EncodeToString([]byte("{bad"))
		b.Receipts[0], _ = json.Marshal(e)
	}}, {"payload type", func(b *receipt.Bundle) {
		var e receipt.Envelope
		json.Unmarshal(b.Receipts[0], &e)
		e.PayloadType = "wrong"
		b.Receipts[0], _ = json.Marshal(e)
	}}, {"missing digest", func(b *receipt.Bundle) { b.Receipts = b.Receipts[1:] }}, {"substituted key", func(b *receipt.Bundle) {
		b.PublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	}}} {
		t.Run(tc.name, func(t *testing.T) {
			var bundle receipt.Bundle
			json.Unmarshal(b, &bundle)
			tc.modify(&bundle)
			raw, _ := json.Marshal(bundle)
			path := filepath.Join(t.TempDir(), "bad.json")
			os.WriteFile(path, raw, 0600)
			var r testutil.Result
			for _, signer := range [][]string{nil, {"--key", filepath.Join(dir, "key.pub")}, {"--key-id", original.KeyID}} {
				args := append([]string{"verify", path, "--json"}, signer...)
				r = runRead(t, dir, args...)
				var result receipt.Verification
				if r.Exit != 10 || json.Unmarshal([]byte(r.Stdout), &result) != nil || r.Stderr != "" || result.SignerConfirmed || !strings.HasPrefix(result.Signer, "Not confirmed:") {
					t.Fatal(r)
				}
			}
			human := runRead(t, dir, "verify", path)
			if human.Exit != 10 || strings.Contains(human.Stdout, "Trusted:") || !strings.Contains(human.Stdout, "Not confirmed:") {
				t.Fatal(human)
			}
			if tc.name == "malformed payload" && !strings.Contains(r.Stdout, "receipt 1 of 2") {
				t.Fatal(r)
			}
		})
	}
	r := runRead(t, dir, "verify", out, "--key", wrongPath, "--json")
	var wrongResult receipt.Verification
	if r.Exit != 10 || json.Unmarshal([]byte(r.Stdout), &wrongResult) != nil || wrongResult.SignerConfirmed || !strings.HasPrefix(wrongResult.Signer, "Not confirmed:") {
		t.Fatal(r)
	}
	r = runRead(t, dir, "verify", out, "--key", wrongPath)
	if r.Exit != 10 || strings.Contains(r.Stdout, "Trusted:") || !strings.Contains(r.Stdout, "Not confirmed:") {
		t.Fatal(r)
	}
	if r := runRead(t, dir, "verify", out, "--key", wrongPath, "--key-id", "foo"); r.Exit != 1 {
		t.Fatal(r)
	}
	single := filepath.Join(dir, "single.json")
	os.WriteFile(single, original.Receipts[0], 0600)
	if r := runRead(t, dir, "verify", single); r.Exit != 1 {
		t.Fatal(r)
	}
	if r := runRead(t, dir, "verify", single, "--key", filepath.Join(dir, "key.pub")); r.Exit != 0 {
		t.Fatal(r)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDoctorOfflineAndReachability(t *testing.T) {
	cfg := config.Default()
	cfg.Upstreams = []upstream.Config{{Name: "custom", BaseURL: "http://custom.invalid", API: "openai"}}
	checks := reachability(cfg, transportFunc(func(r *http.Request) (*http.Response, error) {
		if len(r.Header) != 0 {
			t.Fatal("reachability sends headers")
		}
		return nil, fmt.Errorf("offline")
	}))
	for _, c := range checks {
		expected := "warn"
		if c.Name == "upstream custom" {
			expected = "fail"
		}
		if c.Status != expected || c.Fix == "" {
			t.Fatal(c)
		}
	}
	for _, status := range []int{401, 403, 404, 405} {
		checks = reachability(cfg, transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
		}))
		for _, c := range checks {
			if c.Status != "ok" {
				t.Fatal(c)
			}
		}
	}
	dir := filepath.Join(t.TempDir(), "clean")
	for _, c := range localChecks(dir) {
		if c.Status == "fail" {
			t.Fatal(c)
		}
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("doctor initialized state")
	}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("x"), 0600)
	if localChecks(file)[0].Status != "fail" {
		t.Fatal("accepted non-directory")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tool":"tabslip","version":"test","key_id":"ts_test","unrecorded_calls":3}`)
	}))
	defer server.Close()
	fmt.Sscanf(strings.Split(server.URL, ":")[2], "%d", &cfg.Port)
	checks = portChecks(cfg)
	if len(checks) != 2 || checks[0].Status != "ok" || checks[1].Status != "warn" {
		t.Fatal(checks)
	}
	server.Close()
	if portChecks(cfg)[0].Status != "ok" {
		t.Fatal("port not free")
	}
}

func TestDoctorFailuresAndIdentity(t *testing.T) {
	dir, _ := seedReads(t)
	for _, c := range localChecks(dir) {
		if c.Status == "fail" {
			t.Fatal(c)
		}
	}
	os.Remove(filepath.Join(dir, "key.ed25519"))
	checks := localChecks(dir)
	if checks[len(checks)-1].Status != "fail" || checks[len(checks)-1].Fix == "" {
		t.Fatal(checks)
	}
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(cfg, []byte("port: [broken"), 0600)
	r := testutil.Run(NewApp(), "doctor", "--config", cfg, "--data-dir", dir, "--json")
	if r.Exit != 2 || !json.Valid([]byte(r.Stdout)) || r.Stderr != "" {
		t.Fatal(r)
	}
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "another program") }))
	defer foreign.Close()
	c := config.Default()
	fmt.Sscanf(strings.Split(foreign.URL, ":")[2], "%d", &c.Port)
	checks = portChecks(c)
	if checks[0].Status != "fail" || checks[0].Fix == "" {
		t.Fatal(checks)
	}
}
func TestPricePreviewAndOutputAreOffline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	for _, args := range [][]string{{"prices", "--json"}, {"prices", "update", "--dry-run", "--json"}} {
		r := runRead(t, dir, args...)
		if r.Exit != 0 || !json.Valid([]byte(r.Stdout)) {
			t.Fatal(r)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("price preview initialized state")
		}
	}
}

func TestOrdinaryErrorsRenderOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "absent.yaml"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	type errorCase struct {
		name    string
		args    []string
		exit    int
		message string
	}
	cases := []errorCase{
		{"unknown command", []string{"nosuch"}, 1, "unknown command"},
		{"missing receipt", []string{"show", "rcpt_00000000000000000000000000"}, 5, "not found"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"tool":"tabslip"}`) }))
	defer server.Close()
	port := strings.Split(server.URL, ":")[2]
	cases = append(cases, errorCase{"busy port", []string{"serve", "--port", port}, 2, "Tabslip is already running"})
	for _, tc := range cases {
		for _, machine := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%v", tc.name, machine), func(t *testing.T) {
				args := append([]string{}, tc.args...)
				if machine {
					args = append(args, "--json")
				}
				r := runRead(t, dir, args...)
				if r.Exit != tc.exit || r.Stdout != "" {
					t.Fatal(r)
				}
				if machine {
					var body map[string]json.RawMessage
					if err := json.Unmarshal([]byte(r.Stderr), &body); err != nil || len(body) != 2 {
						t.Fatal(r)
					}
					var code int
					json.Unmarshal(body["exit_code"], &code)
					if code != tc.exit || !strings.Contains(string(body["error"]), tc.message) {
						t.Fatal(r)
					}
				} else if strings.Count(r.Stderr, "\n") != 1 || !strings.HasPrefix(r.Stderr, "Error: ") || !strings.Contains(r.Stderr, tc.message) {
					t.Fatal(r)
				}
			})
		}
	}
}

func TestHumanListLocalTimeAndUnknownCost(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("test", 2*60*60)
	t.Cleanup(func() { time.Local = old })
	dir, _ := seedReads(t)
	result := runRead(t, dir, "receipts", "--model", "equal")
	if result.Exit != 0 || !strings.Contains(result.Stdout, "2026-10-19 02:00:00") || strings.Contains(result.Stdout, "unknown USD") || strings.Contains(result.Stdout, "unknown (unknown)") {
		t.Fatal(result)
	}
	rows, err := readCalls(dir, store.Filter{Model: "equal"})
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	var out bytes.Buffer
	renderReceiptStyled(&out, rows[0], receipt.Verification{SignaturesValid: true}, false)
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "| TOTAL ") && !strings.HasPrefix(line, "| TOTAL TIME") && (strings.Count(line, "unknown") != 1 || strings.Contains(line, "USD")) {
			t.Fatal(line)
		}
	}
	if strings.Contains(out.String(), "| unknown ") {
		t.Fatal("duplicate unknown cost source", out.String())
	}
	result = runRead(t, dir, "receipts", "--model", "equal", "--json")
	if !strings.Contains(result.Stdout, "2026-10-19T00:00:00.000Z") {
		t.Fatal(result)
	}
}

func TestShowChecksAllContributors(t *testing.T) {
	for _, change := range []string{"valid", "base only", "base signature", "supplement signature", "supplement digest", "supplement signer", "supplement type", "base type", "base signer", "malformed base", "malformed supplement", "malformed payload", "missing"} {
		t.Run(change, func(t *testing.T) {
			dir, id := seedReads(t)
			reader, err := store.OpenReader(dir)
			if err != nil {
				t.Fatal(err)
			}
			call, err := reader.Get(id)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", filepath.Join(dir, "tabslip.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := db.Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			want, check := 10, ""
			raw := call.Envelope
			supplement := strings.HasPrefix(change, "supplement")
			if supplement {
				raw = call.SupplementEnvelopes[0]
			}
			var env receipt.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "valid":
				want = 0
			case "base only":
				exec("DELETE FROM supplements")
				want = 0
			case "missing":
				id = "rcpt_missing"
				want = 5
			case "malformed base":
				exec("UPDATE receipts SET envelope = ? WHERE id = ?", "{", id)
				want = 2
			case "malformed payload":
				env.Payload = base64.StdEncoding.EncodeToString([]byte("{"))
				raw, err := json.Marshal(env)
				if err != nil {
					t.Fatal(err)
				}
				exec("UPDATE receipts SET envelope = ? WHERE id = ?", raw, id)
				want = 2
			case "malformed supplement":
				exec("UPDATE supplements SET envelope = ? WHERE receipt_id = ?", "{", id)
				want = 2
			default:
				switch {
				case strings.HasSuffix(change, "signature"):
					sig, _ := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
					sig[0] ^= 1
					env.Signatures[0].Sig = base64.StdEncoding.EncodeToString(sig)
					check = "invalid signature"
				case strings.HasSuffix(change, "signer"):
					env.Signatures[0].KeyID = "ts_wrong"
					check = "key id"
				case strings.HasSuffix(change, "type"):
					env.PayloadType = "wrong"
					check = "payload type"
				case change == "supplement digest":
					// Re-sign with the real key so the digest check is reached.
					key, err := keys.Load(dir, "", true)
					if err != nil {
						t.Fatal(err)
					}
					bad := call.Supplements[0]
					bad.ReceiptSHA256 = "wrong"
					signed, err := receipt.SignSupplement(&bad, key)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(signed, &env); err != nil {
						t.Fatal(err)
					}
					check = "wrong receipt digest"
				}
				altered, err := json.Marshal(env)
				if err != nil {
					t.Fatal(err)
				}
				if supplement {
					exec("UPDATE supplements SET envelope = ? WHERE receipt_id = ?", altered, id)
				} else {
					exec("UPDATE receipts SET envelope = ? WHERE id = ?", altered, id)
					call.Envelope = altered
				}
			}
			for _, machine := range []bool{false, true} {
				args := []string{"show", id}
				if machine {
					args = append(args, "--json")
				}
				got := runRead(t, dir, args...)
				if got.Exit != want {
					t.Fatalf("want exit %d: %+v", want, got)
				}
				if want == 2 || want == 5 {
					if strings.Contains(got.Stdout, "SIGNED") || strings.Contains(got.Stdout, "INVALID") {
						t.Fatal(got)
					}
					continue
				}
				if machine {
					if want == 0 && got.Stderr != "" {
						t.Fatal(got)
					}
					var actual, original receipt.Envelope
					if json.Unmarshal([]byte(got.Stdout), &actual) != nil || json.Unmarshal(call.Envelope, &original) != nil {
						t.Fatal(got)
					}
					a, _ := json.Marshal(actual)
					b, _ := json.Marshal(original)
					if !bytes.Equal(a, b) {
						t.Fatal("JSON changed original envelope", got)
					}
					if want == 10 && (!strings.Contains(got.Stderr, check) || (supplement && !strings.Contains(got.Stderr, "supplement"))) {
						t.Fatal(got)
					}
					if supplement && want == 10 {
						if _, err := receipt.Verify([]byte(got.Stdout), mustPublic(t, dir)); err != nil {
							t.Fatal("base no longer verifies", err)
						}
					}
				} else {
					stamp := "SIGNED"
					if want == 10 {
						stamp = "INVALID"
					}
					if !strings.Contains(got.Stdout, "[ "+stamp) || (want == 10 && (strings.Contains(got.Stdout, "[ SIGNED") || !strings.Contains(got.Stdout, check))) {
						t.Fatal(got)
					}
					for _, line := range strings.Split(strings.TrimSpace(got.Stdout), "\n") {
						if len([]rune(line)) > 44 {
							t.Fatalf("too wide: %q", line)
						}
					}
				}
			}
		})
	}
}
func mustPublic(t *testing.T, dir string) ed25519.PublicKey {
	t.Helper()
	_, pub, err := keys.Public(dir)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestShowDoesNotRepairMissingPublicKey(t *testing.T) {
	dir, id := seedReads(t)
	path := filepath.Join(dir, "key.pub")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"show", id}, {"show", id, "--json"}} {
		got := runRead(t, dir, args...)
		if got.Exit != 5 || got.Stdout != "" {
			t.Fatal(got)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("read repaired the public key", err)
		}
	}
}
