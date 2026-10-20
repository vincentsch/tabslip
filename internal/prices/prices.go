// Package prices calculates exact token costs from a local embedded snapshot.
package prices

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/vincentsch/tollglass/internal/receipt"
)

//go:generate go run ./gen
//go:embed prices.json
var snapshot []byte

type Rate struct {
	Provider string      `json:"litellm_provider"`
	Input    json.Number `json:"input_cost_per_token,omitempty"`
	Output   json.Number `json:"output_cost_per_token,omitempty"`
	Read     json.Number `json:"cache_read_input_token_cost,omitempty"`
	Write    json.Number `json:"cache_creation_input_token_cost,omitempty"`
	Write1h  json.Number `json:"cache_creation_input_token_cost_above_1hr,omitempty"`
}
type Table struct {
	Version string          `json:"version"`
	Models  map[string]Rate `json:"models"`
}

var Embedded = load()

func load() Table {
	var t Table
	d := json.NewDecoder(bytes.NewReader(snapshot))
	d.UseNumber()
	if err := d.Decode(&t); err != nil {
		panic(err)
	}
	return t
}

var dateSuffix = regexp.MustCompile(`(?:-[0-9]{4}-[0-9]{2}-[0-9]{2}|-[0-9]{8})$`)

func (t Table) lookup(provider string, reported, requested *string) (Rate, bool) {
	names := []string{}
	for _, model := range []*string{reported, requested} {
		if model != nil {
			names = append(names, *model)
		}
	}
	for _, name := range append(append([]string{}, names...), stripDates(names)...) {
		for _, key := range []string{name, provider + "/" + name} {
			if rate, ok := t.Models[key]; ok && rate.Provider == provider {
				return rate, true
			}
		}
	}
	return Rate{}, false
}
func stripDates(names []string) []string {
	out := []string{}
	for _, n := range names {
		out = append(out, dateSuffix.ReplaceAllString(n, ""))
	}
	return out
}
func count(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}
func fallback(a, b json.Number) json.Number {
	if a == "" {
		return b
	}
	return a
}
func charge(sum *big.Rat, tokens int64, price json.Number) bool {
	if tokens < 0 {
		return false
	}
	if tokens == 0 {
		return true
	}
	p, ok := new(big.Rat).SetString(string(price))
	if !ok || p.Sign() < 0 {
		return false
	}
	sum.Add(sum, p.Mul(p, new(big.Rat).SetInt64(tokens)))
	return true
}
func estimate(r *receipt.Receipt, rate Rate) *string {
	u := r.Usage
	if u.Source != "reported" || u.Input == nil || (u.Output == nil && r.Request.API != "openai.embeddings") {
		return nil
	}
	if r.Request.Upstream == "anthropic" && (u.Cached == nil || u.CacheWrite == nil) {
		return nil
	}
	input, cached, writes, hour := *u.Input, count(u.Cached), count(u.CacheWrite), count(u.CacheWrite1h)
	if r.Request.Upstream == "openai" {
		input -= cached
		writes, hour = 0, 0
	}
	sum := new(big.Rat)
	if !charge(sum, input, rate.Input) || !charge(sum, cached, fallback(rate.Read, rate.Input)) || !charge(sum, writes-hour, rate.Write) || !charge(sum, hour, fallback(rate.Write1h, rate.Write)) || !charge(sum, count(u.Output), rate.Output) {
		return nil
	}
	s := sum.FloatString(12)
	return &s
}
func (t Table) Apply(r *receipt.Receipt) {
	provider := r.Request.Upstream
	if provider != "openai" && provider != "anthropic" && provider != "openrouter" {
		return
	}
	if provider != "openrouter" {
		version := t.Version
		r.Cost.PriceTable = &version
		if rate, ok := t.lookup(provider, r.Response.Model, r.Request.Model); ok {
			if value := estimate(r, rate); value != nil {
				if r.Cost.Source == "reported" {
					r.Cost.Estimate = value
				} else {
					usd := "USD"
					r.Cost.Source, r.Cost.Amount, r.Cost.Currency = "estimated", value, &usd
				}
			}
			return
		}
	}
	if r.Cost.Amount == nil {
		r.Find("price_unknown", "observed", "The cost of this call is unknown.")
	}
}

// Decimal retains the provider's number text, including exponent notation.
func Decimal(v any) *string {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	s := string(n)
	if len(s) > 128 || strings.ContainsAny(s, "/ ") {
		return nil
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(s[i+1:])
		if err != nil || exponent < -100 || exponent > 100 {
			return nil
		}
	}
	p, ok := new(big.Rat).SetString(s)
	if !ok || p.Sign() < 0 {
		return nil
	}
	return &s
}
