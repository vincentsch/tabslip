package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vincentsch/tollglass/internal/receipt"
)

type Reader struct{ db *sql.DB }

// OpenReader never initializes local state or applies retention.
func OpenReader(dir string) (*Reader, error) {
	path := filepath.Join(dir, "tollglass.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var schema string
	if err = db.QueryRow("SELECT value FROM meta WHERE name='schema_version'").Scan(&schema); err != nil || schema != "1" {
		db.Close()
		return nil, fmt.Errorf("cannot read database schema; use a compatible Tollglass version")
	}
	return &Reader{db}, nil
}
func (r *Reader) Close() error { return r.db.Close() }
func (r *Reader) KeyID() (string, error) {
	var id string
	err := r.db.QueryRow("SELECT value FROM meta WHERE name='key_id'").Scan(&id)
	return id, err
}

type Filter struct {
	Tag, Upstream, Model, Since, Until string
	Limit                              int
}

func Date(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse("2006-01-02", s)
	}
	if err != nil {
		return "", fmt.Errorf("invalid date %q; use RFC 3339 or YYYY-MM-DD", s)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}
func (f Filter) Validate() (Filter, error) {
	var err error
	f.Since, err = Date(f.Since)
	if err != nil {
		return f, err
	}
	f.Until, err = Date(f.Until)
	if err != nil {
		return f, err
	}
	if f.Since != "" && f.Until != "" {
		a, _ := time.Parse(time.RFC3339Nano, f.Since)
		b, _ := time.Parse(time.RFC3339Nano, f.Until)
		if !a.Before(b) {
			return f, fmt.Errorf("--since must precede --until")
		}
	}
	if f.Limit < 0 {
		return f, fmt.Errorf("--limit cannot be negative")
	}
	return f, nil
}

type Summary struct {
	ID             string   `json:"id"`
	Started        string   `json:"started"`
	Tag            *string  `json:"tag"`
	Upstream       string   `json:"upstream"`
	API            string   `json:"api"`
	RequestedModel *string  `json:"requested_model"`
	ReportedModel  *string  `json:"reported_model"`
	Provider       *string  `json:"provider"`
	Status         *int     `json:"status"`
	Input          *int64   `json:"input_tokens"`
	Output         *int64   `json:"output_tokens"`
	Cost           *string  `json:"cost"`
	CostSource     string   `json:"cost_source"`
	TTFB           *int64   `json:"ttfb_ms"`
	Total          int64    `json:"total_ms"`
	Findings       []string `json:"findings"`
}
type Call struct {
	Summary             Summary
	Receipt             receipt.Receipt
	Supplements         []receipt.Supplement
	Envelope            json.RawMessage
	SupplementEnvelopes []json.RawMessage
}

func decode(b []byte, v any) error {
	var e receipt.Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return err
	}
	p, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(p, v)
}
func (r *Reader) Calls(f Filter) ([]Call, error) { return r.calls(f, "") }
func (r *Reader) Get(id string) (Call, error) {
	calls, err := r.calls(Filter{}, id)
	if err != nil {
		return Call{}, err
	}
	if len(calls) == 0 {
		return Call{}, sql.ErrNoRows
	}
	return calls[0], nil
}
func (r *Reader) calls(f Filter, id string) ([]Call, error) {
	where := []string{"1=1"}
	args := []any{}
	for _, p := range []struct{ col, val string }{{"id", id}, {"tag", f.Tag}, {"upstream", f.Upstream}} {
		if p.val != "" {
			where = append(where, p.col+"=?")
			args = append(args, p.val)
		}
	}
	if f.Model != "" {
		where = append(where, "(requested_model=? OR reported_model=?)")
		args = append(args, f.Model, f.Model)
	}
	// Stored timestamps have millisecond precision. Round bounds upward so both
	// inclusive since and exclusive until retain their meaning for finer input.
	boundary := func(s string) string {
		t, _ := time.Parse(time.RFC3339Nano, s)
		rounded := t.Truncate(time.Millisecond)
		if rounded.Before(t) {
			rounded = rounded.Add(time.Millisecond)
		}
		return receipt.Timestamp(rounded)
	}
	if f.Since != "" {
		where = append(where, "started_at>=?")
		args = append(args, boundary(f.Since))
	}
	if f.Until != "" {
		where = append(where, "started_at<?")
		args = append(args, boundary(f.Until))
	}
	selection := " FROM receipts WHERE " + strings.Join(where, " AND ") + " ORDER BY started_at DESC,id DESC"
	if f.Limit > 0 {
		selection += " LIMIT ?"
		args = append(args, f.Limit)
	}
	query := "SELECT id,started_at,tag,upstream,api,requested_model,reported_model,provider,status,input_tokens,output_tokens,cost_amount,cost_source,ttfb_ms,total_ms,envelope" + selection
	// A read transaction keeps summaries and supplement envelopes from the same snapshot.
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	calls := []Call{}
	for rows.Next() {
		var c Call
		s := &c.Summary
		err = rows.Scan(&s.ID, &s.Started, &s.Tag, &s.Upstream, &s.API, &s.RequestedModel, &s.ReportedModel, &s.Provider, &s.Status, &s.Input, &s.Output, &s.Cost, &s.CostSource, &s.TTFB, &s.Total, &c.Envelope)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if err = decode(c.Envelope, &c.Receipt); err != nil {
			rows.Close()
			return nil, err
		}
		calls = append(calls, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Call, len(calls))
	for i := range calls {
		c := &calls[i]
		c.Supplements = []receipt.Supplement{}
		c.SupplementEnvelopes = []json.RawMessage{}
		byID[c.Summary.ID] = c
	}
	if len(calls) > 0 {
		// Keep supplements as the outer scan: receipt_id has no index, and
		// probing supplements separately for each selected receipt is quadratic.
		sr, e := tx.Query("SELECT s.receipt_id,s.envelope FROM supplements AS s CROSS JOIN (SELECT id"+selection+") AS selected ON selected.id=s.receipt_id ORDER BY s.rowid", args...)
		if e != nil {
			return nil, e
		}
		for sr.Next() {
			var id string
			var b []byte
			var supplement receipt.Supplement
			if e = sr.Scan(&id, &b); e == nil {
				e = decode(b, &supplement)
			}
			if e != nil {
				sr.Close()
				return nil, e
			}
			c := byID[id]
			c.Supplements = append(c.Supplements, supplement)
			c.SupplementEnvelopes = append(c.SupplementEnvelopes, json.RawMessage(b))
		}
		e = sr.Err()
		sr.Close()
		if e != nil {
			return nil, e
		}
	}
	for i := range calls {
		c := &calls[i]
		var supp *receipt.Supplement
		if len(c.Supplements) > 0 {
			supp = &c.Supplements[len(c.Supplements)-1]
		}
		_, _, findings := receipt.Combined(&c.Receipt, supp)
		c.Summary.Findings = []string{}
		for _, v := range findings {
			c.Summary.Findings = append(c.Summary.Findings, v.Code)
		}
	}
	return calls, tx.Commit()
}
