package store

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tabslip/internal/apis"
	"github.com/vincentsch/tabslip/internal/receipt"
)

func openTest(t *testing.T, dir string, days int) *Store {
	t.Helper()
	s, e := Open(dir, days, log.New(io.Discard, "", 0))
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestRelativeDataDirectorySupportsRecordingAndReading(t *testing.T) {
	// Keep the working directory and temporary data on the same Windows drive.
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Rel(cwd, filepath.Join(t.TempDir(), "local receipts #1"))
	if err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir, 0)
	defer s.Close()
	r := receipt.New(s.KeyID, time.Now())
	s.Save(r, nil)
	if err = s.Flush(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	call, err := reader.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = receipt.Verify(call.Envelope, s.Key.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
}
func sample(s *Store, when time.Time) *receipt.Receipt {
	r := receipt.New(s.KeyID, when)
	r.Request.Upstream = "openai"
	r.Request.API = "openai.chat"
	r.Time.Finished = receipt.Timestamp(when)
	return r
}
func readDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, e := sql.Open("sqlite", filepath.Join(dir, "tabslip.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func TestMigrationBurstDrainAndExactEnvelopes(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir, 0)
	pub := s.Key.Public().(ed25519.PublicKey)
	for i := 0; i < 1000; i++ {
		s.Save(sample(s, time.Now()), nil)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	db := readDB(t, dir)
	var count int
	if e := db.QueryRow("SELECT count(*) FROM receipts").Scan(&count); e != nil || count != 1000 || s.Unrecorded() != 0 {
		t.Fatalf("burst count=%d lost=%d: %v", count, s.Unrecorded(), e)
	}
	rows, e := db.Query("SELECT envelope FROM receipts")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		rows.Scan(&b)
		p, e := receipt.Verify(b, pub)
		if e != nil {
			t.Fatal(e)
		}
		var env receipt.Envelope
		json.Unmarshal(b, &env)
		if base64.StdEncoding.EncodeToString(p) != env.Payload {
			t.Fatal("signed bytes changed")
		}
	}
	for _, table := range []string{"meta", "supplements", "contents"} {
		if e := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); e != nil {
			t.Fatal(e)
		}
	}
	info, e := os.Stat(filepath.Join(dir, "key.ed25519"))
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("private key permissions")
	}
}
func TestRetentionAndIdentityRefusal(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir, 0)
	r := sample(s, time.Now().AddDate(0, 0, -100))
	s.Save(r, &Content{Request: []byte("old"), Response: []byte("old")})
	fresh := sample(s, time.Now())
	s.Save(fresh, nil)
	s.Close()
	db := readDB(t, dir)
	db.Exec("INSERT INTO supplements VALUES (?,?)", r.ID, []byte("old"))
	s = openTest(t, dir, 90)
	if e := s.retain(time.Now()); e != nil {
		t.Fatal(e)
	}
	s.Close()
	for _, tc := range []struct {
		table string
		want  int
	}{{"receipts", 1}, {"contents", 0}, {"supplements", 0}} {
		var n int
		if e := db.QueryRow("SELECT count(*) FROM " + tc.table).Scan(&n); e != nil || n != tc.want {
			t.Fatalf("retention %s %d: %v", tc.table, n, e)
		}
	}
	key, e := os.ReadFile(filepath.Join(dir, "key.ed25519"))
	if e != nil {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(dir, "key.ed25519"))
	if _, e = Open(dir, 0, nil); e == nil || !strings.Contains(e.Error(), "missing") {
		t.Fatal("missing key accepted")
	}
	if _, e = os.Stat(filepath.Join(dir, "key.ed25519")); !os.IsNotExist(e) {
		t.Fatal("missing key regenerated")
	}
	os.WriteFile(filepath.Join(dir, "key.ed25519"), key, 0600)
	other := t.TempDir()
	s = openTest(t, other, 0)
	s.Close()
	otherKey, _ := os.ReadFile(filepath.Join(other, "key.ed25519"))
	os.WriteFile(filepath.Join(dir, "key.ed25519"), otherKey, 0600)
	if _, e = Open(dir, 0, nil); e == nil || !strings.Contains(e.Error(), "does not match") {
		t.Fatal("mismatched key accepted")
	}
}
func TestStalledWriterAndWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	s, err := Open(t.TempDir(), 0, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// Sign distinct records before checking enqueue; signing throughput is not
	// part of the queue's non-blocking contract.
	work := make([]Record, cap(s.queue)+1)
	for i := range work {
		r := sample(s, time.Now())
		raw, err := receipt.Sign(r, s.Key)
		if err != nil {
			t.Fatal(err)
		}
		work[i] = Record{Receipt: r, Envelope: raw}
	}
	conn, err := s.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Release the connection before closing the store, even after a fatal check.
	t.Cleanup(func() { conn.Close() })
	waits := s.db.Stats().WaitCount
	if !s.Enqueue(work[0]) {
		t.Fatal("initial enqueue failed")
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.db.Stats().WaitCount == waits {
		if time.Now().After(deadline) {
			t.Fatal("writer did not wait for held connection")
		}
		time.Sleep(time.Millisecond)
	}
	for _, record := range work[1:] {
		if !s.Enqueue(record) {
			t.Fatal("queue filled before capacity")
		}
	}
	done := make(chan bool, 1)
	go func() { done <- s.Enqueue(work[0]) }()
	select {
	case accepted := <-done:
		if accepted || s.Unrecorded() != 1 {
			t.Fatal("full queue did not reject and count exactly one call")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("enqueue waited for stalled writer")
	}
	if !strings.Contains(logs.String(), "receipt recording failed") {
		t.Fatal("missing safe diagnostic")
	}
}

func TestFailedWriteCountsUnrecordedCall(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	t.Cleanup(func() { s.Close() })
	if _, err := s.db.Exec("DROP TABLE receipts"); err != nil {
		t.Fatal(err)
	}
	s.Save(sample(s, time.Now()), nil)
	s.Close()
	if s.Unrecorded() != 1 {
		t.Fatal("write failure not counted")
	}
}
func TestContentOnlyInDesignatedTable(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[keep], func(t *testing.T) {
			dir := t.TempDir()
			s := openTest(t, dir, 0)
			r := sample(s, time.Now())
			kind := "invalid_request_error"
			r.Response.Error = &receipt.ProviderError{Type: &kind}
			var content *Content
			if keep {
				content = &Content{Request: []byte("private prompt"), Response: []byte("private error body")}
			}
			s.Save(r, content)
			s.Close()
			db := readDB(t, dir)
			var b []byte
			if e := db.QueryRow("SELECT envelope FROM receipts").Scan(&b); e != nil {
				t.Fatal(e)
			}
			if bytes.Contains(b, []byte("private")) {
				t.Fatal("content in envelope")
			}
			var n int
			db.QueryRow("SELECT count(*) FROM contents").Scan(&n)
			if (n == 1) != keep {
				t.Fatal("content policy")
			}
			if !keep {
				raw, _ := os.ReadFile(filepath.Join(dir, "tabslip.db"))
				if bytes.Contains(raw, []byte("private prompt")) {
					t.Fatal("content in database")
				}
			}
		})
	}
}

