// Package web serves the loopback-only local page alongside the gateway.
package web

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/x509"
	"encoding/pem"
	"github.com/vincentsch/tabslip/internal/config"
	"github.com/vincentsch/tabslip/internal/proxy"
	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/statement"
	"github.com/vincentsch/tabslip/internal/store"
	"github.com/vincentsch/tabslip/internal/upstream"
)

const SetupPrompt = `Set up Tabslip for this project.
Use https://github.com/vincentsch/tabslip: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.`

//go:embed *.html *.css *.js
var assets embed.FS

var templates = template.Must(template.New("pages").Funcs(template.FuncMap{
	"dateInput": func(s string) string {
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	}, "badStatus": func(p *int) bool { return p == nil || *p >= 400 }, "val": value, "num": number, "json": pretty, "text": func(b []byte) string { return string(b) },
	"warn": func(s store.Summary) bool {
		for _, f := range s.Findings {
			if f != "model_alias_resolved" {
				return true
			}
		}
		return false
	},
}).ParseFS(assets, "*.html"))

func value(p *string) string {
	if p == nil {
		return "unknown"
	}
	return *p
}
func number(v any) string {
	switch p := v.(type) {
	case *int64:
		if p != nil {
			return strconv.FormatInt(*p, 10)
		}
	case *int:
		if p != nil {
			return strconv.Itoa(*p)
		}
	}
	return "unknown"
}
func pretty(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }

type Server struct {
	done                chan struct{}
	stop                sync.Once
	Store               *store.Store
	Reader              *store.Reader
	Port                string
	BaseURL, ConfigPath string
	Upstreams           map[string]upstream.Upstream
	Demo, ContentLocked bool
	mu                  sync.RWMutex
	settings            config.Config
	token               string
}

