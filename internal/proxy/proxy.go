package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vincentsch/tollglass/internal/apis"
	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
	"github.com/vincentsch/tollglass/internal/version"
)

type Recorder interface {
	Save(*receipt.Receipt, *store.Content)
	Unrecorded() int64
}
type Gateway struct {
	Upstreams map[string]upstream.Upstream
	Config    config.Config
	KeyID     string
	Store     Recorder
	Transport http.RoundTripper
	Logger    *log.Logger
	active    sync.WaitGroup
}

var sharedTransport = Transport()

func Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableCompression = true
	return t
}
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func Loopback(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (g *Gateway) Wait() { g.active.Wait() }
func (g *Gateway) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	g.active.Add(1)
	defer g.active.Done()
	if req.URL.Path == "/healthz" {
		if !Loopback(req) {
			http.Error(w, "Loopback connections only.", 403)
			return
		}
		if req.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		JSON(w, 200, map[string]any{"tool": "tollglass", "version": version.Version, "key_id": g.KeyID, "unrecorded_calls": g.Store.Unrecorded()})
		return
	}
	name, path, found := strings.Cut(strings.TrimPrefix(req.URL.Path, "/"), "/")
	if !found {
		path = ""
	}
	path = "/" + path
	u, ok := g.Upstreams[name]
	if !ok {
		JSON(w, 404, map[string]any{"error": map[string]any{"type": "unknown_upstream", "message": "Choose a configured upstream.", "upstreams": upstream.Names(g.Upstreams)}})
		return
	}
	api := apis.Endpoint(u.API, req.Method, path)
	tracked := api != ""
	var r *receipt.Receipt
	var capture *apis.Capture
	var tap *responseTap
	var content *store.Content
	var reqDigest *digest
	var requestContent *boundedBuffer
	var started time.Time
	transportFailed := false
	returned := false
	responseStream := false
	if tracked {
		started = time.Now()
		r = receipt.New(g.KeyID, started)
		r.Request.Upstream = name
		r.Request.API = api
		r.Request.Method = req.Method
		r.Request.Path = path
		tag := req.Header.Get("X-Tollglass-Tag")
		if tagPattern.MatchString(tag) {
			r.Request.Tag = tag
		}
		keep := g.Config.KeepContent
		for _, t := range g.Config.KeepContentTags {
			if t == r.Request.Tag && t != "" {
				keep = true
			}
		}
		if keep {
			content = &store.Content{}
			requestContent = &boundedBuffer{}
		}
		reqDigest = newDigest()
		capture = &apis.Capture{Receipt: r, Keep: keep}
		defer func() {
			// This also runs during http.ErrAbortHandler unwinding. Leave that panic intact.
			if tap != nil {
				tap.Close()
				r.Response.BodySHA256, r.Response.BodyBytes = tap.digest.Snapshot()
				r.Response.Complete = returned && tap.eof && !tap.readFailed
				if tap.first != nil {
					s := receipt.Timestamp(*tap.first)
					ms := tap.first.Sub(started).Milliseconds()
					r.Time.FirstByte = &s
					r.Time.TTFB = &ms
				}
			} else {
				r.Response.BodySHA256 = newDigest().Sum()
			}
			capturedWholeBody := r.Response.Complete
			if responseStream && !capture.Done {
				r.Response.Complete = false
			}
			if !r.Response.Complete && !transportFailed {
				// Closing a fully read response can cancel the context before parsing finishes.
				clientEnded := !capturedWholeBody && (req.Context().Err() != nil || (tap != nil && !returned && !tap.readFailed))
				if clientEnded {
					r.Find("stream_interrupted", "observed", "The client disconnected before the response finished.")
				} else {
					if tap != nil && tap.readFailed {
						r.Find("transport_error", "observed", "The upstream response could not be read completely.")
					}
					if responseStream && !capture.Done {
						r.Find("stream_interrupted", "observed", "The upstream stream ended before its final event.")
					} else {
						r.Find("stream_interrupted", "observed", "The upstream response ended before it was complete.")
					}
				}
			}
			r.Request.BodySHA256, r.Request.BodyBytes = reqDigest.Snapshot()
			now := time.Now()
			r.Time.Finished = receipt.Timestamp(now)
			r.Time.Total = now.Sub(started).Milliseconds()
			if content != nil {
				content.Request, content.Truncated = requestContent.Snapshot()
				content.Response = capture.Text
				content.Truncated = content.Truncated || capture.Truncated
			}
			apis.Finish(r)
			g.Store.Save(r, content)
		}()
		body := req.Body
		if body == nil {
			body = http.NoBody
		}
		prefix, err := io.ReadAll(io.LimitReader(body, apis.Limit+1))
		reqDigest.Write(prefix)
		if content != nil {
			requestContent.Write(prefix)
		}
		v, valid := apis.Request(r, prefix)
		if err != nil {
			apis.Unavailable(r)
			valid = false
		}
		if valid && api == "openai.chat" && u.InjectUsage && r.Request.Stream != nil && *r.Request.Stream {
			options, exists := v["stream_options"]
			m, ok := options.(map[string]any)
			if !exists || options == nil {
				m = map[string]any{}
				ok = true
			}
			if ok && m["include_usage"] != true {
				m["include_usage"] = true
				v["stream_options"] = m
				edited, e := json.Marshal(v)
				if e == nil {
					req.Body = &joinedBody{Reader: bytes.NewReader(edited), closer: body}
					req.ContentLength = int64(len(edited))
					r.Request.Modifications = append(r.Request.Modifications, "stream_options.include_usage=true")
				}
			} else if !ok {
				apis.Unavailable(r)
			}
		}
		if len(r.Request.Modifications) == 0 {
			var tail io.ReadCloser = body
			rt := &requestTap{ReadCloser: tail, digest: reqDigest}
			if content != nil {
				rt.content = requestContent
			}
			req.Body = &joinedBody{Reader: io.MultiReader(bytes.NewReader(prefix), rt), closer: body}
		}
	}
	transport := g.Transport
	if transport == nil {
		transport = sharedTransport
	}
	proxy := &httputil.ReverseProxy{Transport: transport, FlushInterval: -1, ErrorLog: log.New(io.Discard, "", 0)}
	proxy.Rewrite = func(p *httputil.ProxyRequest) {
		p.SetURL(u.URL)
		p.Out.Host = u.URL.Host
		// Preserve escaping while removing only the selected upstream prefix.
		target := *u.URL
		rest := *p.In.URL
		rest.Path = path
		escaped := p.In.URL.EscapedPath()
		prefix := "/" + url.PathEscape(name)
		rest.RawPath = strings.TrimPrefix(escaped, prefix)
		if rest.RawPath == rest.Path {
			rest.RawPath = ""
		}
		p.Out.URL.Path = strings.TrimRight(target.Path, "/") + rest.Path
		p.Out.URL.RawPath = ""
		if rest.RawPath != "" || target.RawPath != "" {
			p.Out.URL.RawPath = strings.TrimRight(target.EscapedPath(), "/") + rest.EscapedPath()
		}
		p.Out.URL.RawQuery = p.In.URL.RawQuery
		for _, key := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
			hop := false
			for _, value := range p.In.Header.Values("Connection") {
				for _, token := range strings.Split(value, ",") {
					if strings.EqualFold(strings.TrimSpace(token), key) {
						hop = true
					}
				}
			}
			if values, ok := p.In.Header[key]; ok && !hop {
				p.Out.Header[key] = append([]string(nil), values...)
			}
		}
		for k := range p.Out.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-tollglass-") {
				p.Out.Header.Del(k)
			}
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if tracked {
			status := resp.StatusCode
			r.Response.Status = &status
			if status >= 400 {
				r.Find("upstream_error", "observed", "The upstream returned an HTTP error.")
			}
			stream := strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
			responseStream = stream
			tap = newResponseTap(resp.Body, resp.Header.Get("Content-Encoding"), capture, stream)
			resp.Body = tap
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, q *http.Request, err error) {
		kind := failureKind(err, q.Context())
		if tracked {
			transportFailed = true
			r.Find("transport_error", "observed", kind)
		}
		if g.Logger != nil {
			g.Logger.Printf("upstream failure: %s", kind)
		}
		if q.Context().Err() == nil {
			JSON(w, 502, map[string]any{"error": map[string]any{"type": "transport_error", "upstream": name, "message": kind}})
		}
	}
	proxy.ServeHTTP(w, req)
	returned = true
}

type joinedBody struct {
	io.Reader
	closer io.Closer
}

func (b *joinedBody) Close() error { return b.closer.Close() }
func failureKind(err error, ctx context.Context) string {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return "client cancelled"
	}
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &cert) || errors.As(err, &unknown) {
		return "TLS verification failed"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "name lookup failed"
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return "connection failed"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "upstream read failed"
	}
	return "upstream connection failed"
}
