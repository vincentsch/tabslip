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
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/receipt"
)

func openTest(t *testing.T, dir string, days int) *Store {
	t.Helper()
	s, e := Open(dir, days, log.New(io.Discard, "", 0))
	if e != nil {
		t.Fatal(e)
	}
	return s
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
	db, e := sql.Open("sqlite", filepath.Join(dir, "tollglass.db"))
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
	if e != nil || info.Mode().Perm() != 0600 {
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
	dir := t.TempDir()
	var logs bytes.Buffer
	s, e := Open(dir, 0, log.New(&logs, "", 0))
	if e != nil {
		t.Fatal(e)
	}
	conn, e := s.db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	for i := 0; i < 1100; i++ {
		s.Save(sample(s, time.Now()), nil)
	}
	if time.Since(start) > 2*time.Second || s.Unrecorded() == 0 {
		t.Fatal("stalled writer blocked enqueue or did not count loss")
	}
	conn.Close()
	s.Close()
	if !strings.Contains(logs.String(), "receipt recording failed") {
		t.Fatal("missing safe diagnostic")
	}
	s = openTest(t, t.TempDir(), 0)
	s.db.Exec("DROP TABLE receipts")
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
				raw, _ := os.ReadFile(filepath.Join(dir, "tollglass.db"))
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
