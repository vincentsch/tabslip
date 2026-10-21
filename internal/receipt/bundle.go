package receipt

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

const ClaimsLimit = "Model names, token counts and provider cost come from the provider. Tabslip did not check the model itself."

type Range struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Bundle struct {
	Type        string            `json:"type"`
	Created     string            `json:"created"`
	PublicKey   string            `json:"public_key"`
	KeyID       string            `json:"key_id"`
	Range       Range             `json:"range"`
	Receipts    []json.RawMessage `json:"receipts"`
	Supplements []json.RawMessage `json:"supplements"`
}
type EnvelopeResult struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}
type Verification struct {
	SignaturesValid bool             `json:"signatures_valid"`
	SignerConfirmed bool             `json:"signer_confirmed"`
	Signer          string           `json:"signer"`
	Claims          string           `json:"claims"`
	Envelopes       []EnvelopeResult `json:"envelopes"`
}

// VerifyEnvelopes checks labels as well as signatures; labels never establish trust.
func VerifyEnvelopes(b Bundle, pub ed25519.PublicKey) Verification {
	result := Verification{SignaturesValid: true, Claims: ClaimsLimit, Envelopes: []EnvelopeResult{}}
	id := KeyID(pub)
	digests := map[string]string{}
	check := func(raw []byte, kind string) ([]byte, error) {
		var e Envelope
		if json.Unmarshal(raw, &e) != nil {
			return nil, fmt.Errorf("malformed envelope")
		}
		p, err := base64.StdEncoding.DecodeString(e.Payload)
		if err != nil {
			return nil, fmt.Errorf("malformed payload encoding")
		}
		if e.PayloadType != kind || len(e.Signatures) != 1 || e.Signatures[0].KeyID != id {
			return p, fmt.Errorf("invalid payload type or key id")
		}
		sig, err := base64.StdEncoding.DecodeString(e.Signatures[0].Sig)
		if err != nil || !ed25519.Verify(pub, PAE(kind, p), sig) {
			return p, fmt.Errorf("invalid signature")
		}
		return p, nil
	}
	add := func(name string, err error) {
		r := EnvelopeResult{Name: name, Valid: err == nil}
		if err != nil {
			r.Error = err.Error()
			result.SignaturesValid = false
		}
		result.Envelopes = append(result.Envelopes, r)
	}
	if b.Type != "tabslip.bundle/v1" || b.KeyID != id {
		add("bundle", fmt.Errorf("invalid bundle type or key id"))
	}
	for i, raw := range b.Receipts {
		p, err := check(raw, PayloadType)
		var r Receipt
		parsed := json.Unmarshal(p, &r) == nil && r.ID != ""
		name := fmt.Sprintf("receipt %d of %d", i+1, len(b.Receipts))
		if parsed {
			name = r.ID
		}
		if err == nil && (!parsed || r.Type != "tabslip.receipt/v1" || r.Instance.KeyID != id) {
			err = fmt.Errorf("invalid receipt type or key id")
		}
		if err == nil {
			if _, exists := digests[r.ID]; exists {
				err = fmt.Errorf("duplicate receipt id")
			} else {
				digests[r.ID] = Digest(p)
			}
		}
		add(name, err)
	}
	for i, raw := range b.Supplements {
		p, err := check(raw, SupplementPayloadType)
		var s Supplement
		parsed := json.Unmarshal(p, &s) == nil && s.ReceiptID != ""
		name := fmt.Sprintf("supplement %d of %d", i+1, len(b.Supplements))
		if parsed {
			name += " for " + s.ReceiptID
		}
		if err == nil && (!parsed || s.Type != "tabslip.supplement/v1") {
			err = fmt.Errorf("invalid supplement type")
		}
		if err == nil && (digests[s.ReceiptID] == "" || digests[s.ReceiptID] != s.ReceiptSHA256) {
			err = fmt.Errorf("missing receipt or wrong receipt digest")
		}
		add(name, err)
	}
	return result
}

// WriteBundle preserves the exact envelope JSON, including its original whitespace.
func WriteBundle(w io.Writer, b Bundle) error {
	header := struct {
		Type      string `json:"type"`
		Created   string `json:"created"`
		PublicKey string `json:"public_key"`
		KeyID     string `json:"key_id"`
		Range     Range  `json:"range"`
	}{b.Type, b.Created, b.PublicKey, b.KeyID, b.Range}
	h, err := json.Marshal(header)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	out.Write(h[:len(h)-1])
	for _, part := range []struct {
		name      string
		envelopes []json.RawMessage
	}{{"receipts", b.Receipts}, {"supplements", b.Supplements}} {
		out.WriteString(",\"" + part.name + "\":[")
		for i, raw := range part.envelopes {
			if !json.Valid(raw) {
				return fmt.Errorf("invalid envelope JSON")
			}
			if i > 0 {
				out.WriteByte(',')
			}
			out.Write(raw)
		}
		out.WriteByte(']')
	}
	out.WriteString("}\n")
	_, err = w.Write(out.Bytes())
	return err
}
