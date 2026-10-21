package receipt

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// Build envelopes independently of the production signing helpers.
func seal(t *testing.T, kind string, v any, key ed25519.PrivateKey) json.RawMessage {
	t.Helper()
	p, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(key, PAE(kind, p))
	b, err := json.Marshal(Envelope{kind, base64.StdEncoding.EncodeToString(p), []Signature{{KeyID(key.Public().(ed25519.PublicKey)), base64.StdEncoding.EncodeToString(sig)}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestVerificationChecksSignedTypesLabelsAndReferences(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	pub := key.Public().(ed25519.PublicKey)
	id := KeyID(pub)
	r := Receipt{Type: "tabslip.receipt/v1", ID: "rcpt_example", Instance: Instance{KeyID: id}}
	p, _ := json.Marshal(r)
	s := Supplement{Type: "tabslip.supplement/v1", ReceiptID: r.ID, ReceiptSHA256: Digest(p)}
	makeBundle := func() Bundle {
		return Bundle{Type: "tabslip.bundle/v1", KeyID: id, Receipts: []json.RawMessage{seal(t, PayloadType, r, key)}, Supplements: []json.RawMessage{seal(t, SupplementPayloadType, s, key)}}
	}
	if v := VerifyEnvelopes(makeBundle(), pub); !v.SignaturesValid {
		t.Fatal(v)
	}
	for _, tc := range []struct {
		name  string
		alter func(*Bundle)
	}{{"signed wrong receipt type", func(b *Bundle) { bad := r; bad.Type = "wrong"; b.Receipts[0] = seal(t, PayloadType, bad, key) }}, {"signed wrong receipt key label", func(b *Bundle) {
		bad := r
		bad.Instance.KeyID = "ts_wrong"
		b.Receipts[0] = seal(t, PayloadType, bad, key)
	}}, {"signed wrong digest", func(b *Bundle) {
		bad := s
		bad.ReceiptSHA256 = "wrong"
		b.Supplements[0] = seal(t, SupplementPayloadType, bad, key)
	}}, {"signed wrong supplement type", func(b *Bundle) {
		bad := s
		bad.Type = "wrong"
		b.Supplements[0] = seal(t, SupplementPayloadType, bad, key)
	}}, {"signed missing reference", func(b *Bundle) {
		bad := s
		bad.ReceiptID = "absent"
		b.Supplements[0] = seal(t, SupplementPayloadType, bad, key)
	}}, {"wrong bundle label", func(b *Bundle) { b.KeyID = "ts_wrong" }}} {
		t.Run(tc.name, func(t *testing.T) {
			b := makeBundle()
			tc.alter(&b)
			if v := VerifyEnvelopes(b, pub); v.SignaturesValid {
				t.Fatal(v)
			}
		})
	}
}

func TestBundleKeepsExactEnvelopeBytes(t *testing.T) {
	raw := json.RawMessage("{\n  \"payloadType\": \"sample\", \"payload\":\"YWJj\", \"signatures\": []\n}")
	b := Bundle{Type: "tabslip.bundle/v1", Receipts: []json.RawMessage{raw}, Supplements: []json.RawMessage{raw}}
	var output bytes.Buffer
	if err := WriteBundle(&output, b); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(output.Bytes()) || bytes.Count(output.Bytes(), raw) != 2 {
		t.Fatal(output.String())
	}
}
