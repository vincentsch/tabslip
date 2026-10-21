package prices

import (
	"bytes"
	"encoding/json"
	"github.com/vincentsch/tabslip/internal/receipt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotDownloadInstallAndFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prices")
	_, info, err := Selected(dir)
	if err != nil || info.Source != "embedded" {
		t.Fatal(info, err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read initialized directory")
	}
	source := `{"model":{"mode":"chat","litellm_provider":"openai","input_cost_per_token":0.000000000123456789,"output_cost_per_token":1.2300e-6,"unused":123},"image":{"mode":"image_generation","input_cost_per_token":3}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(source)) }))
	defer server.Close()
	b, name, err := Download(server.Client(), server.URL, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("0.000000000123456789")) || !bytes.Contains(b, []byte("1.2300e-6")) || bytes.Contains(b, []byte("unused")) || bytes.Contains(b, []byte(`"image"`)) {
		t.Fatal(string(b))
	}
	if err = Install(dir, b, name); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, name+".json")); err != nil {
		t.Fatal(err)
	}
	table, info, err := Selected(dir)
	if err != nil || info.Source != "downloaded" || info.Date != "2026-10-20" {
		t.Fatal(info, err)
	}
	if string(table.Models["model"].Output) != "1.2300e-6" {
		t.Fatal(table)
	}
	for _, bad := range []string{`{broken`, `{}`, `{"model":{"mode":"chat","litellm_provider":"openai","input_cost_per_token":-1}}`} {
		source = bad
		if _, _, err = Download(server.Client(), server.URL, time.Now()); err == nil {
			t.Fatal("accepted bad prices", bad)
		}
		current, _ := os.ReadFile(filepath.Join(dir, "prices.json"))
		if !bytes.Equal(current, b) {
			t.Fatal("failed download changed state")
		}
	}
	if !strings.HasPrefix(name, "litellm-2026-10-20T120000.") {
		t.Fatal(name)
	}
}

func TestSelectedTableRemainsStableUntilReload(t *testing.T) {
	dir := t.TempDir()
	first := []byte(`{"version":"first","models":{"gpt-test":{"litellm_provider":"openai","input_cost_per_token":0.01,"output_cost_per_token":0.02}}}`)
	second := bytes.ReplaceAll(first, []byte("0.01"), []byte("0.09"))
	if err := Install(dir, first, "first"); err != nil {
		t.Fatal(err)
	}
	running, _, err := Selected(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = Install(dir, second, "second"); err != nil {
		t.Fatal(err)
	}
	restarted, _, err := Selected(dir)
	if err != nil {
		t.Fatal(err)
	}
	if running.Models["gpt-test"].Input != "0.01" || restarted.Models["gpt-test"].Input != "0.09" {
		t.Fatal("running price table changed")
	}
}

func TestDownloadedRatesPreserveMissingAndExplicitZero(t *testing.T) {
	source := `{
 "openai-cache":{"mode":"chat","litellm_provider":"openai","input_cost_per_token":0.00003,"output_cost_per_token":0.000003},
 "anthropic-cache":{"mode":"chat","litellm_provider":"anthropic","input_cost_per_token":0.000003,"output_cost_per_token":0.000015,"cache_creation_input_token_cost":0.00000375},
 "missing-output":{"mode":"chat","litellm_provider":"openai","input_cost_per_token":0.00003},
 "free-output":{"mode":"chat","litellm_provider":"openai","input_cost_per_token":0.00003,"output_cost_per_token":0}
 }`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, source) }))
	defer server.Close()
	b, name, err := Download(server.Client(), server.URL, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Models map[string]map[string]json.RawMessage `json:"models"`
	}
	if err = json.Unmarshal(b, &document); err != nil {
		t.Fatal(err)
	}
	for model, row := range document.Models {
		for _, field := range []string{"cache_read_input_token_cost", "cache_creation_input_token_cost_above_1hr", "cache_creation_input_token_cost"} {
			if model == "anthropic-cache" && field == "cache_creation_input_token_cost" {
				continue
			}
			if _, exists := row[field]; exists {
				t.Fatalf("%s gained %s: %s", model, field, b)
			}
		}
	}
	if _, exists := document.Models["missing-output"]["output_cost_per_token"]; exists {
		t.Fatal("missing output became a rate")
	}
	if string(document.Models["free-output"]["output_cost_per_token"]) != "0" {
		t.Fatal("explicit zero was lost")
	}
	dir := t.TempDir()
	if err = Install(dir, b, name); err != nil {
		t.Fatal(err)
	}
	table, _, err := Selected(dir)
	if err != nil {
		t.Fatal(err)
	}
	number := func(n int64) *int64 { return &n }
	for _, tc := range []struct {
		model, upstream, want              string
		input, output, cached, write, hour int64
	}{
		{"openai-cache", "openai", "0.030600000000", 1000, 200, 500, 0, 0},
		{"anthropic-cache", "anthropic", "0.004200000000", 100, 10, 0, 1000, 1000},
		{"missing-output", "openai", "", 1000, 200, 0, 0, 0},
		{"free-output", "openai", "0.030000000000", 1000, 200, 0, 0, 0},
	} {
		t.Run(tc.model, func(t *testing.T) {
			r := receipt.New("test", time.Now())
			r.Request.Upstream = tc.upstream
			r.Request.API = "openai.chat"
			if tc.upstream == "anthropic" {
				r.Request.API = "anthropic.messages"
			}
			r.Request.Model = &tc.model
			r.Usage = receipt.Usage{Source: "reported", Input: number(tc.input), Output: number(tc.output), Cached: number(tc.cached), CacheWrite: number(tc.write), CacheWrite1h: number(tc.hour)}
			table.Apply(r)
			if tc.want == "" {
				if r.Cost.Amount != nil || r.Cost.Source != "unknown" {
					t.Fatal(r.Cost)
				}
			} else if r.Cost.Amount == nil || *r.Cost.Amount != tc.want {
				t.Fatal(r.Cost)
			}
		})
	}
}
