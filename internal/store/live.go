package store

import (
	"database/sql"
	"time"
)

type Change struct{ Kind, ID string }

// Subscribe never makes the writer wait for a browser. A slow reader gets a refresh.
func (s *Store) Subscribe() (<-chan Change, func()) {
	ch := make(chan Change, 64)
	s.eventsMu.Lock()
	s.subs[ch] = struct{}{}
	s.eventsMu.Unlock()
	return ch, func() { s.eventsMu.Lock(); delete(s.subs, ch); s.eventsMu.Unlock() }
}
func (s *Store) publish(c Change) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- c:
		default:
		drain:
			for {
				select {
				case <-ch:
				default:
					break drain
				}
			}
			ch <- Change{Kind: "refresh"}
		}
	}
}
func (s *Store) Reader() *Reader { return s.reader }
func (r *Reader) Summaries(f Filter) ([]Summary, error) {
	selection, args := selection(f, "")
	rows, err := r.db.Query("SELECT id,started_at,tag,upstream,api,requested_model,reported_model,provider,status,input_tokens,output_tokens,cost_amount,cost_source,ttfb_ms,total_ms,has_findings"+selection, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var v Summary
		var finding bool
		if err = rows.Scan(&v.ID, &v.Started, &v.Tag, &v.Upstream, &v.API, &v.RequestedModel, &v.ReportedModel, &v.Provider, &v.Status, &v.Input, &v.Output, &v.Cost, &v.CostSource, &v.TTFB, &v.Total, &finding); err != nil {
			return nil, err
		}
		if finding {
			v.Findings = []string{"findings"}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Reader) Content(id string) (*Content, error) {
	c := &Content{}
	err := r.db.QueryRow("SELECT request_body,response_text,truncated FROM contents WHERE receipt_id=?", id).Scan(&c.Request, &c.Response, &c.Truncated)
	return c, err
}
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM receipts WHERE id=?", id)
	if err == nil {
		s.publish(Change{Kind: "refresh"})
	}
	return err
}
func (s *Store) SetRetention(days int) error {
	if days != 0 {
		if _, err := s.db.Exec("DELETE FROM receipts WHERE started_at < ?", time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05.000Z")); err != nil {
			return err
		}
	}
	s.days.Store(int64(days))
	s.publish(Change{Kind: "refresh"})
	return nil
}
func (r *Reader) Options(column string) ([]string, error) {
	// Only fixed column names from the page are accepted.
	if column != "tag" && column != "upstream" && column != "requested_model" && column != "reported_model" {
		return nil, nil
	}
	rows, err := r.db.Query("SELECT DISTINCT " + column + " FROM receipts WHERE " + column + " IS NOT NULL ORDER BY 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Reader) Matching(id string, f Filter) ([]Call, error) {
	f.Cursor = ""
	f.CursorID = ""
	f.Limit = 1
	return r.calls(f, id)
}

func (s *Store) commitChange(tx *sql.Tx, kind, id string) error {
	if err := tx.Commit(); err != nil {
		return err
	}
	s.publish(Change{Kind: kind, ID: id})
	return nil
}
