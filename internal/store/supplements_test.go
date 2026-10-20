package store

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/receipt"
)

func TestSupplementTransactionPreservesSignedRecords(t *testing.T) {
	for _, extra := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovered-cost", true: "other-findings-remain"}[extra], func(t *testing.T) {
			s := openTest(t, t.TempDir(), 0)
			defer s.Close()
			r := sample(s, time.Now())
			provider := "Known provider"
			r.Response.Provider = &provider
			r.Find("price_unknown", "observed", "Unknown cost.")
			r.Find("model_alias_resolved", "reported", "Alias.")
			if extra {
				r.Find("usage_missing", "observed", "Unknown usage.")
			}
			original, err := receipt.Sign(r, s.Key)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.insert(Record{Receipt: r, Envelope: original}); err != nil {
				t.Fatal(err)
			}
			var amount *string
			var source, gotProvider string
			var flag bool
			var before []byte
			read := func() {
				t.Helper()
				if err := s.db.QueryRow("SELECT cost_amount,cost_source,provider,has_findings,envelope FROM receipts WHERE id=?", r.ID).Scan(&amount, &source, &gotProvider, &flag, &before); err != nil {
					t.Fatal(err)
				}
			}
			read()
			if amount != nil || source != "unknown" || !flag {
				t.Fatal("before summary")
			}
			var e receipt.Envelope
			json.Unmarshal(original, &e)
			payload, _ := base64.StdEncoding.DecodeString(e.Payload)
			zero := "0"
			supplement := &receipt.Supplement{Type: "tollglass.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(payload), Source: "openrouter.generation", Cost: &zero, Currency: "USD", Attempts: []receipt.Attempt{}, Findings: []receipt.Finding{}}
			signed, err := receipt.SignSupplement(supplement, s.Key)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.insertSupplement(supplement, signed); err != nil {
				t.Fatal(err)
			}
			read()
			if amount == nil || *amount != "0" || source != "reported" || gotProvider != provider || flag != extra || !bytes.Equal(before, original) {
				t.Fatal("combined summary or immutable receipt changed")
			}
			var stored []byte
			if err = s.db.QueryRow("SELECT envelope FROM supplements WHERE receipt_id=?", r.ID).Scan(&stored); err != nil || !bytes.Equal(stored, signed) {
				t.Fatal("supplement bytes", err)
			}
			var se receipt.Envelope
			json.Unmarshal(stored, &se)
			sp, _ := base64.StdEncoding.DecodeString(se.Payload)
			signature, _ := base64.StdEncoding.DecodeString(se.Signatures[0].Sig)
			// Construct the DSSE framing separately from production signing helpers.
			pae := append([]byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(se.PayloadType), se.PayloadType, len(sp))), sp...)
			if se.PayloadType != receipt.SupplementPayloadType || !ed25519.Verify(s.Key.Public().(ed25519.PublicKey), pae, signature) {
				t.Fatal("signature")
			}
			var decoded receipt.Supplement
			json.Unmarshal(sp, &decoded)
			digest := sha256.Sum256(payload)
			if decoded.ReceiptSHA256 != hex.EncodeToString(digest[:]) {
				t.Fatal("digest")
			}
			if err = s.insertSupplement(supplement, signed); err != nil {
				t.Fatal(err)
			}
			var count int
			s.db.QueryRow("SELECT count(*) FROM supplements").Scan(&count)
			if count != 1 {
				t.Fatal("counted twice")
			}
			cost, p, findings := receipt.Combined(r, supplement)
			if cost.Amount == nil || *cost.Amount != "0" || p == nil || *p != provider {
				t.Fatal("combined record")
			}
			for _, f := range findings {
				if f.Code == "price_unknown" {
					t.Fatal("recovered unknown cost still active")
				}
			}
			if len(r.Findings) != 2+map[bool]int{true: 1, false: 0}[extra] {
				t.Fatal("mutated signed findings")
			}
		})
	}
}
func TestNullSupplementCostKeepsReceiptSummary(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	r := sample(s, time.Now())
	amount, usd, provider := "0.00100", "USD", "Original"
	r.Cost = receipt.Cost{Source: "reported", Amount: &amount, Currency: &usd}
	r.Response.Provider = &provider
	original, _ := receipt.Sign(r, s.Key)
	if err := s.insert(Record{Receipt: r, Envelope: original}); err != nil {
		t.Fatal(err)
	}
	var e receipt.Envelope
	json.Unmarshal(original, &e)
	p, _ := base64.StdEncoding.DecodeString(e.Payload)
	supplement := &receipt.Supplement{ReceiptID: r.ID, ReceiptSHA256: receipt.Digest(p), Attempts: []receipt.Attempt{}, Findings: []receipt.Finding{}}
	b, _ := receipt.SignSupplement(supplement, s.Key)
	if err := s.insertSupplement(supplement, b); err != nil {
		t.Fatal(err)
	}
	var cost, gotProvider string
	s.db.QueryRow("SELECT cost_amount,provider FROM receipts WHERE id=?", r.ID).Scan(&cost, &gotProvider)
	if cost != amount || gotProvider != provider {
		t.Fatal("null erased summary")
	}
}

func TestRejectedSupplementKeepsRecordedCallUsable(t *testing.T) {
	s := openTest(t, t.TempDir(), 0)
	defer s.Close()
	r := sample(s, time.Now())
	s.Save(r, nil)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s.SaveSupplement(&receipt.Supplement{ReceiptID: r.ID, ReceiptSHA256: "wrong-payload-digest"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM supplements").Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid supplement persisted", err)
	}
	if s.Unrecorded() != 0 {
		t.Fatal("optional supplement failure counted as an unrecorded call")
	}
	if err := s.db.QueryRow("SELECT count(*) FROM receipts WHERE id=?", r.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("original receipt lost", err)
	}
}
