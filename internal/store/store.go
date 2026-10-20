package store

import (
	"crypto/ed25519"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vincentsch/tollglass/internal/keys"
	"github.com/vincentsch/tollglass/internal/receipt"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Content struct {
	Request   []byte
	Response  []byte
	Truncated bool
}
type Record struct {
	Receipt    *receipt.Receipt
	Envelope   []byte
	Content    *Content
	Supplement *receipt.Supplement
	flushed    chan struct{}
}
type Store struct {
	db         *sql.DB
	Key        ed25519.PrivateKey
	KeyID      string
	queue      chan Record
	done       chan struct{}
	mu         sync.RWMutex
	closed     bool
	unrecorded atomic.Int64
	logger     *log.Logger
	days       int
}

func Open(dir string, days int, logger *log.Logger) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create data directory")
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(dir, "tollglass.db")}
	q := u.Query()
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("cannot open database")
	}
	db.SetMaxOpenConns(1)
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	var exists int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='meta'").Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("cannot read database")
	}
	if exists == 0 {
		b, _ := migrations.ReadFile("migrations/001_initial.sql")
		tx, e := db.Begin()
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(string(b)); e != nil {
			tx.Rollback()
			return nil, fmt.Errorf("cannot migrate database")
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
	}
	var schema string
	if db.QueryRow("SELECT value FROM meta WHERE name='schema_version'").Scan(&schema) != nil || schema != "1" {
		return nil, fmt.Errorf("unsupported database schema")
	}
	var id string
	err = db.QueryRow("SELECT value FROM meta WHERE name='key_id'").Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("cannot read database identity")
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM receipts").Scan(&count); err != nil {
		return nil, err
	}
	key, err := keys.Load(dir, id, count > 0)
	if err != nil {
		return nil, err
	}
	id = receipt.KeyID(key.Public().(ed25519.PublicKey))
	if _, err = db.Exec("INSERT INTO meta(name,value) VALUES('key_id',?) ON CONFLICT(name) DO NOTHING", id); err != nil {
		return nil, err
	}
	var bound string
	if err = db.QueryRow("SELECT value FROM meta WHERE name='key_id'").Scan(&bound); err != nil || bound != id {
		return nil, fmt.Errorf("database key changed during startup; restart with the original key")
	}
	s := &Store{db: db, Key: key, KeyID: id, queue: make(chan Record, 1024), done: make(chan struct{}), logger: logger, days: days}
	if err = s.retain(time.Now()); err != nil {
		return nil, fmt.Errorf("cannot apply receipt retention")
	}
	ok = true
	go s.writeLoop()
	return s, nil
}
func (s *Store) Unrecorded() int64 { return s.unrecorded.Load() }
func (s *Store) failed() {
	s.unrecorded.Add(1)
	if s.logger != nil {
		s.logger.Print("receipt recording failed")
	}
}
func (s *Store) recordFailed(r Record) {
	if r.Supplement != nil {
		if s.logger != nil {
			s.logger.Print("supplement recording failed")
		}
		return
	}
	s.failed()
}
func (s *Store) Enqueue(r Record) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		s.recordFailed(r)
		return false
	}
	select {
	case s.queue <- r:
		return true
	default:
		s.recordFailed(r)
		return false
	}
}
func (s *Store) Save(r *receipt.Receipt, c *Content) {
	b, err := receipt.Sign(r, s.Key)
	if err != nil {
		s.failed()
		return
	}
	s.Enqueue(Record{Receipt: r, Envelope: b, Content: c})
}

// Flush waits until all previously queued writes have been attempted.
func (s *Store) Flush() error {
	done := make(chan struct{})
	if !s.Enqueue(Record{flushed: done}) {
		return fmt.Errorf("cannot flush receipt queue")
	}
	<-done
	if s.Unrecorded() != 0 {
		return fmt.Errorf("some receipts could not be recorded")
	}
	return nil
}
func (s *Store) writeLoop() {
	defer close(s.done)
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case r, ok := <-s.queue:
			if !ok {
				return
			}
			if r.flushed != nil {
				close(r.flushed)
				continue
			}
			if s.insert(r) != nil {
				s.recordFailed(r)
			}
		case <-t.C:
			if s.retain(time.Now()) != nil && s.logger != nil {
				s.logger.Print("receipt retention failed")
			}
		}
	}
}
func (s *Store) insert(v Record) error {
	if v.Supplement != nil {
		return s.insertSupplement(v.Supplement, v.Envelope)
	}
	r := v.Receipt
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tag *string
	if r.Request.Tag != "" {
		tag = &r.Request.Tag
	}
	_, err = tx.Exec(`INSERT INTO receipts (id,started_at,upstream,api,requested_model,reported_model,provider,tag,status,input_tokens,output_tokens,cost_amount,cost_source,ttfb_ms,total_ms,has_findings,envelope) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.Time.Started, r.Request.Upstream, r.Request.API, r.Request.Model, r.Response.Model, r.Response.Provider, tag, r.Response.Status, r.DisplayInput(), r.Usage.Output, r.Cost.Amount, r.Cost.Source, r.Time.TTFB, r.Time.Total, r.HasFindings(), v.Envelope)
	if err != nil {
		return err
	}
	if v.Content != nil {
		c := v.Content
		_, err = tx.Exec("INSERT INTO contents VALUES (?,?,?,?)", r.ID, c.Request, c.Response, c.Truncated)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) retain(now time.Time) error {
	if s.days == 0 {
		return nil
	}
	_, err := s.db.Exec("DELETE FROM receipts WHERE started_at < ?", receipt.Timestamp(now.AddDate(0, 0, -s.days)))
	return err
}
func (s *Store) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	<-s.done
	return s.db.Close()
}
