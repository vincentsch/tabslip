package receipt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

const SupplementPayloadType = "application/vnd.tabslip.supplement+json"

type Attempt struct {
	Provider *string `json:"provider"`
	Status   *int    `json:"status"`
}
type NativeTokens struct {
	Prompt     *int64 `json:"prompt"`
	Completion *int64 `json:"completion"`
}
type Supplement struct {
	Type          string       `json:"type"`
	ReceiptID     string       `json:"receipt_id"`
	ReceiptSHA256 string       `json:"receipt_sha256"`
	Source        string       `json:"source"`
	Fetched       string       `json:"fetched"`
	Provider      *string      `json:"provider"`
	Attempts      []Attempt    `json:"attempts"`
	NativeTokens  NativeTokens `json:"native_tokens"`
	Cost          *string      `json:"cost"`
	Currency      string       `json:"currency"`
	DataRegion    *string      `json:"data_region"`
	Findings      []Finding    `json:"findings"`
}

func Digest(payload []byte) string { h := sha256.Sum256(payload); return hex.EncodeToString(h[:]) }
func SignSupplement(s *Supplement, key ed25519.PrivateKey) ([]byte, error) {
	p, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(key, PAE(SupplementPayloadType, p))
	return json.Marshal(Envelope{SupplementPayloadType, base64.StdEncoding.EncodeToString(p), []Signature{{KeyID(key.Public().(ed25519.PublicKey)), base64.StdEncoding.EncodeToString(sig)}}})
}

// Combined returns the active cost, provider and findings without changing signed records.
func Combined(r *Receipt, s *Supplement) (Cost, *string, []Finding) {
	cost, provider := r.Cost, r.Response.Provider
	findings := append([]Finding{}, r.Findings...)
	if s == nil {
		return cost, provider, findings
	}
	if s.Cost != nil {
		cost.Source, cost.Amount, cost.Currency = "reported", s.Cost, &s.Currency
		filtered := findings[:0]
		for _, f := range findings {
			if f.Code != "price_unknown" {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}
	if s.Provider != nil {
		provider = s.Provider
	}
	findings = append(findings, s.Findings...)
	return cost, provider, findings
}