func TestSummaryTagsPreserveMissingValues(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir, 0)
	untagged := sample(s, time.Now())
	tagged := sample(s, time.Now())
	tagged.Request.Tag = "acme"
	s.Save(untagged, nil)
	s.Save(tagged, nil)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db := readDB(t, dir)
	for _, tc := range []struct {
		id   string
		want sql.NullString
	}{{untagged.ID, sql.NullString{}}, {tagged.ID, sql.NullString{String: "acme", Valid: true}}} {
		var tag sql.NullString
		if err := db.QueryRow("SELECT tag FROM receipts WHERE id=?", tc.id).Scan(&tag); err != nil {
			t.Fatal(err)
		}
		if tag != tc.want {
			t.Fatalf("summary tag=%+v, want %+v", tag, tc.want)
		}
	}
}

func TestAnthropicSummaryUsesTotalWhileEnvelopeKeepsUncachedInput(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir, 0)
	pub := s.Key.Public().(ed25519.PublicKey)
	var ids []string
	for _, mode := range []string{"complete", "missing", "null"} {
		r := sample(s, time.Now())
		r.Request.API = "anthropic.messages"
		raw := `{"source":"reported","input_tokens":100,"output_tokens":12,"cached_input_tokens":50,"cache_write_tokens":25,"cache_write_1h_tokens":10}`
		if mode == "missing" {
			raw = strings.Replace(raw, `,"cached_input_tokens":50`, "", 1)
		}
		if mode == "null" {
			raw = strings.Replace(raw, `"cache_write_tokens":25`, `"cache_write_tokens":null`, 1)
		}
		if err := json.Unmarshal([]byte(raw), &r.Usage); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
		s.Save(r, nil)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db := readDB(t, dir)
	for i, id := range ids {
		var input sql.NullInt64
		var env []byte
		if err := db.QueryRow("SELECT input_tokens,envelope FROM receipts WHERE id=?", id).Scan(&input, &env); err != nil {
			t.Fatal(err)
		}
		if i == 0 && (!input.Valid || input.Int64 != 175) {
			t.Fatal("display total", input)
		}
		if i > 0 && input.Valid {
			t.Fatal("unknown cache usage displayed as zero", input)
		}
		payload, err := receipt.Verify(env, pub)
		if err != nil {
			t.Fatal(err)
		}
		var r receipt.Receipt
		if err = json.Unmarshal(payload, &r); err != nil || r.Usage.Input == nil || *r.Usage.Input != 100 {
			t.Fatal("signed uncached count changed")
		}
	}
}

