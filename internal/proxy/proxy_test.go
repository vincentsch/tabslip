package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/vincentsch/tabslip/internal/apis"
	"github.com/vincentsch/tabslip/internal/config"
	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/store"
	"github.com/vincentsch/tabslip/internal/upstream"
)

type captured struct {
	r *receipt.Receipt
	c *store.Content
}
type recorder struct{ ch chan captured }

func (s *recorder) Save(r *receipt.Receipt, c *store.Content) { s.ch <- captured{r, c} }
func (s *recorder) Unrecorded() int64                         { return 7 }
func take(t *testing.T, s *recorder) captured {
	t.Helper()
	select {
	case v := <-s.ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("receipt not finalized")
		return captured{}
	}
}
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("../../testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func gateway(t *testing.T, h http.HandlerFunc, keep bool) (*Gateway, *recorder, *httptest.Server) {
	t.Helper()
	up := httptest.NewServer(h)
	t.Cleanup(up.Close)
	us, e := upstream.Build([]upstream.Config{{Name: "local", BaseURL: up.URL, API: "openai"}})
	if e != nil {
		t.Fatal(e)
	}
	s := &recorder{make(chan captured, 10)}
	c := config.Default()
	c.KeepContent = keep
	g := &Gateway{Upstreams: us, Config: c, KeyID: "ts_test", Store: s, Transport: Transport()}
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	return g, s, srv
}
func call(t *testing.T, srv *httptest.Server, body []byte, tag string) []byte {
	t.Helper()
	q, _ := http.NewRequest("POST", srv.URL+"/local/v1/chat/completions?test=1", bytes.NewReader(body))
	q.Header.Set("Authorization", "Bearer synthetic-private-marker")
	q.Header.Set("X-Tabslip-Tag", tag)
	q.Header.Set("X-Tabslip-Internal", "omit")
	q.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: Transport()}
	defer client.CloseIdleConnections()
	resp, e := client.Do(q)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func finding(r *receipt.Receipt, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
func TestForwardingAndReceipt(t *testing.T) {
	for _, name := range []string{"chat.json", "chat.sse"} {
		t.Run(name, func(t *testing.T) {
			raw := fixture(t, name)
			stream := strings.HasSuffix(name, "sse")
			request := []byte(`{"model":"gpt-4.1","messages":[],"stream":` + map[bool]string{true: "true", false: "false"}[stream] + `,"temperature":0.2,"response_format":{"type":"json_object"},"tools":[]}`)
			_, s, srv := gateway(t, func(w http.ResponseWriter, q *http.Request) {
				b, _ := io.ReadAll(q.Body)
				if !stream && !bytes.Equal(b, request) {
					t.Error("request changed")
				}
				if q.URL.Path != "/v1/chat/completions" || q.URL.RawQuery != "test=1" || q.Header.Get("Authorization") != "Bearer synthetic-private-marker" || q.Header.Get("X-Tabslip-Tag") != "" || q.Header.Get("X-Tabslip-Internal") != "" {
					t.Error("routing or header contract")
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				for i := 0; i < len(raw); i += 7 {
					end := min(i+7, len(raw))
					w.Write(raw[i:end])
					w.(http.Flusher).Flush()
				}
			}, false)
			got := call(t, srv, request, "acme")
			if !bytes.Equal(got, raw) {
				t.Fatal("response bytes changed")
			}
			r := take(t, s).r
			if r.Request.BodySHA256 != sha(request) || r.Response.BodySHA256 != sha(raw) || r.Request.BodyBytes != int64(len(request)) || r.Response.BodyBytes != int64(len(raw)) {
				t.Fatal("incorrect hashes or sizes")
			}
			if !r.Response.Complete || r.Response.Status == nil || *r.Response.Status != 200 || r.Response.Model == nil || *r.Response.Model != "gpt-4.1-2025-04-14" || r.Usage.Input == nil || *r.Usage.Input != 19 || r.Usage.Output == nil || *r.Usage.Output != 10 || r.Usage.Cached == nil || *r.Usage.Cached != 0 || r.Usage.CacheWrite != nil || r.Time.FirstByte == nil || r.Time.TTFB == nil {
				t.Fatalf("receipt mapping: %+v", r)
			}
			if !finding(r, "model_alias_resolved") || r.HasFindings() || r.Cost.Source != "unknown" || r.Cost.Currency != nil || r.Response.APIStatus != nil || r.Response.Error != nil {
				t.Fatal("finding/null contract")
			}
			if r.Request.Params["response_format"] != "json_object" || r.Request.Params["tools"] != 0 {
				t.Fatal("request parameter mapping")
			}
		})
	}
}
func TestInjectionTruthTable(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for index, option := range []string{"", `,"stream_options":{"include_obfuscation":false}`, `,"stream_options":{"include_usage":false,"include_obfuscation":false}`, `,"stream_options":{"include_usage":true,"include_obfuscation":false}`} {
			t.Run(fmt.Sprintf("enabled=%t/option=%d", enabled, index), func(t *testing.T) {
				body := []byte(`{ "model":"gpt-4.1", "stream":true` + option + `}`)
				var forwarded []byte
				g, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
					forwarded, _ = io.ReadAll(r.Body)
					w.Write(fixture(t, "chat.json"))
				}, false)
				u := g.Upstreams["local"]
				u.InjectUsage = enabled
				g.Upstreams["local"] = u
				call(t, srv, body, "")
				r := take(t, s).r
				changed := enabled && !strings.Contains(option, `"include_usage":true`)
				if (len(r.Request.Modifications) > 0) != changed || r.Request.BodySHA256 != sha(body) {
					t.Fatal("edit/hash contract")
				}
				if !changed && !bytes.Equal(body, forwarded) {
					t.Fatal("unchanged body rewritten")
				}
				if changed {
					m, ok := apis.Object(forwarded)
					if !ok || m["stream_options"].(map[string]any)["include_usage"] != true {
						t.Fatal("missing injection")
					}
					if strings.Contains(option, "include_obfuscation") && m["stream_options"].(map[string]any)["include_obfuscation"] != false {
						t.Fatal("sibling lost")
					}
				}
			})
		}
	}
}
func TestCompressionTap(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate", "br", "broken-gzip"} {
		for _, accept := range []string{"", "gzip, deflate, br"} {
			t.Run(encoding+accept, func(t *testing.T) {
				plain := fixture(t, "chat.json")
				var compressed bytes.Buffer
				var zw io.WriteCloser
				switch encoding {
				case "gzip":
					zw = gzip.NewWriter(&compressed)
				case "deflate":
					zw = zlib.NewWriter(&compressed)
				case "br":
					zw = brotli.NewWriter(&compressed)
				}
				if zw != nil {
					zw.Write(plain)
					zw.Close()
				} else {
					compressed.WriteString("invalid compressed bytes")
				}
				raw := compressed.Bytes()
				_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Accept-Encoding") != accept {
						t.Error("encoding negotiation changed")
					}
					w.Header().Set("Content-Encoding", strings.TrimPrefix(encoding, "broken-"))
					w.Write(raw)
				}, true)
				q, _ := http.NewRequest("POST", srv.URL+"/local/v1/chat/completions", strings.NewReader(`{"model":"gpt-4.1"}`))
				if accept != "" {
					q.Header.Set("Accept-Encoding", accept)
				}
				client := &http.Client{Transport: Transport()}
				defer client.CloseIdleConnections()
				resp, e := client.Do(q)
				if e != nil {
					t.Fatal(e)
				}
				got, e := io.ReadAll(resp.Body)
				resp.Body.Close()
				if e != nil || !bytes.Equal(got, raw) {
					t.Fatal("compressed bytes changed")
				}
				v := take(t, s)
				if v.r.Response.BodySHA256 != sha(raw) || !v.r.Response.Complete {
					t.Fatal("compressed capture incomplete")
				}
				if encoding == "broken-gzip" {
					if !finding(v.r, "metadata_unavailable") {
						t.Fatal("decoder failure missing")
					}
				} else if v.r.Usage.Input == nil || !bytes.Equal(v.c.Response, plain) {
					t.Fatal("metadata or local decoded content missing")
				}
			})
		}
	}
}
func TestStreamArrivesBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"model\":\"gpt-4.1\",\"choices\":[]}\n\n"))
		w.(http.Flusher).Flush()
		<-release
		w.Write([]byte("data: [DONE]\n\n"))
	}, false)
	resp, e := http.Post(srv.URL+"/local/v1/chat/completions", "application/json", strings.NewReader(`{"stream":true}`))
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	got := make(chan error, 1)
	go func() { _, e := bufio.NewReader(resp.Body).ReadString('\n'); got <- e }()
	select {
	case e := <-got:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("first event was buffered")
	}
	select {
	case <-s.ch:
		t.Fatal("stream already finalized")
	default:
	}
}
func TestLimitsAndInvalidMetadata(t *testing.T) {
	tool := `data: {"model":"gpt-4.1","choices":[{"delta":{"tool_calls":[{"function":{"arguments":"` + strings.Repeat("a", 128*1024) + `"}}]}}]}` + "\n\ndata: [DONE]\n\n"
	cases := []struct {
		name, body string
		request    []byte
		stream     bool
		bad        bool
	}{{"large tool", tool, []byte(`{"stream":true}`), true, false}, {"invalid response", `{invalid`, []byte(`{}`), false, true}, {"oversized response", `{"model":"` + strings.Repeat("x", apis.Limit) + `"}`, []byte(`{}`), false, true}, {"oversized stream", "data: {\"model\":\"known\"}\n\ndata: " + strings.Repeat("x", apis.Limit) + "\n\ndata: [DONE]\n\n", []byte(`{"stream":true}`), true, true}, {"invalid request", string(fixture(t, "chat.json")), []byte(`{bad`), false, true}, {"oversized request", string(fixture(t, "chat.json")), []byte(`{"stream":true,"messages":"` + strings.Repeat("x", apis.Limit) + `"}`), false, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var forwarded []byte
			_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
				forwarded, _ = io.ReadAll(r.Body)
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				w.Write([]byte(tc.body))
			}, true)
			got := call(t, srv, tc.request, "")
			v := take(t, s)
			if !bytes.Equal(got, []byte(tc.body)) || v.r.Response.BodySHA256 != sha(got) || v.r.Request.BodySHA256 != sha(tc.request) {
				t.Fatal("bytes or hashes lost")
			}
			if tc.bad != finding(v.r, "metadata_unavailable") {
				t.Fatal("metadata availability finding")
			}
			if strings.Contains(tc.name, "request") && !bytes.Equal(tc.request, forwarded) {
				t.Fatal("invalid request edited")
			}
			if len(v.c.Request) > apis.Limit || len(v.c.Response) > apis.Limit {
				t.Fatal("unbounded content")
			}
			if strings.HasPrefix(tc.name, "oversized") && !v.c.Truncated {
				t.Fatal("truncation not recorded")
			}
			if tc.name == "large tool" && len(v.c.Response) != 128*1024 {
				t.Fatal("tool arguments lost")
			}
			if tc.name == "oversized stream" && (!v.r.Response.Complete || v.r.Response.Model == nil || *v.r.Response.Model != "known") {
				t.Fatal("known metadata lost")
			}
		})
	}
}
func TestErrorsAndContentPolicy(t *testing.T) {
	raw := []byte(`{"error":{"type":"invalid_request_error","code":1234567890123456789,"message":"private prompt synthetic-private-marker"}}`)
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[keep], func(t *testing.T) {
			g, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400); w.Write(raw) }, keep)
			var logs bytes.Buffer
			g.Logger = log.New(&logs, "", 0)
			got := call(t, srv, []byte(`{"model":"gpt-4.1"}`), "")
			v := take(t, s)
			b, _ := json.Marshal(v.r)
			if !bytes.Equal(got, raw) || bytes.Contains(b, []byte("private prompt")) || bytes.Contains(b, []byte("synthetic-private-marker")) || logs.Len() > 0 {
				t.Fatal("error privacy or passthrough")
			}
			if v.r.Response.Error == nil || v.r.Response.Error.Code != json.Number("1234567890123456789") || !finding(v.r, "upstream_error") || !v.r.Response.Complete {
				t.Fatalf("error mapping: response=%+v error=%+v findings=%+v", v.r.Response, v.r.Response.Error, v.r.Findings)
			}
			if keep {
				if v.c == nil || !bytes.Equal(v.c.Response, raw) {
					t.Fatal("local error body missing")
				}
			} else if v.c != nil {
				t.Fatal("content stored")
			}
		})
	}
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t, "chat.json")) }, false)
	srv.Config.Handler.(*Gateway).Config.KeepContentTags = []string{"acme"}
	call(t, srv, []byte(`{}`), "acme")
	if take(t, s).c == nil {
		t.Fatal("tag content not captured")
	}
	call(t, srv, []byte(`{}`), "other")
	if take(t, s).c != nil {
		t.Fatal("tag policy leaked")
	}
}
func TestEarlyEndAndDisconnect(t *testing.T) {
	t.Run("early end", func(t *testing.T) {
		_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"model\":\"gpt-4o\"}\n\n"))
		}, false)
		call(t, srv, []byte(`{"model":"gpt-4.1","stream":true}`), "")
		r := take(t, s).r
		if r.Response.Complete || !finding(r, "stream_interrupted") || !finding(r, "model_differs") {
			t.Fatal("early end missing")
		}
		if finding(r, "transport_error") || findingText(r, "stream_interrupted") != "The upstream stream ended before its final event." {
			t.Fatal("upstream interruption cause missing")
		}
	})
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnect/stream=%t", stream), func(t *testing.T) {
			_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Write([]byte("data: {\"model\":\"gpt-4.1\"}\n\n"))
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"model":"gpt-4.1","choices":[`))
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}, false)
			resp, e := http.Post(srv.URL+"/local/v1/chat/completions", "application/json", strings.NewReader(fmt.Sprintf(`{"stream":%t}`, stream)))
			if e != nil {
				t.Fatal(e)
			}
			resp.Body.Close()
			requireClientInterruption(t, take(t, s).r)
			select {
			case <-s.ch:
				t.Fatal("duplicate receipt")
			default:
			}
		})
	}

	t.Run("copy failure", func(t *testing.T) {
		g, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.Write([]byte(`{"model":"gpt-4.1"}`))
			w.(http.Flusher).Flush()
		}, false)
		var logs bytes.Buffer
		g.Logger = log.New(&logs, "", 0)
		srv.Config.ErrorLog = log.New(&logs, "", 0)
		resp, e := http.Post(srv.URL+"/local/v1/chat/completions", "application/json", strings.NewReader(`{}`))
		if e == nil {
			_, e = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if e == nil {
			t.Fatal("HTTP abort swallowed")
		}
		r := take(t, s).r
		if r.Response.Complete || r.Response.Status == nil || !finding(r, "transport_error") || logs.Len() > 0 {
			t.Fatal("copy error receipt or raw logs")
		}
		select {
		case <-s.ch:
			t.Fatal("duplicate receipt")
		default:
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPresetRoutingAndUntrackedPaths(t *testing.T) {
	for _, name := range []string{"openai", "openrouter", "ollama", "anthropic"} {
		t.Run(name, func(t *testing.T) {
			us, _ := upstream.Build(nil)
			s := &recorder{make(chan captured, 10)}
			g := &Gateway{Upstreams: us, Store: s, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				want := us[name].URL.String() + "/v1/chat/completions"
				if r.URL.String() != want || r.Host != us[name].URL.Host {
					t.Errorf("route %s != %s", r.URL, want)
				}
				body, _ := io.ReadAll(r.Body)
				if name != "anthropic" {
					if bytes.Contains(body, []byte(`"include_usage":true`)) != (name != "openrouter") {
						t.Error("preset injection")
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(fixture(t, "chat.json")))}, nil
			})}
			q := httptest.NewRequest("POST", "http://localhost/"+name+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
			g.ServeHTTP(httptest.NewRecorder(), q)
			if name != "anthropic" {
				take(t, s)
			} else if len(s.ch) != 0 {
				t.Fatal("unsupported endpoint recorded")
			}
		})
	}
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("models")) }, false)
	resp, e := http.Get(srv.URL + "/local/v1/models")
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if len(s.ch) != 0 {
		t.Fatal("untracked path recorded")
	}
	resp, e = http.Get(srv.URL + "/missing/v1/models")
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 404 || !bytes.Contains(b, []byte("local")) {
		t.Fatal("unknown upstream error")
	}
}
func TestTransportFailures(t *testing.T) {
	for _, mode := range []string{"connect", "tls", "cancel", "read"} {
		t.Run(mode, func(t *testing.T) {
			g, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {}, false)
			var logs bytes.Buffer
			g.Logger = log.New(&logs, "", 0)
			var target string
			switch mode {
			case "connect":
				ln, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				target = "http://" + ln.Addr().String()
				ln.Close()
			case "tls":
				tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				tlsSrv.Config.ErrorLog = log.New(io.Discard, "", 0)
				defer tlsSrv.Close()
				target = tlsSrv.URL
			case "read":
				g.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("synthetic-private-marker upstream reset")
				})
			case "cancel":
				g.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, context.Canceled })
			}
			if target != "" {
				u := g.Upstreams["local"]
				u.URL, _ = u.URL.Parse(target)
				g.Upstreams["local"] = u
			}
			call(t, srv, []byte(`{}`), "")
			r := take(t, s).r
			if r.Response.Status != nil || r.Time.FirstByte != nil || r.Time.TTFB != nil || r.Response.Complete || !finding(r, "transport_error") {
				t.Fatal("transport failure contract")
			}
			b, _ := json.Marshal(r)
			if bytes.Contains(b, []byte("synthetic-private-marker")) || strings.Contains(logs.String(), "synthetic-private-marker") {
				t.Fatal("raw error retained")
			}
		})
	}
}
func TestLoopbackHealth(t *testing.T) {
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t, "chat.json")) }, false)
	g := srv.Config.Handler.(*Gateway)
	q := httptest.NewRequest("GET", "http://localhost/healthz", nil)
	q.RemoteAddr = "203.0.113.1:5000"
	w := httptest.NewRecorder()
	g.ServeHTTP(w, q)
	if w.Code != 403 {
		t.Fatal("remote health exposed")
	}
	q.RemoteAddr = "127.0.0.1:5000"
	w = httptest.NewRecorder()
	g.ServeHTTP(w, q)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unrecorded_calls":7`) {
		t.Fatal("health fields")
	}
	if len(s.ch) != 0 {
		t.Fatal("health recorded")
	}
}

