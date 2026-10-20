package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/prices"
	"github.com/vincentsch/tollglass/internal/upstream"
)

func TestGatewayKeepsStartupPricesUntilRestart(t *testing.T) {
	dir := t.TempDir()
	first := []byte(`{"version":"first","models":{"test-model":{"litellm_provider":"openai","input_cost_per_token":0.01,"output_cost_per_token":0.02}}}`)
	second := []byte(strings.ReplaceAll(string(first), "0.01", "0.09"))
	if err := prices.Install(dir, first, "first"); err != nil {
		t.Fatal(err)
	}
	table, _, err := prices.Selected(dir)
	if err != nil {
		t.Fatal(err)
	}
	us, _ := upstream.Build(nil)
	records := &recorder{make(chan captured, 3)}
	g := &Gateway{Upstreams: us, Config: config.Default(), KeyID: "tg_test", Store: records, Prices: &table, Transport: injectedTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"model":"test-model","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	call := func(want string) {
		t.Helper()
		req := httptest.NewRequest("POST", "http://localhost/openai/v1/chat/completions", strings.NewReader(`{"model":"test-model"}`))
		g.ServeHTTP(httptest.NewRecorder(), req)
		r := take(t, records).r
		if r.Cost.Amount == nil || *r.Cost.Amount != want {
			t.Fatal(r.Cost)
		}
	}
	call("0.030000000000")
	if err = prices.Install(dir, second, "second"); err != nil {
		t.Fatal(err)
	}
	call("0.030000000000")
	restarted, _, err := prices.Selected(dir)
	if err != nil {
		t.Fatal(err)
	}
	g.Prices = &restarted
	call("0.110000000000")
}
