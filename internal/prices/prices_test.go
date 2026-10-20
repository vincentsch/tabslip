package prices_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/apis"
	"github.com/vincentsch/tollglass/internal/prices"
	"github.com/vincentsch/tollglass/internal/receipt"
)

func ptr[T any](v T) *T { return &v }
func TestExactTokenCosts(t *testing.T) {
	for _, tc := range []struct {
		model, api                                    string
		input, output, cached, write, hour, reasoning int64
		want                                          string
	}{
		{"gpt-4.1", "openai.chat", 1000, 200, 400, 0, 0, 0, "0.003000000000"},
		{"gpt-4o-mini", "openai.chat", 1000, 200, 0, 0, 0, 0, "0.000270000000"},
		{"o4-mini", "openai.responses", 1000, 200, 0, 0, 0, 100, "0.001980000000"},
		{"text-embedding-3-small", "openai.embeddings", 1000, 0, 0, 0, 0, 0, "0.000020000000"},
		{"claude-sonnet-4-5", "anthropic.messages", 100, 12, 50, 25, 10, 0, "0.000611250000"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			r := receipt.New("test", time.Now())
			r.Request.Upstream = "openai"
			if tc.api == "anthropic.messages" {
				r.Request.Upstream = "anthropic"
			}
			r.Request.API = tc.api
			r.Request.Model = &tc.model
			r.Usage = receipt.Usage{Source: "reported", Input: &tc.input, Output: &tc.output, Cached: &tc.cached, CacheWrite: &tc.write, CacheWrite1h: &tc.hour, Reasoning: &tc.reasoning}
			if tc.api == "openai.embeddings" {
				r.Usage.Output = nil
			}
			prices.Embedded.Apply(r)
			if r.Cost.Source != "estimated" || r.Cost.Amount == nil || *r.Cost.Amount != tc.want || r.Cost.PriceTable == nil {
				t.Fatalf("cost %+v", r.Cost)
			}
		})
	}
}
func TestCostSourcesAndNamespace(t *testing.T) {
	for _, tc := range []struct {
		upstream, model  string
		reported         *string
		source           string
		amount           *string
		warning, version bool
	}{
		{"openai", "gpt-4o-mini", nil, "estimated", ptr("0.000270000000"), false, true},
		{"openai", "gpt-4o-mini", ptr("0.0100"), "reported", ptr("0.0100"), false, true},
		{"openrouter", "gpt-4o-mini", ptr("0"), "reported", ptr("0"), false, false},
		{"openrouter", "gpt-4o-mini", nil, "unknown", nil, true, false},
		{"openai", "claude-sonnet-4-5", nil, "unknown", nil, true, true},
		{"anthropic", "gpt-4o-mini", nil, "unknown", nil, true, true},
		{"openai", "missing", nil, "unknown", nil, true, true},
		{"ollama", "gpt-4o-mini", nil, "unknown", nil, false, false},
		{"local", "gpt-4o-mini", nil, "unknown", nil, false, false},
		{"demo", "gpt-4o-mini", nil, "unknown", nil, false, false},
	} {
		t.Run(tc.upstream+"/"+tc.model+"/"+tc.source, func(t *testing.T) {
			r := receipt.New("test", time.Now())
			r.Request.Upstream = tc.upstream
			r.Request.API = "openai.chat"
			r.Request.Model = &tc.model
			r.Usage = receipt.Usage{Source: "reported", Input: ptr(int64(1000)), Output: ptr(int64(200))}
			if tc.reported != nil {
				r.Cost = receipt.Cost{Source: "reported", Amount: tc.reported, Currency: ptr("USD")}
			}
			prices.Embedded.Apply(r)
			warn := false
			for _, f := range r.Findings {
				if f.Code == "price_unknown" {
					warn = true
				}
			}
			if r.Cost.Source != tc.source || (r.Cost.PriceTable != nil) != tc.version || warn != tc.warning {
				t.Fatalf("%+v %v", r.Cost, r.Findings)
			}
			if tc.amount != nil && (r.Cost.Amount == nil || *r.Cost.Amount != *tc.amount) {
				t.Fatal("amount", r.Cost)
			}
			if tc.upstream == "openai" && tc.reported != nil && (r.Cost.Estimate == nil || *r.Cost.Estimate != "0.000270000000") {
				t.Fatal("missing comparison estimate")
			}
			if tc.upstream == "openrouter" && r.Cost.Estimate != nil {
				t.Fatal("router estimate")
			}
		})
	}
	for _, tc := range []struct {
		upstream, model string
		priceUnknown    bool
	}{
		{"openai", "gpt-4o-mini", false},
		{"anthropic", "claude-sonnet-4-5", false},
		{"openai", "missing", true},
		{"anthropic", "missing", true},
		{"openrouter", "gpt-4o-mini", true},
	} {
		t.Run(tc.upstream+"/"+tc.model+"/unknown-usage", func(t *testing.T) {
			r := receipt.New("test", time.Now())
			r.Request.Upstream = tc.upstream
			r.Request.Model = &tc.model
			apis.Finish(r)
			prices.Embedded.Apply(r)
			if r.Cost.Source != "unknown" || r.Cost.Amount != nil || r.Cost.Estimate != nil {
				t.Fatalf("unknown usage must leave cost unknown: %+v", r.Cost)
			}
			if (r.Cost.PriceTable != nil) != (tc.upstream != "openrouter") {
				t.Fatalf("lookup table provenance: %+v", r.Cost)
			}
			priceUnknown, usageMissing := false, false
			for _, f := range r.Findings {
				priceUnknown = priceUnknown || f.Code == "price_unknown"
				usageMissing = usageMissing || f.Code == "usage_missing"
			}
			if priceUnknown != tc.priceUnknown || !usageMissing {
				t.Fatalf("incorrect findings for unknown usage: %+v", r.Findings)
			}
		})
	}

}
func TestUnknownUsageAndModelMatching(t *testing.T) {
	r := receipt.New("test", time.Now())
	r.Request.Upstream = "openai"
	r.Request.API = "openai.chat"
	r.Request.Model = ptr("gpt-4.1")
	prices.Embedded.Apply(r)
	if r.Cost.Amount != nil {
		t.Fatal("unknown usage priced at zero")
	}
	if r.Cost.PriceTable == nil || len(r.Findings) != 0 {
		t.Fatalf("matched price with unknown usage: cost=%+v findings=%+v", r.Cost, r.Findings)
	}
	r.Usage = receipt.Usage{Source: "reported", Input: ptr(int64(1000)), Output: ptr(int64(200))}
	r.Response.Model = ptr("gpt-4o-mini")
	prices.Embedded.Apply(r)
	if *r.Cost.Amount != "0.000270000000" {
		t.Fatal("reported model not preferred")
	}
	r.Response.Model = ptr("missing")
	prices.Embedded.Apply(r)
	if *r.Cost.Amount != "0.003600000000" {
		t.Fatal("request fallback")
	}
	r.Request.Model = ptr("gpt-4o-mini-2099-01-01")
	prices.Embedded.Apply(r)
	if *r.Cost.Amount != "0.000270000000" {
		t.Fatal("date fallback")
	}
}
func TestAnthropicFixtureCostAndDisplay(t *testing.T) {
	b, err := os.ReadFile("../../testdata/messages.json")
	if err != nil {
		t.Fatal(err)
	}
	r := receipt.New("test", time.Now())
	r.Request.Upstream = "anthropic"
	r.Request.API = "anthropic.messages"
	r.Request.Model = ptr("claude-sonnet-4-5")
	c := apis.Capture{Receipt: r}
	c.Parse(bytes.NewReader(b), false)
	// The source fixture's synthetic model is changed only for this price lookup.
	r.Response.Model = ptr("claude-sonnet-4-5")
	prices.Embedded.Apply(r)
	if r.DisplayInput() == nil || *r.DisplayInput() != 175 || r.Cost.Amount == nil || *r.Cost.Amount != "0.000611250000" {
		t.Fatalf("display=%v cost=%+v", r.DisplayInput(), r.Cost)
	}
}
func TestDecimalPrecisionAndRounding(t *testing.T) {
	for _, s := range []string{"0", "0.000000123456789012345", "1.234e-7"} {
		v := prices.Decimal(json.Number(s))
		if v == nil || *v != s {
			t.Fatal(s)
		}
	}
	for _, v := range []any{nil, "1.2", json.Number("-1")} {
		if prices.Decimal(v) != nil {
			t.Fatal(v)
		}
	}
	table := prices.Table{Version: "test", Models: map[string]prices.Rate{"tiny": {Provider: "openai", Input: json.Number("5e-13")}}}
	r := receipt.New("test", time.Now())
	r.Request.Upstream = "openai"
	r.Request.API = "openai.embeddings"
	r.Request.Model = ptr("tiny")
	r.Usage = receipt.Usage{Source: "reported", Input: ptr(int64(1))}
	table.Apply(r)
	if r.Cost.Amount == nil || *r.Cost.Amount != "0.000000000001" {
		t.Fatal("half-up rounding", r.Cost)
	}
}
