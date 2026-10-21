package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/vincentsch/tabslip/internal/receipt"
)

func (s *Store) SaveSupplement(supplement *receipt.Supplement) {
	b, err := receipt.SignSupplement(supplement, s.Key)
	if err != nil {
		return
	}
	s.Enqueue(Record{Supplement: supplement, Envelope: b})
}
func (s *Store) insertSupplement(supplement *receipt.Supplement, envelope []byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var original []byte
	if err = tx.QueryRow("SELECT envelope FROM receipts WHERE id=?", supplement.ReceiptID).Scan(&original); err != nil {
		return err
	}
	var e receipt.Envelope
	if err = json.Unmarshal(original, &e); err != nil {
		return err
	}
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return err
	}
	if receipt.Digest(payload) != supplement.ReceiptSHA256 {
		return fmt.Errorf("supplement receipt digest mismatch")
	}
	var r receipt.Receipt
	if err = json.Unmarshal(payload, &r); err != nil {
		return err
	}
	// A completed lookup is stored once even if the caller repeats its callback.
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM supplements WHERE receipt_id=?", r.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cost, provider, findings := receipt.Combined(&r, supplement)
	r.Findings = findings
	if _, err = tx.Exec("INSERT INTO supplements(receipt_id,envelope) VALUES(?,?)", r.ID, envelope); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE receipts SET cost_amount=?,cost_source=?,provider=?,has_findings=? WHERE id=?", cost.Amount, cost.Source, provider, r.HasFindings(), r.ID); err != nil {
		return err
	}
	return s.commitChange(tx, "supplement", r.ID)
}
