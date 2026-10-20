package proxy

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincentsch/tollglass/internal/receipt"
)

func TestAdapterCaptureThroughProxy(t *testing.T) {
	cases := []struct {
		api, path, file         string
		complete, providerError bool
	}{
		{"anthropic", "/v1/messages", "messages.json", true, false},
		{"anthropic", "/v1/messages", "messages.sse", true, false},
		{"anthropic", "/v1/messages", "messages-error.sse", true, true},
		{"openai", "/v1/responses", "responses.json", true, false},
		{"openai", "/v1/responses", "responses-completed.sse", true, false},
		{"openai", "/v1/responses", "responses-incomplete.sse", true, false},
		{"openai", "/v1/responses", "responses-failed.sse", true, true},
		{"openai", "/v1/responses", "responses-error.sse", true, true},
		{"openai", "/v1/responses", "responses-nested-error.sse", true, true},
		{"openai", "/v1/embeddings", "embeddings.json", true, false},
	}
	for _, tc := range cases {
		for _, mode := range []string{"plain", "gzip", "cutoff", "done-trailer"} {
			if (mode == "cutoff" || mode == "done-trailer") && !strings.HasSuffix(tc.file, "sse") {
				continue
			}
			t.Run(tc.file+"/"+mode, func(t *testing.T) {
				raw := fixture(t, tc.file)
				complete := tc.complete
				if mode == "done-trailer" {
					raw = append(raw, []byte("data: [DONE]\n\n")...)
				}
				if mode == "cutoff" {
					raw = raw[:bytes.LastIndex(raw, []byte("data:"))+15]
					complete = false
				}
				wire := raw
				if mode == "gzip" {
					var b bytes.Buffer
					z := gzip.NewWriter(&b)
					z.Write(raw)
					z.Close()
					wire = b.Bytes()
				}
				body := []byte(`{ "model":"example", "stream":true, "stream_options":{"include_usage":false}, "reasoning_effort":"low", "reasoning":{"effort":"high"} }`)
				g, s, srv := gateway(t, func(w http.ResponseWriter, q *http.Request) {
					got, _ := io.ReadAll(q.Body)
					if !bytes.Equal(got, body) || q.URL.Path != tc.path || q.Header.Get("X-Api-Key") != "synthetic-secret-marker" {
						t.Error("request bytes, auth or path changed")
					}
					if strings.HasSuffix(tc.file, "sse") {
						w.Header().Set("Content-Type", "text/event-stream")
					}
					if mode == "gzip" {
						w.Header().Set("Content-Encoding", "gzip")
					}
					for i := 0; i < len(wire); i += 1 {
						w.Write(wire[i : i+1])
						w.(http.Flusher).Flush()
					}
				}, false)
				u := g.Upstreams["local"]
				u.API = tc.api
				u.InjectUsage = true
				g.Upstreams["local"] = u
				var logs bytes.Buffer
				g.Logger = log.New(&logs, "", 0)
				q, _ := http.NewRequest("POST", srv.URL+"/local"+tc.path, bytes.NewReader(body))
				q.Header.Set("X-Api-Key", "synthetic-secret-marker")
				client := &http.Client{Transport: Transport()}
				defer client.CloseIdleConnections()
				resp, err := client.Do(q)
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, wire) {
					t.Fatal("response bytes changed")
				}
				v := take(t, s)
				r := v.r
				if r.Response.Complete != complete || finding(r, "stream_interrupted") == complete {
					t.Fatal("capture completeness", r.Response.Complete, r.Findings)
				}
				if r.Request.BodySHA256 != sha(body) || r.Response.BodySHA256 != sha(wire) || len(r.Request.Modifications) != 0 || v.c != nil {
					t.Fatal("hash, injection or content contract")
				}
				if complete && finding(r, "upstream_error") != tc.providerError {
					t.Fatal("provider failure mapping")
				}
				if mode == "cutoff" && strings.Contains(tc.file, "error") && finding(r, "upstream_error") {
					t.Fatal("partial error frame treated as complete error")
				}
				if r.Usage.Input == nil && !(tc.path == "/v1/responses" && strings.HasSuffix(tc.file, "error.sse")) && (mode != "cutoff" || tc.api == "anthropic") {
					t.Fatal("prior usage lost")
				}
				if mode == "done-trailer" && finding(r, "metadata_unavailable") {
					t.Fatal("trailer caused false finding")
				}
				if tc.path == "/v1/responses" && r.Request.Params["reasoning_effort"] != "high" {
					t.Fatal("wrong Responses reasoning effort")
				}
				if complete && strings.HasSuffix(tc.file, "error.sse") && tc.path == "/v1/responses" {
					expected := `{"type":null,"code":503}`
					if tc.file == "responses-nested-error.sse" {
						expected = `{"type":"invalid_request_error","code":"rate_limit_exceeded"}`
					}
					b, _ := json.Marshal(r.Response.Error)
					if string(b) != expected {
						t.Fatal("error metadata", string(b))
					}
				}

				metadata, _ := json.Marshal(r)
				if bytes.Contains(metadata, []byte("sensitive")) || bytes.Contains(metadata, []byte("synthetic-secret-marker")) || logs.Len() != 0 {
					t.Fatal("sensitive data in receipt/logs")
				}
				pub, key, _ := ed25519.GenerateKey(rand.Reader)
				r.Instance.KeyID = receipt.KeyID(pub)
				env, err := receipt.Sign(r, key)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = receipt.Verify(env, pub); err != nil {
					t.Fatal(err)
				}
				g.Wait()
				if len(s.ch) != 0 {
					t.Fatal("duplicate receipt")
				}
			})
		}
	}
}

