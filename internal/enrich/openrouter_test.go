package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGenerationMappingAndNullAttempts(t *testing.T) {
	b, err := os.ReadFile("../../testdata/openrouter-generation.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if s.Cost == nil || *s.Cost != "0.001500" || s.Provider == nil || *s.Provider != "Infermatic" || len(s.Attempts) != 2 || *s.Attempts[0].Status != 503 || *s.Attempts[1].Provider != "Infermatic" || *s.NativeTokens.Prompt != 10 || *s.NativeTokens.Completion != 25 || *s.DataRegion != "global" {
		t.Fatalf("mapping %+v", s)
	}
	if len(s.Findings) != 1 || s.Findings[0].Text != "OpenRouter tried 2 providers. The answer came from Infermatic." {
		t.Fatal(s.Findings)
	}
	saved, _ := json.Marshal(s)
	if strings.Contains(string(saved), "synthetic-private-marker") || strings.Contains(string(saved), "external_user") {
		t.Fatal("unselected metadata copied")
	}
	for _, raw := range []string{`{"data":{"provider_responses":null}}`, `{"data":{}}`} {
		s, err := Parse([]byte(raw))
		if err != nil || len(s.Attempts) != 0 || s.Attempts == nil || len(s.Findings) != 0 || s.Provider != nil || s.Cost != nil || s.NativeTokens.Prompt != nil {
			t.Fatalf("invented metadata: %+v %v", s, err)
		}
	}
}
func TestLookupDelayedSuccessAndExhaustion(t *testing.T) {
	b, err := os.ReadFile("../../testdata/openrouter-generation.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "delayed-success", false: "exhausted"}[success], func(t *testing.T) {
			var count atomic.Int32
			lookup := OpenRouter(roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://openrouter.ai/api/v1/generation?id=gen-test" || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
					t.Error("lookup target or authorization")
				}
				n := count.Add(1)
				if !success {
					return nil, errors.New("synthetic-secret must never be logged")
				}
				if n == 1 {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"not ready"}}`))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s, err := lookup(ctx, "gen-test", "Bearer synthetic-secret")
			if success && (err != nil || s == nil || count.Load() != 2) {
				t.Fatal(count.Load(), err)
			}
			if !success && (err == nil || s != nil || count.Load() != 3 || strings.Contains(err.Error(), "synthetic-secret")) {
				t.Fatal(count.Load(), err)
			}
		})
	}
}
func TestLookupHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	lookup := OpenRouter(roundTrip(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() }))
	started := time.Now()
	if s, err := lookup(ctx, "id", ""); err == nil || s != nil || time.Since(started) > time.Second {
		t.Fatal("lookup ignored deadline")
	}
}
