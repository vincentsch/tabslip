package web

import (
	"bufio"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
)

func fixture(t *testing.T) (*Server, *receipt.Receipt) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := config.Default()
	c.RetentionDays = 0
	us, _ := upstream.Build(nil)
	s := New(db, c, filepath.Join(dir, "config.yaml"), "http://127.0.0.1:4545", us, false, false)
	t.Cleanup(s.Close)
	r := receipt.New(db.KeyID, time.Now())
	asked, reported, provider := "asked/model", "reported/<script>alert(1)</script>", "Known provider"
	status := 200
	r.Request.Upstream = "openrouter"
	r.Request.API = "openai.chat"
	r.Request.Model = &asked
	r.Response.Model = &reported
	r.Response.Provider = &provider
	r.Response.Status = &status
	r.Request.Tag = "client-a"
	r.Time.Total = 430
	r.Find("model_differs", "reported", "The provider reports a different model.")
	r.Find("price_unknown", "observed", "The cost is unknown.")
	db.Save(r, &store.Content{Request: []byte("content-only-needle"), Response: []byte("private-answer")})
	if err = db.Flush(); err != nil {
		t.Fatal(err)
	}
	return s, r
}
func request(s *Server, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://127.0.0.1:4545"+path, nil)
	r.RemoteAddr = "127.0.0.1:3333"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func post(s *Server, path string, form url.Values, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1:4545"+path, strings.NewReader(form.Encode()))
	r.RemoteAddr = "127.0.0.1:3333"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestPagesPrivacyEscapingAndDownloads(t *testing.T) {
	s, r := fixture(t)
	d, e := s.data(httptest.NewRequest("GET", "http://127.0.0.1:4545/", nil))
	if e != nil {
		t.Fatal(e)
	}
	if e = templates.ExecuteTemplate(io.Discard, "layout", d); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/", "/spend", "/setup", "/r/" + r.ID} {
		w := request(s, path)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		body := w.Body.String()
		if strings.Contains(body, "<script>alert(1)</script>") {
			t.Fatalf("unescaped model: %s", path)
		}
		if path != "/r/"+r.ID && (strings.Contains(body, "content-only-needle") || strings.Contains(body, "private-answer")) {
			t.Fatal("content leaked", path)
		}
	}
	view := request(s, "/r/"+r.ID).Body.String()
	for _, want := range []string{"SIGNED", "content-only-needle", "private-answer", "Model names", "≠", "&lt;script&gt;"} {
		if !strings.Contains(view, want) {
			t.Error("receipt missing", want)
		}
	}
	download := request(s, "/r/"+r.ID+"/download")
	c, err := s.Reader.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if download.Body.String() != string(c.Envelope) {
		t.Fatal("download changed signed bytes")
	}
	pub := request(s, "/key.pub")
	if !strings.Contains(pub.Body.String(), "BEGIN PUBLIC KEY") {
		t.Fatal("missing key")
	}
	for _, q := range []string{"asked/model", *r.Response.Model} {
		v := request(s, "/?model="+url.QueryEscape(q))
		if !strings.Contains(v.Body.String(), `data-id="`+r.ID+`"`) {
			t.Error("exact model filter failed", q)
		}
	}
	for _, q := range []string{"PROVIDER", r.ID, "client-a", "openrouter"} {
		if !strings.Contains(request(s, "/?q="+url.QueryEscape(q)).Body.String(), `data-id="`+r.ID+`"`) {
			t.Error("search missed", q)
		}
	}
	for _, q := range []string{"content-only-needle", "private-answer"} {
		if strings.Contains(request(s, "/?q="+q).Body.String(), `data-id="`+r.ID+`"`) {
			t.Error("searched content")
		}
	}
	if strings.Contains(request(s, "/?model=ASKED/model").Body.String(), `data-id="`+r.ID+`"`) {
		t.Fatal("model matching lost case")
	}
}
func TestAccessBoundaryCoversLocalRoutesAndAllowsProxy(t *testing.T) {
	s, r := fixture(t)
	routes := []string{"/", "/spend", "/setup", "/events", "/key.pub", "/settings", "/style.css", "/app.js", "/r/" + r.ID, "/r/" + r.ID + "/download", "/r/" + r.ID + "/delete"}
	for _, path := range routes {
		for _, foreign := range []bool{false, true} {
			req := httptest.NewRequest("GET", "http://127.0.0.1:4545"+path, nil)
			req.RemoteAddr = "127.0.0.1:12"
			if foreign {
				req.RemoteAddr = "192.0.2.1:12"
			} else {
				req.Host = "evil.example:4545"
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatal(path, foreign, w.Code)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("CORS enabled")
			}
		}
	}
	for _, origin := range []string{"", "http://evil.example:4545"} {
		w := post(s, "/settings", url.Values{"token": {s.token}, "retention_days": {"0"}}, origin)
		if w.Code != 403 {
			t.Fatal("foreign origin accepted")
		}
	}
	if post(s, "/settings", url.Values{"retention_days": {"0"}}, s.BaseURL).Code != 403 {
		t.Fatal("missing token accepted")
	}
	req := httptest.NewRequest("POST", "http://evil.example:4545/openai/v1/chat/completions", nil)
	req.RemoteAddr = "192.0.2.1:12"
	w := httptest.NewRecorder()
	s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) })).ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatal("page restriction blocked proxy")
	}
}
func TestSaveSettingsPreservesUpstreamsAndFailureLeavesState(t *testing.T) {
	s, _ := fixture(t)
	original := "# selected config\nupstreams:\n  - name: local\n    base_url: http://localhost:8001\n    api: openai\n"
	if err := os.WriteFile(s.ConfigPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"token": {s.token}, "keep_content": {"on"}, "tags": {"client-a, client-b"}, "retention_days": {"0"}}
	w := post(s, "/settings", form, s.BaseURL)
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body)
	}
	b, _ := os.ReadFile(s.ConfigPath)
	loaded, err := config.Load(s.ConfigPath)
	if err != nil || len(loaded.Upstreams) != 1 || !loaded.KeepContent {
		t.Fatal("config lost", string(b), err)
	}
	if !strings.Contains(request(s, "/").Body.String(), "Content: stored for all calls") {
		t.Fatal("header stale")
	}
	delete(form, "keep_content")
	if post(s, "/settings", form, s.BaseURL).Code != 303 {
		t.Fatal("tag settings failed")
	}
	if !strings.Contains(request(s, "/").Body.String(), "Content: stored for tags client-a, client-b") {
		t.Fatal("per-tag header stale")
	}
	old := s.Settings()
	s.ConfigPath = filepath.Join(s.ConfigPath, "cannot-save")
	form.Set("keep_content", "on")
	w = post(s, "/settings", form, s.BaseURL)
	if !strings.Contains(w.Body.String(), "Settings were not saved") || s.Settings().KeepContent != old.KeepContent {
		t.Fatal("failed save changed settings", w.Body)
	}
	s.ConfigPath = filepath.Dir(s.ConfigPath)
	s.ContentLocked = true
	w = request(s, "/setup")
	if !strings.Contains(w.Body.String(), "disabled") || !strings.Contains(w.Body.String(), "--keep-content") {
		t.Fatal("flag override not explained")
	}
	if post(s, "/settings", form, s.BaseURL).Code != 303 || s.Settings().KeepContent != old.KeepContent {
		t.Fatal("flag overridden by form")
	}
	loaded, err = config.Load(s.ConfigPath)
	if err != nil || loaded.KeepContent != old.KeepContent {
		t.Fatal("locked save changed file preference", loaded.KeepContent, err)
	}
}
func TestLockedContentSettingPreservesFilePreference(t *testing.T) {
	for _, tc := range []struct {
		name, original string
		active, stored bool
	}{
		{"flag enables storage", "keep_content: false # saved preference\nretention_days: 90\n", true, false},
		{"flag disables storage", "keep_content: true # saved preference\nretention_days: 90\n", false, true},
		{"setting omitted", "retention_days: 90\n", true, false},
		{"first save", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := fixture(t)
			if tc.original != "" {
				if err := os.WriteFile(s.ConfigPath, []byte(tc.original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s.ContentLocked = true
			s.settings.KeepContent = tc.active
			form := url.Values{"token": {s.token}, "keep_content": {"on"}, "tags": {"client-b"}, "retention_days": {"14"}}
			if w := post(s, "/settings", form, s.BaseURL); w.Code != 303 {
				t.Fatalf("save: %d %s", w.Code, w.Body)
			}
			saved, err := config.Load(s.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if saved.KeepContent != tc.stored {
				t.Fatalf("saved content preference = %t, want %t", saved.KeepContent, tc.stored)
			}
			if s.Settings().KeepContent != tc.active {
				t.Fatal("save changed startup override")
			}
			if saved.RetentionDays != 14 || len(saved.KeepContentTags) != 1 || saved.KeepContentTags[0] != "client-b" {
				t.Fatal("editable settings were not saved", saved)
			}
			b, err := os.ReadFile(s.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(tc.original, "keep_content:") && strings.Contains(string(b), "keep_content:") {
				t.Fatal("save added the locked setting")
			}
			if strings.Contains(tc.original, "# saved preference") && !strings.Contains(string(b), "# saved preference") {
				t.Fatal("save replaced the locked YAML node")
			}
		})
	}
}

func supplement(r *receipt.Receipt, cost *string, provider *string) *receipt.Supplement {
	p, _ := json.Marshal(r)
	return &receipt.Supplement{Type: "tollglass.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(p), Currency: "USD", Cost: cost, Provider: provider, Attempts: []receipt.Attempt{}, Findings: []receipt.Finding{}}
}
func TestVerificationIncludesContributors(t *testing.T) {
	s, r := fixture(t)
	c, _ := s.Reader.Get(r.ID)
	pub := s.Store.Key.Public().(ed25519.PublicKey)
	if !verify(c, pub).SignaturesValid {
		t.Fatal("base only invalid")
	}
	damage := func(b []byte) []byte {
		var e receipt.Envelope
		json.Unmarshal(b, &e)
		raw, _ := base64.StdEncoding.DecodeString(e.Signatures[0].Sig)
		raw[0] ^= 1
		e.Signatures[0].Sig = base64.StdEncoding.EncodeToString(raw)
		out, _ := json.Marshal(e)
		return out
	}
	c.Envelope = damage(c.Envelope)
	if verify(c, pub).SignaturesValid {
		t.Fatal("damaged base accepted")
	}
	c, _ = s.Reader.Get(r.ID)
	cost := "0.02"
	supp := supplement(r, &cost, nil)
	raw, _ := receipt.SignSupplement(supp, s.Store.Key)
	c.SupplementEnvelopes = []json.RawMessage{raw}
	if !verify(c, pub).SignaturesValid {
		t.Fatal("valid supplement invalid")
	}
	for _, kind := range []string{"signature", "digest", "signer"} {
		bad := *supp
		switch kind {
		case "digest":
			bad.ReceiptSHA256 = "wrong"
		case "signer":
			bad.ReceiptID = "wrong"
		}
		raw, _ = receipt.SignSupplement(&bad, s.Store.Key)
		if kind == "signature" {
			raw = damage(raw)
		}
		if kind == "signer" {
			_, key, _ := ed25519.GenerateKey(nil)
			raw, _ = receipt.SignSupplement(supp, key)
		}
		c.SupplementEnvelopes = []json.RawMessage{raw}
		v := verify(c, pub)
		svg, _ := receiptSVG(c, v.SignaturesValid)
		if v.SignaturesValid || !strings.Contains(string(svg), "INVALID") {
			t.Fatal("invalid contributor accepted", kind)
		}
	}
}
func TestSpendUTCIntersectionsAndRecoveredCost(t *testing.T) {
	s, r := fixture(t)
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// The receipt from fixture is unknown. Two known calls sit around UTC midnight.
	for _, when := range []time.Time{today.Add(-time.Millisecond), today.Add(time.Millisecond)} {
		v := receipt.New(s.Store.KeyID, when)
		v.Request.Upstream = "openai"
		v.Request.Tag = "client-a"
		cost := "0.1"
		v.Cost = receipt.Cost{Source: "estimated", Amount: &cost}
		s.Store.Save(v, nil)
	}
	s.Store.Flush()
	d, err := s.spend(store.Filter{Tag: "client-a"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if d.Tiles[0].Rows[0].Total.Amount() != "0.1" || d.Tiles[0].Rows[0].Total.Unknown != 1 || d.Tiles[1].Rows[0].Total.Amount() != "0.2" {
		t.Fatal("UTC windows wrong", pretty(d))
	}
	f, _ := (store.Filter{Since: receipt.Timestamp(today), Until: receipt.Timestamp(today.Add(2 * time.Millisecond))}).Validate()
	d, err = s.spend(f, now)
	if err != nil || d.Tiles[1].Rows[0].Total.Amount() != "0.1" || d.Tiles[1].Rows[0].Total.Calls != 1 {
		t.Fatal("bounds did not intersect tiles", err, pretty(d))
	}
	cost := "0.03"
	s.Store.SaveSupplement(supplement(r, &cost, nil))
	s.Store.Flush()
	d, err = s.spend(store.Filter{}, now)
	if err != nil || d.Tiles[0].Rows[0].Total.Amount() != "0.13" || d.Tiles[0].Rows[0].Total.Unknown != 0 {
		t.Fatal("recovered cost wrong", err, pretty(d))
	}
	calls, _ := s.Reader.Calls(store.Filter{Search: "Known provider"})
	if len(calls) != 1 || *calls[0].Summary.Provider != "Known provider" {
		t.Fatal("null provider erased original")
	}
}
func TestSSECommittedRowsDelayedSupplementAndIndependentCounter(t *testing.T) {
	s, r := fixture(t)
	server := httptest.NewServer(s)
	defer server.Close()
	defer s.Close()
	u, _ := url.Parse(server.URL)
	s.Port = u.Port()
	s.BaseURL = server.URL
	resp, err := server.Client().Get(server.URL + "/events?findings=on")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 30)
	go func() {
		defer close(lines)
		scan := bufio.NewScanner(resp.Body)
		for scan.Scan() {
			lines <- scan.Text()
		}
	}()
	await := func(needle string) string {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case line := <-lines:
				if strings.Contains(line, needle) {
					return line
				}
			case <-deadline:
				t.Fatal("missing event", needle)
				return ""
			}
		}
	}
	await("event: counter")
	provider := "Delayed <script>provider</script>"
	cost := "0.02"
	s.Store.SaveSupplement(supplement(r, &cost, &provider))
	s.Store.Flush()
	line := await(`"row":`)
	var v map[string]any
	if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v) != nil {
		t.Fatal(line)
	}
	row := v["row"].(string)
	if strings.Contains(row, "<script>") || !strings.Contains(row, "Delayed &lt;script&gt;") {
		t.Fatal("SSE row unsafe or stale", row)
	}
	// A failing insert raises the counter with no subsequent successful commit.
	s.Store.Save(r, nil)
	await("event: counter")
	await("data: 1")
	// Price recovery removes a call with no other finding from the active filter.
	vcall := receipt.New(s.Store.KeyID, time.Now())
	vcall.Request.Upstream = "openrouter"
	vcall.Find("price_unknown", "observed", "Unknown cost.")
	s.Store.Save(vcall, nil)
	s.Store.Flush()
	await(vcall.ID)
	s.Store.SaveSupplement(supplement(vcall, &cost, nil))
	line = await(`"row":""`)
	if !strings.Contains(line, vcall.ID) {
		t.Fatal("wrong removed call", line)
	}
	s.Close()
	io.Copy(io.Discard, resp.Body)
}
func TestDeleteCascadesAndPromptMatchesCopySource(t *testing.T) {
	s, r := fixture(t)
	cost := "1"
	s.Store.SaveSupplement(supplement(r, &cost, nil))
	s.Store.Flush()
	for _, path := range []string{"../../README.md", "../../AGENTS.md"} {
		public, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(public), SetupPrompt) {
			t.Fatalf("%s: public setup prompt differs from displayed and copied source: %v", path, err)
		}
	}
	if !strings.Contains(html.UnescapeString(request(s, "/setup").Body.String()), SetupPrompt) {
		t.Fatal("setup prompt differs")
	}
	w := post(s, "/r/"+r.ID+"/delete", url.Values{"token": {s.token}}, s.BaseURL)
	if w.Code != 303 || request(s, "/r/"+r.ID).Code != 404 {
		t.Fatal("delete failed")
	}
	if _, err := s.Reader.Content(r.ID); err == nil {
		t.Fatal("content survived delete")
	}
}

func TestReceiptPageShowsInvalidForDamagedStoredContributors(t *testing.T) {
	s, r := fixture(t)
	db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(s.ConfigPath), "tollglass.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c, _ := s.Reader.Get(r.ID)
	var e receipt.Envelope
	json.Unmarshal(c.Envelope, &e)
	sig, _ := base64.StdEncoding.DecodeString(e.Signatures[0].Sig)
	sig[0] ^= 1
	e.Signatures[0].Sig = base64.StdEncoding.EncodeToString(sig)
	damaged, _ := json.Marshal(e)
	if _, err = db.Exec("UPDATE receipts SET envelope=? WHERE id=?", damaged, r.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request(s, "/r/"+r.ID).Body.String(), ">INVALID<") {
		t.Fatal("damaged base page did not show INVALID")
	}
	db.Exec("UPDATE receipts SET envelope=? WHERE id=?", []byte(c.Envelope), r.ID)
	cost := "0.3"
	supp := supplement(r, &cost, nil)
	s.Store.SaveSupplement(supp)
	s.Store.Flush()
	c, _ = s.Reader.Get(r.ID)
	json.Unmarshal(c.SupplementEnvelopes[0], &e)
	sig, _ = base64.StdEncoding.DecodeString(e.Signatures[0].Sig)
	sig[0] ^= 1
	e.Signatures[0].Sig = base64.StdEncoding.EncodeToString(sig)
	damaged, _ = json.Marshal(e)
	if _, err = db.Exec("UPDATE supplements SET envelope=? WHERE receipt_id=?", damaged, r.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request(s, "/r/"+r.ID).Body.String(), ">INVALID<") {
		t.Fatal("damaged supplement page did not show INVALID")
	}
}
func TestReceiptWrapsLongValuesWithoutLosingText(t *testing.T) {
	s, r := fixture(t)
	long := strings.Repeat("very-long/provider.model-name/", 12) + "<script>text</script>"
	r.Request.Model = &long
	r.Response.Model = &long
	r.Response.Provider = &long
	r.Find("fallback_used", "reported", long)
	c, _ := s.Reader.Get(r.ID)
	c.Receipt = *r
	c.Summary.RequestedModel = &long
	c.Summary.ReportedModel = &long
	c.Summary.Provider = &long
	svg, text := receiptSVG(c, true)
	if strings.Contains(string(svg), "<script>") {
		t.Fatal("SVG interpreted provider text")
	}
	parts := wrap(long, 38)
	if strings.Join(parts, "") != long {
		t.Fatal("wrapping lost text")
	}
	for _, p := range parts {
		if len([]rune(p)) > 38 {
			t.Fatal("line exceeds width")
		}
	}
	if !strings.Contains(text, strings.Join(parts, "\n")) && !strings.Contains(text, parts[0]) {
		t.Fatal("missing accessible receipt text")
	}
}
func TestDemoExamplesUseTheSimulatedRouteAndSyntheticSDKKey(t *testing.T) {
	s, _ := fixture(t)
	s.Demo = true
	for _, snip := range s.snippets() {
		if strings.Contains(snip.Title, "Python") || strings.Contains(snip.Title, "Node") || strings.Contains(snip.Title, "TypeScript") {
			if !strings.Contains(snip.Code, "/demo") || !strings.Contains(snip.Code, "demo") {
				t.Fatal("demo snippet uses a real provider", snip.Title, snip.Code)
			}
			if strings.Contains(snip.Code, "/openai/") || strings.Contains(snip.Code, "/anthropic") {
				t.Fatal("wrong demo route")
			}
		}
	}
	if !strings.Contains(request(s, "/?q=absent").Body.String(), s.BaseURL+"/demo/v1") {
		t.Fatal("empty demo points at unavailable preset")
	}
}

func TestSSEFailureCounterArrivesWhileWriterRemainsStalled(t *testing.T) {
	s, r := fixture(t)
	server := httptest.NewServer(s)
	defer server.Close()
	defer s.Close()
	u, _ := url.Parse(server.URL)
	s.Port = u.Port()
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(server.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scan := bufio.NewScanner(response.Body)
	for scan.Scan() {
		if scan.Text() == "data: 0" {
			break
		}
	}
	locked, err := sql.Open("sqlite", filepath.Join(filepath.Dir(s.ConfigPath), "tollglass.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	if _, err = locked.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer locked.Exec("ROLLBACK")
	for i := 0; i < 1100; i++ {
		s.Store.Save(r, nil)
	}
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "data: ") {
			var n int64
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &n) == nil && n > 0 {
				return
			}
		}
	}
	t.Fatal("no positive counter arrived while the writer was stalled", scan.Err())
}