func TestAdapterTransportFailureAndPresetRouting(t *testing.T) {
	for _, tc := range []struct{ name, path, api string }{
		{"anthropic", "/v1/messages", "anthropic.messages"},
		{"openai", "/v1/responses", "openai.responses"},
		{"openai", "/v1/embeddings", "openai.embeddings"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			g, s, _ := gateway(t, func(http.ResponseWriter, *http.Request) {}, false)
			g.Transport = roundTripFunc(func(q *http.Request) (*http.Response, error) {
				if q.URL.String() != g.Upstreams[tc.name].URL.String()+tc.path {
					t.Error("preset URL", q.URL)
				}
				return nil, fmt.Errorf("synthetic-secret-marker")
			})
			g.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "http://localhost/"+tc.name+tc.path, strings.NewReader(`{}`)))
			r := take(t, s).r
			if r.Request.API != tc.api || r.Response.Complete || r.Response.Status != nil || r.Time.FirstByte != nil || !finding(r, "transport_error") || len(s.ch) != 0 {
				t.Fatal("transport finalization")
			}
		})
	}
}

func TestStreamFramingAfterMetadataFailure(t *testing.T) {
	for _, tc := range []struct {
		api, path, body       string
		complete, unavailable bool
	}{
		{"openai", "/v1/responses", "data: [DONE]\n\n", false, false},
		{"openai", "/v1/responses", "data: not-json\n\ndata: {\"type\":\"response.completed\"}\n\n", true, true},
		{"anthropic", "/v1/messages", "data: not-json\n\ndata: {\"type\":\"message_stop\"}\n\n", true, true},
	} {
		t.Run(tc.api+tc.path, func(t *testing.T) {
			g, s, srv := gateway(t, func(w http.ResponseWriter, q *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, tc.body)
			}, false)
			u := g.Upstreams["local"]
			u.API = tc.api
			g.Upstreams["local"] = u
			resp, err := http.Post(srv.URL+"/local"+tc.path, "application/json", strings.NewReader(`{"stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(got) != tc.body {
				t.Fatal("forwarding changed")
			}
			r := take(t, s).r
			if r.Response.Complete != tc.complete || finding(r, "stream_interrupted") == tc.complete || finding(r, "metadata_unavailable") != tc.unavailable {
				t.Fatal("incorrect capture findings", r.Response, r.Findings)
			}
		})
	}
}
