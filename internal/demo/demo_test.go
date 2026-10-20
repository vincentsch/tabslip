package demo_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/demo"
	"github.com/vincentsch/tollglass/internal/enrich"
	"github.com/vincentsch/tollglass/internal/proxy"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
)

func TestOfflineDemoMixAndLaterSDKModel(t *testing.T) {
	// Any accidental default outbound HTTP call fails immediately.
	previous := http.DefaultTransport
	http.DefaultTransport = &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		t.Error("outbound connection attempted")
		return nil, io.ErrClosedPipe
	}}
	defer func() { http.DefaultTransport = previous }()
	dir := t.TempDir()
	db, err := store.Open(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("http://demo.invalid")
	worker := &enrich.Worker{Lookup: demo.Lookup, Save: db.SaveSupplement}
	g := &proxy.Gateway{Upstreams: map[string]upstream.Upstream{"demo": {Name: "demo", URL: target, API: "openai"}}, Config: config.Default(), KeyID: db.KeyID, Store: db, Transport: demo.Transport{}, Demo: true, Enrich: worker.Schedule}
	server := httptest.NewServer(g)
	defer server.Close()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	if err = demo.Seed(context.Background(), client, server.URL); err != nil {
		t.Fatal(err)
	}
	g.Wait()
	worker.Wait()
	// A ninth, manually sent call accepts normal SDK model names and ignores keys.
	req, _ := http.NewRequest("POST", server.URL+"/demo/v1/chat/completions", strings.NewReader(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer ignored-synthetic-key")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || !bytes.Contains(body, []byte("gpt-4.1")) {
		t.Fatal("manual call", err)
	}
	g.Wait()
	worker.Wait()
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	read, err := sql.Open("sqlite", filepath.Join(dir, "tollglass.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	rows, err := read.Query("SELECT envelope FROM receipts ORDER BY started_at,id")
	if err != nil {
		t.Fatal(err)
	}
	var records []receipt.Receipt
	for rows.Next() {
		var b []byte
		rows.Scan(&b)
		var e receipt.Envelope
		json.Unmarshal(b, &e)
		p, _ := base64.StdEncoding.DecodeString(e.Payload)
		var r receipt.Receipt
		json.Unmarshal(p, &r)
		records = append(records, r)
	}
	rows.Close()
	if len(records) != 9 {
		t.Fatal("receipt count", len(records))
	}
	counts := map[string]int{}
	tags := map[string]int{}
	streamed := 0
	fallback := ""
	for _, r := range records {
		counts[r.Request.API]++
		tags[r.Request.Tag]++
		if r.Request.Stream != nil && *r.Request.Stream {
			streamed++
		}
		if r.Cost.PriceTable != nil {
			t.Fatal("demo price table estimate")
		}
		if r.Request.Model != nil && *r.Request.Model == "demo/large" {
			fallback = r.ID
			if _, ok := r.Request.Params["models"]; ok {
				t.Fatal("invented fallback list")
			}
			found := false
			for _, f := range r.Findings {
				if f.Code == "model_differs" {
					found = true
				}
				if f.Code == "fallback_used" {
					t.Fatal("attempt finding in base receipt")
				}
			}
			if !found {
				t.Fatal("missing model mismatch")
			}
		}
		for _, f := range r.Findings {
			if f.Code == "price_unknown" {
				t.Fatal("demo unknown price warning")
			}
		}
	}
	if counts["openai.chat"] != 6 || counts["openai.embeddings"] != 1 || counts["anthropic.messages"] != 2 || streamed != 2 || tags["acme-shop"] != 4 || tags["blog-tool"] != 4 {
		t.Fatal("demo mix", counts, tags, streamed)
	}
	var count int
	read.QueryRow("SELECT count(*) FROM supplements").Scan(&count)
	if count != 1 || fallback == "" {
		t.Fatal("fallback supplement", count)
	}
	var cost, source, provider string
	var flag bool
	read.QueryRow("SELECT cost_amount,cost_source,provider,has_findings FROM receipts WHERE id=?", fallback).Scan(&cost, &source, &provider, &flag)
	if cost != "0.000123" || source != "reported" || provider != demo.Provider || !flag {
		t.Fatal("supplement summary", cost, source, provider, flag)
	}
	var senv []byte
	read.QueryRow("SELECT envelope FROM supplements WHERE receipt_id=?", fallback).Scan(&senv)
	if !bytes.Contains(senv, []byte("payload")) {
		t.Fatal("signed supplement missing")
	}
}