func TestTerminalProviderErrorsDoNotStoreSensitiveMessages(t *testing.T) {
	dir := t.TempDir()
	var logs bytes.Buffer
	s, err := Open(dir, 0, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	pub := s.Key.Public().(ed25519.PublicKey)
	for _, tc := range []struct{ api, file string }{
		{"anthropic.messages", "messages-error.sse"}, {"openai.responses", "responses-failed.sse"}, {"openai.responses", "responses-error.sse"}, {"openai.responses", "responses-nested-error.sse"},
	} {
		raw, err := os.ReadFile("../../testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		r := sample(s, time.Now())
		r.Request.API = tc.api
		c := &apis.Capture{Receipt: r}
		c.Parse(bytes.NewReader(raw), true)
		r.Response.Complete = c.Done
		if !c.Done || r.Response.Error == nil || (r.Usage.Input == nil && tc.file != "responses-error.sse" && tc.file != "responses-nested-error.sse") {
			t.Fatal("terminal error metadata lost")
		}
		s.Save(r, nil)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db := readDB(t, dir)
	var n int
	if err := db.QueryRow("SELECT count(*) FROM contents").Scan(&n); err != nil || n != 0 {
		t.Fatal("stored content when disabled")
	}
	rows, err := db.Query("SELECT envelope FROM receipts")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var env []byte
		if err := rows.Scan(&env); err != nil {
			t.Fatal(err)
		}
		payload, err := receipt.Verify(env, pub)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(payload, []byte("sensitive")) || bytes.Contains(payload, []byte("synthetic-secret-marker")) {
			t.Fatal("message in signed receipt")
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tabslip.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("sensitive")) || bytes.Contains(raw, []byte("synthetic-secret-marker")) || logs.Len() != 0 {
		t.Fatal("sensitive error stored or logged")
	}
}

func TestNotificationsFollowCommitAndQueueFailuresBypassStalledWriter(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	ch, stop := s.Subscribe()
	defer stop()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	r := sample(s, time.Now())
	raw, err := receipt.Sign(r, s.Key)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1100; i++ {
		s.Enqueue(Record{Receipt: r, Envelope: raw})
	}
	if s.Unrecorded() == 0 {
		tx.Rollback()
		t.Fatal("queue did not fill")
	}
	select {
	case c := <-ch:
		if c.Kind != "counter" && c.Kind != "refresh" {
			t.Fatal("published a row before commit", c)
		}
	case <-time.After(time.Second):
		tx.Rollback()
		t.Fatal("stalled writer prevented failure notification")
	}
	tx.Rollback()
	// Drain failed duplicate writes before closing. No successful write is needed
	// for the counter notification above.
}
func TestRetentionSettingsDeleteContentsAndSupplementsAndZeroKeepsAll(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	r := sample(s, time.Now().AddDate(0, 0, -40))
	s.Save(r, &Content{Request: []byte("old"), Response: []byte("old")})
	s.Flush()
	p, _ := json.Marshal(r)
	supp := &receipt.Supplement{Type: "tabslip.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(p)}
	s.SaveSupplement(supp)
	s.Flush()
	if err := s.SetRetention(0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reader().Get(r.ID); err != nil {
		t.Fatal("zero retention deleted a call")
	}
	if err := s.SetRetention(30); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"receipts", "supplements", "contents"} {
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("retention left rows", table, n, err)
		}
	}
}

func TestSlowSubscribersDoNotBlockNotifications(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	events, stop := s.Subscribe()
	defer stop()
	quit := make(chan struct{})
	defer close(quit)
	go func() {
		for {
			select {
			case <-quit:
				return
			case <-events:
				time.Sleep(time.Microsecond)
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10000; i++ {
			s.publish(Change{Kind: "receipt", ID: "call"})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("slow browser held up store notifications")
	}
}