func New(db *store.Store, c config.Config, path, base string, us map[string]upstream.Upstream, demo, locked bool) *Server {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	u, _ := url.Parse(base)
	return &Server{done: make(chan struct{}), Store: db, Reader: db.Reader(), Port: u.Port(), BaseURL: base, ConfigPath: path, Upstreams: us, Demo: demo, ContentLocked: locked, settings: c, token: hex.EncodeToString(b)}
}
func (s *Server) Close() { s.stop.Do(func() { close(s.done) }) }
func (s *Server) Settings() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.settings
	c.KeepContentTags = append([]string{}, c.KeepContentTags...)
	return c
}
func (s *Server) contentState() string {
	c := s.Settings()
	if c.KeepContent {
		return "Content: stored for all calls"
	}
	if len(c.KeepContentTags) > 0 {
		return "Content: stored for tags " + strings.Join(c.KeepContentTags, ", ")
	}
	return "Content: not stored"
}
func (s *Server) Handler(gateway http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
		if _, ok := s.Upstreams[name]; ok || r.URL.Path == "/healthz" {
			gateway.ServeHTTP(w, r)
			return
		}
		s.ServeHTTP(w, r)
	})
}
func (s *Server) allowed(r *http.Request) bool {
	if !proxy.Loopback(r) {
		return false
	}
	h, p, e := net.SplitHostPort(r.Host)
	return e == nil && p == s.Port && (h == "localhost" || h == "127.0.0.1" || h == "::1")
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.allowed(r) {
		http.Error(w, "The local page needs a loopback connection and a local Host header.", 403)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if r.Header.Get("Origin") != "http://"+r.Host || r.ParseForm() != nil || subtle.ConstantTimeCompare([]byte(r.Form.Get("token")), []byte(s.token)) != 1 {
			http.Error(w, "This change needs the local page's token and origin.", 403)
			return
		}
		switch {
		case r.URL.Path == "/settings":
			s.save(w, r)
		case strings.HasPrefix(r.URL.Path, "/r/") && strings.HasSuffix(r.URL.Path, "/delete"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/r/"), "/delete")
			if s.Store.Delete(id) != nil {
				http.Error(w, "Cannot delete this receipt.", 500)
				return
			}
			http.Redirect(w, r, "/", 303)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(405)
		return
	}
	switch r.URL.Path {
	case "/style.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		b, _ := assets.ReadFile("style.css")
		fmt.Fprint(w, statement.Tokens)
		w.Write(b)
		return
	case "/app.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		b, _ := assets.ReadFile("app.js")
		w.Write(b)
		return
	case "/events":
		s.events(w, r)
		return
	case "/key.pub":
		der, err := x509.MarshalPKIXPublicKey(s.Store.Key.Public().(ed25519.PublicKey))
		b := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		if err != nil {
			http.Error(w, "Cannot read public key.", 500)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", `attachment; filename="tabslip.pub"`)
		w.Write(b)
		return
	}
	s.page(w, r, "")
}

type option struct {
	Name, Label string
	Values      []string
	Selected    string
}
type snippet struct{ Title, Code string }
type findingDetail struct {
	receipt.Finding
	Source   string
	Resolved bool
}

func detailedFindings(r *receipt.Receipt, supplement *receipt.Supplement) []findingDetail {
	// Compare base findings against Combined without supplement findings: identical
	// codes from distinct sources must remain separate entries.
	var baseSupplement *receipt.Supplement
	if supplement != nil {
		copy := *supplement
		copy.Findings = nil
		baseSupplement = &copy
	}
	_, _, active := receipt.Combined(r, baseSupplement)
	var details []findingDetail
	for _, f := range r.Findings {
		resolved := true
		for _, a := range active {
			if a == f {
				resolved = false
				break
			}
		}
		details = append(details, findingDetail{f, "Signed receipt", resolved})
	}
	if supplement != nil {
		source := "Signed OpenRouter details"
		if supplement.Source == "demo.generation" {
			source = "Signed demo details (simulated)"
		}
		for _, f := range supplement.Findings {
			details = append(details, findingDetail{f, source, false})
		}
	}
	return details
}

type pageData struct {
	OpenAIURL                                                                  string
	Findings                                                                   []findingDetail
	Page, Title, Content, Token, Error, Path, Prompt, KeyID, Fingerprint, Next string
	Demo, Locked                                                               bool
	Config                                                                     config.Config
	Filter                                                                     store.Filter
	Options                                                                    []option
	Calls                                                                      []store.Call
	Call                                                                       store.Call
	SVG                                                                        template.HTML
	ReceiptText                                                                string
	Verification                                                               receipt.Verification
	Stored                                                                     *store.Content
	Spend                                                                      spendData
	Snippets                                                                   []snippet
}

func (s *Server) data(r *http.Request) (pageData, error) {
	openaiURL := s.BaseURL + "/openai/v1"
	if s.Demo {
		openaiURL = s.BaseURL + "/demo/v1"
	}
	d := pageData{OpenAIURL: openaiURL, Page: "calls", Title: "Calls", Content: s.contentState(), Token: s.token, Demo: s.Demo, Locked: s.ContentLocked, Path: s.ConfigPath, Prompt: SetupPrompt, KeyID: s.Store.KeyID, Config: s.Settings()}
	q := r.URL.Query()
	d.Filter = store.Filter{Tag: q.Get("tag"), Upstream: q.Get("upstream"), Model: q.Get("model"), Since: q.Get("since"), Until: q.Get("until"), Search: q.Get("q"), Status: q.Get("status"), FindingsOnly: q.Get("findings") == "on", Slowest: q.Get("order") == "slowest", Cursor: q.Get("cursor"), CursorID: q.Get("cursor_id"), Limit: 200}
	var err error
	d.Filter, err = d.Filter.Validate()
	if err != nil {
		return d, err
	}
	for _, o := range []struct{ name, col, label, selected string }{{"tag", "tag", "Tag", d.Filter.Tag}, {"upstream", "upstream", "Upstream", d.Filter.Upstream}, {"model", "requested_model", "Model", d.Filter.Model}} {
		values, e := s.Reader.Options(o.col)
		if e != nil {
			return d, e
		}
		if o.name == "model" {
			more, e := s.Reader.Options("reported_model")
			if e != nil {
				return d, e
			}
			values = append(values, more...)
			sort.Strings(values)
			values = unique(values)
		}
		d.Options = append(d.Options, option{o.name, o.label, values, o.selected})
	}
	switch {
	case r.URL.Path == "/":
		d.Calls, err = s.Reader.Calls(d.Filter)
		if len(d.Calls) == 200 {
			last := d.Calls[len(d.Calls)-1].Summary
			n := r.URL.Query()
			n.Set("cursor_id", last.ID)
			n.Set("cursor", last.Started)
			if d.Filter.Slowest {
				n.Set("cursor", strconv.FormatInt(last.Total, 10))
			}
			d.Next = "/?" + n.Encode()
		}
	case r.URL.Path == "/spend":
		d.Page = "spend"
		d.Title = "Spend"
		d.Spend, err = s.spend(d.Filter, time.Now())
	case r.URL.Path == "/setup":
		d.Page = "setup"
		d.Title = "Setup"
		pub := s.Store.Key.Public().(ed25519.PublicKey)
		h := sha256.Sum256(pub)
		d.Fingerprint = hex.EncodeToString(h[:])
		d.Snippets = s.snippets()
	case strings.HasPrefix(r.URL.Path, "/r/"):
		d.Page = "receipt"
		d.Title = "Receipt"
		id := strings.TrimPrefix(r.URL.Path, "/r/")
		if strings.HasSuffix(id, "/download") {
			id = strings.TrimSuffix(id, "/download")
		}
		d.Call, err = s.Reader.Get(id)
		if err != nil {
			return d, err
		}
		var contributing *receipt.Supplement
		if len(d.Call.Supplements) > 0 {
			contributing = &d.Call.Supplements[len(d.Call.Supplements)-1]
		}
		d.Findings = detailedFindings(&d.Call.Receipt, contributing)
		d.Verification = verify(d.Call, s.Store.Key.Public().(ed25519.PublicKey))
		d.SVG, d.ReceiptText = receiptSVG(d.Call, d.Verification.SignaturesValid)
		d.Stored, err = s.Reader.Content(id)
		if errors.Is(err, sql.ErrNoRows) {
			d.Stored = nil
			err = nil
		}
	default:
		return d, sql.ErrNoRows
	}
	return d, err
}
func verify(c store.Call, pub ed25519.PublicKey) receipt.Verification {
	return receipt.VerifyEnvelopes(receipt.Bundle{Type: "tabslip.bundle/v1", KeyID: receipt.KeyID(pub), Receipts: []json.RawMessage{c.Envelope}, Supplements: c.SupplementEnvelopes}, pub)
}
func unique(a []string) []string {
	out := []string{}
	for _, s := range a {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
func (s *Server) page(w http.ResponseWriter, r *http.Request, message string) {
	d, err := s.data(r)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Cannot load the page. Check the filters and local database.", 400)
		return
	}
	d.Error = message
	if strings.HasSuffix(r.URL.Path, "/download") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+d.Call.Summary.ID+`.json"`)
		w.Write(d.Call.Envelope)
		return
	}
	var b bytes.Buffer
	name := "layout"
	if r.URL.Query().Get("fragment") == "1" {
		name = d.Page
	}
	if err = templates.ExecuteTemplate(&b, name, d); err != nil {
		http.Error(w, "Cannot render the local page.", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	c := s.settings
	if !s.ContentLocked {
		c.KeepContent = r.Form.Get("keep_content") == "on"
	}
	c.KeepContentTags = nil
	for _, tag := range strings.FieldsFunc(r.Form.Get("tags"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if !tagPattern.MatchString(tag) {
			s.mu.Unlock()
			http.Error(w, "Tags need 1 to 64 letters, digits, dots, underscores or hyphens.", 400)
			return
		}
		c.KeepContentTags = append(c.KeepContentTags, tag)
	}

	days, err := strconv.Atoi(r.Form.Get("retention_days"))
	if err != nil || days < 0 || days > 365000 {
		s.mu.Unlock()
		http.Error(w, "Retention needs a whole number of days, or 0 to keep all receipts.", 400)
		return
	}
	c.RetentionDays = days
	err = config.SaveSettings(s.ConfigPath, c, s.ContentLocked)
	var cleanupErr error
	if err == nil {
		cleanupErr = s.Store.SetRetention(days)
		s.settings = c
	}
	s.mu.Unlock()
	r.URL.Path = "/setup"
	if err != nil {
		s.page(w, r, "Settings were not saved. Check the config directory permissions and free space.")
		return
	}
	if cleanupErr != nil {
		s.page(w, r, "Settings saved. Cleanup failed; check the local database before retrying.")
		return
	}
	http.Redirect(w, r, "/setup?saved=1", 303)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming is unavailable.", 500)
		return
	}
	ch, cancel := s.Store.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(kind string, v any) bool {
		b, _ := json.Marshal(v)
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, b)
		f.Flush()
		http.NewResponseController(w).SetWriteDeadline(time.Time{})
		return err == nil
	}
	if !send("counter", s.Store.Unrecorded()) {
		return
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-r.Context().Done():
			return
		case <-tick.C:
			if !send("ping", nil) {
				return
			}
		case change := <-ch:
			if !send("counter", s.Store.Unrecorded()) {
				return
			}
			if change.Kind == "counter" {
				continue
			}
			q := r.URL.Query()
			filter, err := (store.Filter{Tag: q.Get("tag"), Upstream: q.Get("upstream"), Model: q.Get("model"), Since: q.Get("since"), Until: q.Get("until"), Search: q.Get("q"), Status: q.Get("status"), FindingsOnly: q.Get("findings") == "on"}).Validate()
			if err != nil {
				return
			}
			row := ""
			if change.ID != "" {
				calls, e := s.Reader.Matching(change.ID, filter)
				if e != nil {
					return
				}
				if len(calls) > 0 {
					var b bytes.Buffer
					if templates.ExecuteTemplate(&b, "row", calls[0].Summary) != nil {
						return
					}
					row = b.String()
				}
			}
			if !send("change", map[string]any{"kind": change.Kind, "id": change.ID, "row": row, "content": s.contentState()}) {
				return
			}
		}
	}
}