type persistedRecorder struct {
	target  *store.Store
	capture *recorder
}

func (p *persistedRecorder) Save(r *receipt.Receipt, c *store.Content) {
	p.target.Save(r, c)
	p.capture.Save(r, c)
}
func (p *persistedRecorder) Unrecorded() int64 { return p.target.Unrecorded() }
func TestFailuresAndPrivateErrorsReachSQLite(t *testing.T) {
	for _, mode := range []string{"copy failure", "disconnect", "JSON disconnect", "cancel before headers", "private error off", "private error on"} {
		t.Run(mode, func(t *testing.T) {
			seen := make(chan struct{})
			var once sync.Once
			g, capture, srv := gateway(t, func(w http.ResponseWriter, q *http.Request) {
				io.Copy(io.Discard, q.Body)
				once.Do(func() { close(seen) })
				switch mode {
				case "copy failure":
					w.Header().Set("Content-Length", "1000")
					w.Write([]byte(`{"model":"gpt-4.1"}`))
					w.(http.Flusher).Flush()
				case "disconnect", "JSON disconnect":
					if mode == "disconnect" {
						w.Header().Set("Content-Type", "text/event-stream")
						w.Write([]byte("data: {\"model\":\"gpt-4.1\"}\n\n"))
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.Write([]byte(`{"model":"gpt-4.1","choices":[`))
					}
					w.(http.Flusher).Flush()
					<-q.Context().Done()
				case "cancel before headers":
					<-q.Context().Done()
				default:
					w.WriteHeader(400)
					w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"invalid","message":"private-prompt-and-synthetic-key"}}`))
				}
			}, mode == "private error on")
			dir := t.TempDir()
			var logs bytes.Buffer
			db, e := store.Open(dir, 0, log.New(&logs, "", 0))
			if e != nil {
				t.Fatal(e)
			}
			g.Store = &persistedRecorder{db, capture}
			g.KeyID = db.KeyID
			g.Logger = log.New(&logs, "", 0)
			srv.Config.ErrorLog = log.New(&logs, "", 0)
			q, _ := http.NewRequest("POST", srv.URL+"/local/v1/chat/completions", strings.NewReader(`{"model":"gpt-4.1","messages":[{"role":"user","content":"private-prompt-and-synthetic-key"}]}`))
			q.Header.Set("Authorization", "Bearer header-only-private-credential")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			q = q.WithContext(ctx)
			if mode == "cancel before headers" {
				go func() { <-seen; cancel() }()
			}
			resp, e := http.DefaultClient.Do(q)
			if e == nil {
				if mode == "disconnect" || mode == "JSON disconnect" {
					resp.Body.Close()
				} else {
					_, e = io.ReadAll(resp.Body)
					resp.Body.Close()
				}
			}
			if mode == "copy failure" && e == nil {
				t.Fatal("HTTP abort not preserved")
			}
			v := take(t, capture)
			if mode == "cancel before headers" && (v.r.Response.Status != nil || v.r.Time.FirstByte != nil || !finding(v.r, "transport_error")) {
				t.Fatal("cancellation contract")
			}
			if mode == "disconnect" || mode == "JSON disconnect" {
				requireClientInterruption(t, v.r)
			}
			if mode == "disconnect" || mode == "JSON disconnect" || mode == "copy failure" || mode == "cancel before headers" {
				if v.r.Response.Complete {
					t.Fatal("failed call complete")
				}
			}
			db.Close()
			read, e := sql.Open("sqlite", filepath.Join(dir, "tabslip.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer read.Close()
			var n int
			read.QueryRow("SELECT count(*) FROM receipts").Scan(&n)
			if n != 1 {
				t.Fatalf("stored %d receipts", n)
			}
			var envelope []byte
			if e = read.QueryRow("SELECT envelope FROM receipts").Scan(&envelope); e != nil {
				t.Fatal(e)
			}
			payload, e := receipt.Verify(envelope, db.Key.Public().(ed25519.PublicKey))
			if e != nil {
				t.Fatal(e)
			}
			if mode == "disconnect" || mode == "JSON disconnect" {
				var stored receipt.Receipt
				if e = json.Unmarshal(payload, &stored); e != nil {
					t.Fatal(e)
				}
				requireClientInterruption(t, &stored)
			}
			if bytes.Contains(envelope, []byte("private-prompt-and-synthetic-key")) || strings.Contains(logs.String(), "header-only-private-credential") || strings.Contains(logs.String(), "private-prompt-and-synthetic-key") {
				t.Fatal("private text reached receipt or logs")
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "tabslip.db"))
			if bytes.Contains(raw, []byte("header-only-private-credential")) {
				t.Fatal("header credential persisted")
			}
			if mode != "private error on" && bytes.Contains(raw, []byte("private-prompt-and-synthetic-key")) {
				t.Fatal("content persisted when disabled")
			}
			read.QueryRow("SELECT count(*) FROM contents").Scan(&n)
			if mode == "private error on" {
				if n != 1 {
					t.Fatal("local content missing")
				}
				var body []byte
				read.QueryRow("SELECT response_text FROM contents").Scan(&body)
				if !bytes.Contains(body, []byte("private-prompt-and-synthetic-key")) {
					t.Fatal("error content missing")
				}
			} else if n != 0 {
				t.Fatal("unexpected content row")
			}
		})
	}
}
func TestEscapedPathsAndHeaderRules(t *testing.T) {
	_, _, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/v1/files/a%2Fb?x=1&x=2" || r.Header.Get("Accept-Encoding") != "custom" || r.Header.Get("Cookie") != "session=synthetic" || r.Header.Get("X-Forwarded-For") != "192.0.2.1" || r.Header.Get("X-Remove") != "" || r.Header.Get("X-Tabslip-Tag") != "" {
			t.Errorf("forwarding changed: %s %+v", r.RequestURI, r.Header)
		}
		w.Write([]byte("ok"))
	}, false)
	q, _ := http.NewRequest("GET", srv.URL+"/local/v1/files/a%2Fb?x=1&x=2", nil)
	q.Header.Set("Accept-Encoding", "custom")
	q.Header.Set("Cookie", "session=synthetic")
	q.Header.Set("X-Forwarded-For", "192.0.2.1")
	q.Header.Set("Connection", "X-Remove")
	q.Header.Set("X-Remove", "secret")
	q.Header.Set("X-Tabslip-Tag", "acme")
	resp, e := http.DefaultClient.Do(q)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
}

func TestLargeRequestRejectedBeforeUploadFinishes(t *testing.T) {
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(413)
		w.Write([]byte(`{"error":{"type":"too_large","code":null}}`))
	}, true)
	body := []byte(`{"messages":"` + strings.Repeat("x", apis.Limit+1024*1024) + `"}`)
	call(t, srv, body, "")
	v := take(t, s)
	if v.r.Response.Status == nil || *v.r.Response.Status != 413 || !finding(v.r, "upstream_error") || len(v.c.Request) > apis.Limit {
		t.Fatal("early rejection capture")
	}
}

func TestStreamContentLimitDoesNotStopUsageOrHashes(t *testing.T) {
	event := `data: {"model":"gpt-4.1","choices":[{"delta":{"content":"` + strings.Repeat("x", 1024*1024) + `"}}]}` + "\n\n"
	raw := []byte(strings.Repeat(event, 11) + "data: {\"usage\":{\"prompt_tokens\":19,\"completion_tokens\":10}}\n\ndata: [DONE]\n\n")
	_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(raw)
	}, true)
	got := call(t, srv, []byte(`{"stream":true}`), "")
	v := take(t, s)
	if !bytes.Equal(got, raw) || v.r.Response.BodySHA256 != sha(raw) || !v.r.Response.Complete || v.r.Usage.Input == nil || *v.r.Usage.Input != 19 || finding(v.r, "metadata_unavailable") || !v.c.Truncated || len(v.c.Response) != apis.Limit {
		t.Fatal("content limit affected metadata or forwarding")
	}
}

func TestCompressedStreamsKeepOriginalBytes(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate", "br"} {
		t.Run(encoding, func(t *testing.T) {
			var compressed bytes.Buffer
			var writer io.WriteCloser
			switch encoding {
			case "gzip":
				writer = gzip.NewWriter(&compressed)
			case "deflate":
				writer = zlib.NewWriter(&compressed)
			case "br":
				writer = brotli.NewWriter(&compressed)
			}
			writer.Write(fixture(t, "chat.sse"))
			writer.Close()
			raw := compressed.Bytes()
			_, s, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Content-Encoding", encoding)
				for i := 0; i < len(raw); i += 7 {
					w.Write(raw[i:min(i+7, len(raw))])
					w.(http.Flusher).Flush()
				}
			}, true)
			got := call(t, srv, []byte(`{"stream":true}`), "")
			v := take(t, s)
			if !bytes.Equal(got, raw) || v.r.Response.BodySHA256 != sha(raw) || !v.r.Response.Complete || v.r.Usage.Input == nil || *v.r.Usage.Input != 19 || string(v.c.Response) != "Hello ü" {
				t.Fatal("compressed streaming capture")
			}
		})
	}
}

func findingText(r *receipt.Receipt, code string) string {
	for _, f := range r.Findings {
		if f.Code == code {
			return f.Text
		}
	}
	return ""
}
func requireClientInterruption(t *testing.T, r *receipt.Receipt) {
	t.Helper()
	if r.Response.Complete || r.Response.Status == nil || finding(r, "transport_error") || findingText(r, "stream_interrupted") != "The client disconnected before the response finished." {
		t.Fatalf("client interruption misclassified: response=%+v findings=%+v", r.Response, r.Findings)
	}
}

type failedClientWriter struct{ http.ResponseWriter }

func (w failedClientWriter) Write([]byte) (int, error)   { return 0, io.ErrClosedPipe }
func (w failedClientWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type flushedHeaders struct {
	http.ResponseWriter
	ready chan struct{}
}

func (w flushedHeaders) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	_ = http.NewResponseController(w.ResponseWriter).Flush()
	close(w.ready)
}
func (w flushedHeaders) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type delayedEOFBody struct {
	io.ReadCloser
	remaining int64
	ready     <-chan struct{}
	result    chan<- error
}

func (b *delayedEOFBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		<-b.ready
		n, err := b.ReadCloser.Read(p)
		b.result <- err
		return n, err
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func TestResponseHeadersDoNotCloseForwardingRequestBody(t *testing.T) {
	ready := make(chan struct{})
	result := make(chan error, 1)
	raw := fixture(t, "chat.json")
	g, s, _ := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.(http.Flusher).Flush()
		// Let response headers reach the client before the transport's final
		// request EOF check. This used to make the server close the shared wrapper.
		select {
		case err := <-result:
			if err != io.EOF {
				t.Errorf("final request read = %v, want EOF", err)
			}
		case <-r.Context().Done():
			return
		}
		w.Write(raw)
	}, false)
	transport := g.Transport
	g.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.Body = &delayedEOFBody{r.Body, r.ContentLength, ready, result}
		return transport.RoundTrip(r)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.ServeHTTP(flushedHeaders{w, ready}, r)
	}))
	defer srv.Close()
	got := call(t, srv, []byte(`{"model":"gpt-4.1","messages":[]}`), "")
	if !bytes.Equal(got, raw) {
		t.Fatal("response bytes changed")
	}
	r := take(t, s).r
	if !r.Response.Complete || finding(r, "transport_error") || finding(r, "stream_interrupted") {
		t.Fatal("complete response recorded as interrupted", r.Findings)
	}
}

func TestClientWriteFailureDoesNotBlameUpstream(t *testing.T) {
	g, s, _ := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t, "chat.json")) }, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { g.ServeHTTP(failedClientWriter{w}, r) }))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/local/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("HTTP copy abort swallowed")
	}
	requireClientInterruption(t, take(t, s).r)
}

func TestContentSettingsSnapshotPerCallAndTag(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	raw := fixture(t, "chat.json")
	g, rec, srv := gateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Stall") == "yes" {
			close(started)
			<-release
		}
		w.Write(raw)
	}, false)
	var mu sync.Mutex
	settings := config.Default()
	g.Settings = func() config.Config { mu.Lock(); defer mu.Unlock(); return settings }
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, _ := http.NewRequest("POST", srv.URL+"/local/v1/chat/completions", strings.NewReader(`{"model":"gpt-4.1","messages":[]}`))
		req.Header.Set("X-Stall", "yes")
		resp, err := srv.Client().Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	<-started
	mu.Lock()
	settings.KeepContentTags = []string{"client-a"}
	mu.Unlock()
	close(release)
	<-done
	if take(t, rec).c != nil {
		t.Fatal("setting changed content capture for an in-flight call")
	}
	call(t, srv, []byte(`{"model":"gpt-4.1","messages":[]}`), "client-a")
	if take(t, rec).c == nil {
		t.Fatal("selected tag content missing")
	}
	call(t, srv, []byte(`{"model":"gpt-4.1","messages":[]}`), "client-b")
	if take(t, rec).c != nil {
		t.Fatal("content stored for an unselected tag")
	}
	mu.Lock()
	settings.KeepContent = true
	mu.Unlock()
	call(t, srv, []byte(`{"model":"gpt-4.1","messages":[]}`), "client-b")
	if take(t, rec).c == nil {
		t.Fatal("all-call content missing")
	}
}
