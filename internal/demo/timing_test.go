package demo

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/proxy"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
)

func TestDelayProfiles(t *testing.T) {
	cases := []struct {
		path, model  string
		stream       bool
		first, total time.Duration
	}{
		{"/v1/chat/completions", "demo/small", false, 250, 250},
		{"/v1/chat/completions", "demo/small", true, 300, 1200},
		{"/v1/embeddings", "demo/embedding", false, 200, 200},
		{"/v1/messages", "demo/messages", false, 450, 450},
		{"/v1/messages", "demo/messages", true, 400, 1500},
		{"/v1/chat/completions", "demo/large", false, 900, 900},
		{"/v1/chat/completions", "demo/error", false, 200, 200},
		{"/v1/chat/completions", "demo/no-usage", false, 350, 350},
		{"/v1/chat/completions", "gpt-4.1", false, 40, 40},
		{"/v1/chat/completions", "gpt-4.1", true, 40, 160},
		{"/v1/messages", "demo/small", false, 40, 40},
	}
	var seed time.Duration
	for i, c := range cases {
		got := delayProfile(c.path, c.model, c.stream)
		want := timingProfile{c.first * time.Millisecond, c.total * time.Millisecond}
		if got != want || delayProfile(c.path, c.model, c.stream) != got {
			t.Fatalf("%+v: %+v", c, got)
		}
		if i < 8 {
			seed += got.total
		}
	}
	if seed > 6*time.Second {
		t.Fatal("seed too slow", seed)
	}
}

func TestDelayedStreamReachesClientAndRecordsMeasuredTimes(t *testing.T) {
	db, err := store.Open(t.TempDir(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	target, _ := url.Parse("http://demo.invalid")
	gateway := &proxy.Gateway{Upstreams: map[string]upstream.Upstream{"demo": {Name: "demo", URL: target, API: "openai"}}, Config: config.Default(), KeyID: db.KeyID, Store: db, Transport: Transport{Delays: true}, Demo: true}
	server := httptest.NewServer(gateway)
	defer server.Close()
	start := time.Now()
	resp, err := server.Client().Post(server.URL+"/demo/v1/chat/completions", "application/json", strings.NewReader(`{"model":"gpt-4.1","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	event, err := reader.ReadString('\n')
	first := time.Since(start)
	if err != nil || !strings.Contains(event, "data:") {
		t.Fatal(event, err)
	}
	rest, err := io.ReadAll(reader)
	total := time.Since(start)
	if err != nil || !strings.Contains(string(rest), "[DONE]") {
		t.Fatal(string(rest), err)
	}
	if first < 25*time.Millisecond || total-first < 80*time.Millisecond || total > 2*time.Second {
		t.Fatal("delivery times", first, total)
	}
	gateway.Wait()
	if err := db.Flush(); err != nil {
		t.Fatal(err)
	}
	calls, err := db.Reader().Calls(store.Filter{Limit: 10})
	if err != nil || len(calls) != 1 {
		t.Fatal(calls, err)
	}
	recorded := calls[0].Receipt.Time
	if recorded.TTFB == nil || *recorded.TTFB < 25 || recorded.Total < 130 || recorded.Total-*recorded.TTFB < 80 {
		t.Fatalf("unmeasured delays: %+v", recorded)
	}
}

func TestDelayedResponseCancellation(t *testing.T) {
	for _, stage := range []string{"headers", "body", "close"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", "http://demo.invalid/v1/chat/completions", strings.NewReader(`{"model":"demo/small","stream":true}`))
			if stage == "headers" {
				time.AfterFunc(20*time.Millisecond, cancel)
				start := time.Now()
				_, err := (Transport{Delays: true}).RoundTrip(req)
				if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
					t.Fatal(err)
				}
				return
			}
			resp, err := (Transport{Delays: true}).RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b := make([]byte, 4096)
			if _, err = resp.Body.Read(b); err != nil {
				t.Fatal(err)
			}
			time.AfterFunc(20*time.Millisecond, func() {
				if stage == "close" {
					resp.Body.Close()
				} else {
					cancel()
				}
			})
			start := time.Now()
			_, err = resp.Body.Read(b)
			expected := error(context.Canceled)
			if stage == "close" {
				expected = io.ErrClosedPipe
			}
			if !errors.Is(err, expected) || time.Since(start) > time.Second {
				t.Fatal(err)
			}
		})
	}
}

func TestSeedReportsGenuineFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := Seed(context.Background(), server.Client(), server.URL); err == nil || err.Error() != "demo call failed" {
		t.Fatal(err)
	}
}
