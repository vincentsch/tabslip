package proxy

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/enrich"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
)

type injectedTransport func(*http.Request) (*http.Response, error)

func (f injectedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPresetOpenRouterEnrichmentDoesNotDelayClient(t *testing.T) {
	raw := fixture(t, "openrouter-chat.sse")
	generation := fixture(t, "openrouter-generation.json")
	for _, mode := range []string{"delayed", "exhausted", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			var logs bytes.Buffer
			db, err := store.Open(dir, 0, log.New(&logs, "", 0))
			if err != nil {
				t.Fatal(err)
			}
			pub := db.Key.Public().(ed25519.PublicKey)
			us, _ := upstream.Build(nil)
			cfg := config.Default()
			cfg.EnrichOpenrouter = mode != "disabled"
			var attempts atomic.Int32
			allow := make(chan struct{})
			transport := injectedTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "openrouter.ai" || req.Header.Get("Authorization") != "Bearer synthetic-private-marker" {
					t.Error("preset or auth changed")
				}
				if req.URL.Path == "/api/v1/chat/completions" {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
				}
				if req.URL.Path != "/api/v1/generation" || req.URL.Query().Get("id") != "gen-synthetic" {
					t.Error("generation target")
				}
				n := attempts.Add(1)
				select {
				case <-allow:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
				if mode == "exhausted" || n == 1 {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic-private-marker"}}`))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(generation))}, nil
			})
			worker := &enrich.Worker{Lookup: enrich.OpenRouter(transport), Save: db.SaveSupplement}
			gateway := &Gateway{Upstreams: us, Config: cfg, KeyID: db.KeyID, Store: db, Transport: transport, Enrich: worker.Schedule}
			srv := httptest.NewServer(gateway)
			defer srv.Close()
			req, _ := http.NewRequest("POST", srv.URL+"/openrouter/v1/chat/completions", strings.NewReader(`{"model":"sao10k/l3-stheno-8b","stream":true}`))
			req.Header.Set("Authorization", "Bearer synthetic-private-marker")
			client := &http.Client{Timeout: time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || !bytes.Equal(body, raw) {
				t.Fatal("response affected", err)
			}
			gateway.Wait()
			// The blocked generation lookup cannot prevent the receipt being usable.
			read, err := sql.Open("sqlite", filepath.Join(dir, "tollglass.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			var original []byte
			var source, cost, provider string
			var findings bool
			deadline := time.Now().Add(time.Second)
			for {
				err = read.QueryRow("SELECT envelope,cost_source,cost_amount,provider,has_findings FROM receipts").Scan(&original, &source, &cost, &provider, &findings)
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(time.Millisecond)
			}
			if cost != "0.001500" || source != "reported" || provider != "Infermatic" || findings {
				t.Fatal("initial summary", cost, provider, findings)
			}
			close(allow)
			worker.Wait()
			db.Close()
			var after []byte
			read.QueryRow("SELECT envelope,cost_source,cost_amount,provider,has_findings FROM receipts").Scan(&after, &source, &cost, &provider, &findings)
			if !bytes.Equal(original, after) {
				t.Fatal("receipt mutated")
			}
			if _, err = receipt.Verify(after, pub); err != nil {
				t.Fatal(err)
			}
			var count int
			read.QueryRow("SELECT count(*) FROM supplements").Scan(&count)
			if mode == "delayed" {
				if count != 1 || attempts.Load() != 2 || !findings {
					t.Fatal("enrichment missing", count, attempts.Load(), findings)
				}
				var signed []byte
				read.QueryRow("SELECT envelope FROM supplements").Scan(&signed)
				var e receipt.Envelope
				json.Unmarshal(signed, &e)
				p, _ := base64.StdEncoding.DecodeString(e.Payload)
				var s receipt.Supplement
				json.Unmarshal(p, &s)
				var re receipt.Envelope
				json.Unmarshal(original, &re)
				rp, _ := base64.StdEncoding.DecodeString(re.Payload)
				if s.ReceiptSHA256 != receipt.Digest(rp) {
					t.Fatal("wrong receipt digest")
				}
			} else if count != 0 || findings || (mode == "disabled" && attempts.Load() != 0) || (mode == "exhausted" && attempts.Load() != 3) {
				t.Fatal("unexpected enrichment", count, attempts.Load())
			}
			all := append(original, after...)
			if bytes.Contains(all, []byte("synthetic-private-marker")) || logs.Len() != 0 {
				t.Fatal("credential or failed lookup leaked")
			}
		})
	}
}
func TestOptionalChatProviderAndReportedCost(t *testing.T) {
	for _, present := range []bool{false, true} {
		raw := fixture(t, "openrouter-chat.sse")
		if !present {
			raw = bytes.ReplaceAll(raw, []byte(`,"provider":"Infermatic"`), nil)
		}
		_, rec, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write(raw)
		}, false)
		call(t, srv, []byte(`{"model":"test","stream":true}`), "")
		r := take(t, rec).r
		if (r.Response.Provider != nil) != present || r.Cost.Amount == nil || *r.Cost.Amount != "0.001500" || r.Cost.Source != "reported" {
			t.Fatal("optional provider or decimal cost", r)
		}
	}
}

func TestOpenRouterReportedZeroAndMissingCost(t *testing.T) {
	for _, cost := range []string{"0", "1.23456789012345e-6", "null"} {
		raw := bytes.ReplaceAll(fixture(t, "openrouter-chat.sse"), []byte("0.001500"), []byte(cost))
		us, _ := upstream.Build(nil)
		rec := &recorder{ch: make(chan captured, 1)}
		g := &Gateway{Upstreams: us, Store: rec, Transport: injectedTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
		})}
		srv := httptest.NewServer(g)
		resp, err := http.Post(srv.URL+"/openrouter/v1/chat/completions", "application/json", strings.NewReader(`{"model":"test","stream":true}`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		r := take(t, rec).r
		srv.Close()
		if cost == "null" {
			if r.Cost.Amount != nil || r.Cost.Source != "unknown" || !finding(r, "price_unknown") {
				t.Fatal("unknown cost", r.Cost)
			}
		} else {
			if r.Cost.Amount == nil || *r.Cost.Amount != cost || r.Cost.Source != "reported" || finding(r, "price_unknown") {
				t.Fatal("reported decimal", r.Cost)
			}
		}
		if r.Cost.Estimate != nil || r.Cost.PriceTable != nil {
			t.Fatal("router was estimated")
		}
	}
}
