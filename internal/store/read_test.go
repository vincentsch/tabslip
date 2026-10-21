package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/vincentsch/tabslip/internal/receipt"
	"testing"
	"time"
)

func TestSummaryDatesLimitAndRawUsage(t *testing.T) {
	dir := t.TempDir()
	writer, err := Open(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	n := int64(2)
	for i := 0; i < 25; i++ {
		r := receipt.New(writer.KeyID, time.Date(2026, 10, 20, 0, 0, i, 0, time.UTC))
		r.Request.API = "anthropic.messages"
		r.Request.Upstream = "anthropic"
		r.Usage.Input = &n
		r.Usage.Cached = &n
		r.Usage.CacheWrite = &n
		writer.Save(r, nil)
	}
	writer.Flush()
	reader, err := OpenReader(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	filter, err := (Filter{Since: "2026-10-20T02:00:00+02:00", Until: "2026-10-20T00:00:03Z"}).Validate()
	if err != nil {
		t.Fatal(err)
	}
	calls, err := reader.Calls(filter)
	if err != nil || len(calls) != 3 {
		t.Fatal(len(calls), err)
	}
	if *calls[0].Summary.Input != 6 || *calls[0].Receipt.Usage.Input != 2 || calls[0].Summary.Started != "2026-10-20T00:00:02.000Z" {
		t.Fatal(calls[0])
	}
	fine, e := (Filter{Since: "2026-10-20T00:00:00.0001Z", Until: "2026-10-20T00:00:01.0001Z"}).Validate()
	if e != nil {
		t.Fatal(e)
	}
	bounded, e := reader.Calls(fine)
	if e != nil || len(bounded) != 1 || bounded[0].Summary.Started != "2026-10-20T00:00:01.000Z" {
		t.Fatal(bounded, e)
	}
	calls, err = reader.Calls(Filter{Limit: 20})
	if err != nil || len(calls) != 20 || calls[0].Summary.Started != "2026-10-20T00:00:24.000Z" {
		t.Fatal(len(calls), err)
	}
	calls, err = reader.Calls(Filter{})
	if err != nil || len(calls) != 25 {
		t.Fatal(len(calls), err)
	}
}

func supplementedReader(tb testing.TB, count, perCall int) *Reader {
	tb.Helper()
	dir := tb.TempDir()
	writer, err := Open(dir, 0, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { writer.Close() })
	tx, err := writer.db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO receipts(id,started_at,upstream,api,tag,cost_amount,cost_source,total_ms,has_findings,envelope) VALUES(?,?,?,?,?,?,'reported',0,0,?)`)
	if err != nil {
		tb.Fatal(err)
	}
	defer insert.Close()
	add, err := tx.Prepare("INSERT INTO supplements(receipt_id,envelope) VALUES(?,?)")
	if err != nil {
		tb.Fatal(err)
	}
	defer add.Close()
	for i := 0; i < count; i++ {
		r := receipt.New(writer.KeyID, time.Date(2026, 10, 20, 0, 0, 0, i*1000000, time.UTC))
		r.Request.API = "openai.chat"
		r.Request.Upstream = "openrouter"
		r.Request.Tag = "other"
		if i%2 == 0 {
			r.Request.Tag = "selected"
		}
		r.Find("price_unknown", "observed", "The cost of this call is unknown.")
		envelope, e := receipt.Sign(r, writer.Key)
		if e != nil {
			tb.Fatal(e)
		}
		if _, e = insert.Exec(r.ID, r.Time.Started, r.Request.Upstream, r.Request.API, r.Request.Tag, "0.01", envelope); e != nil {
			tb.Fatal(e)
		}
		var decoded receipt.Envelope
		json.Unmarshal(envelope, &decoded)
		payload, e := base64.StdEncoding.DecodeString(decoded.Payload)
		if e != nil {
			tb.Fatal(e)
		}
		for j := 0; j < perCall; j++ {
			cost := "0.01"
			supp := receipt.Supplement{Type: "tabslip.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(payload), Cost: &cost, Currency: "USD", Fetched: fmt.Sprint(j)}
			raw, e := receipt.SignSupplement(&supp, writer.Key)
			if e != nil {
				tb.Fatal(e)
			}
			if _, e = add.Exec(r.ID, raw); e != nil {
				tb.Fatal(e)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	reader, err := OpenReader(dir)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { reader.Close() })
	return reader
}
func TestSelectedSupplementsKeepAssociationAndOrder(t *testing.T) {
	reader := supplementedReader(t, 30, 2)
	for _, filter := range []Filter{{}, {Limit: 1}, {Tag: "selected", Limit: 7}, {Tag: "other"}, {Tag: "absent"}} {
		calls, err := reader.Calls(filter)
		if err != nil {
			t.Fatal(err)
		}
		want := 30
		if filter.Tag != "" {
			want = 15
		}
		if filter.Tag == "absent" {
			want = 0
		}
		if filter.Limit > 0 {
			want = filter.Limit
		}
		if len(calls) != want {
			t.Fatal(len(calls), want)
		}
		for _, c := range calls {
			if len(c.Supplements) != 2 || len(c.SupplementEnvelopes) != 2 || len(c.Summary.Findings) != 0 {
				t.Fatal(c)
			}
			for j, s := range c.Supplements {
				if s.ReceiptID != c.Summary.ID || s.Fetched != fmt.Sprint(j) {
					t.Fatal(c)
				}
			}
			if filter.Tag != "" && (c.Summary.Tag == nil || *c.Summary.Tag != filter.Tag) {
				t.Fatal(c)
			}
		}
		if len(calls) > 0 {
			one, err := reader.Get(calls[0].Summary.ID)
			if err != nil || len(one.Supplements) != 2 || one.Supplements[1].Fetched != "1" {
				t.Fatal(one, err)
			}
		}
	}
}
func BenchmarkReadSupplementedReceipts(b *testing.B) {
	for _, size := range []int{2000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			reader := supplementedReader(b, size, 1)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				calls, err := reader.Calls(Filter{})
				if err != nil || len(calls) != size {
					b.Fatal(len(calls), err)
				}
			}
		})
	}
}

func TestKeysetOrdersCombinedSearchAndActiveFindings(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	for i := 0; i < 423; i++ {
		r := sample(s, time.Date(2026, 10, 20, 0, 0, i/3, 0, time.UTC))
		r.Time.Total = int64(i / 4)
		tag := "project"
		r.Request.Tag = tag
		s.Save(r, nil)
		if i%100 == 0 {
			if err := s.Flush(); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.Flush()
	for _, slow := range []bool{false, true} {
		f := Filter{Tag: "project", Slowest: slow, Limit: 200}
		seen := map[string]bool{}
		var previous Summary
		for {
			calls, err := s.Reader().Calls(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range calls {
				v := c.Summary
				if seen[v.ID] {
					t.Fatal("duplicate cursor row")
				}
				seen[v.ID] = true
				if previous.ID != "" {
					if slow && (previous.Total < v.Total || (previous.Total == v.Total && previous.ID < v.ID)) {
						t.Fatal("slowest order lost")
					}
					if !slow && (previous.Started < v.Started || (previous.Started == v.Started && previous.ID < v.ID)) {
						t.Fatal("newest order lost")
					}
				}
				previous = v
			}
			if len(calls) == 0 {
				break
			}
			last := calls[len(calls)-1].Summary
			f.Cursor = last.Started
			if slow {
				f.Cursor = fmt.Sprint(last.Total)
			}
			f.CursorID = last.ID
		}
		if len(seen) != 423 {
			t.Fatal("pagination lost calls", len(seen))
		}
	}
	r := sample(s, time.Now())
	asked, reported, provider := "Requested", "Reported", "Original provider"
	r.Request.Model = &asked
	r.Response.Model = &reported
	r.Response.Provider = &provider
	r.Find("price_unknown", "observed", "Unknown cost.")
	s.Save(r, &Content{Request: []byte("private word")})
	s.Flush()
	c := "0.5"
	p, _ := json.Marshal(r)
	supp := &receipt.Supplement{Type: "tabslip.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(p), Cost: &c, Currency: "USD"}
	s.SaveSupplement(supp)
	s.Flush()
	active, err := s.Reader().Calls(Filter{Model: "Requested", FindingsOnly: true})
	if err != nil || len(active) != 0 {
		t.Fatal("recovered pricing still warns")
	}
	active, err = s.Reader().Calls(Filter{Model: "Reported", Search: "ORIGINAL"})
	if err != nil || len(active) != 1 || len(active[0].Receipt.Findings) != 1 {
		t.Fatal("combined search or signed findings lost")
	}
}
