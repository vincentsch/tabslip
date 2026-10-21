package receipt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/vincentsch/tabslip/internal/version"
)

const PayloadType = "application/vnd.tabslip.receipt+json"

type Finding struct {
	Code  string `json:"code"`
	Level string `json:"level"`
	Text  string `json:"text"`
}
type Instance struct {
	KeyID     string `json:"key_id"`
	Tabslip string `json:"tabslip"`
}
type Timing struct {
	Started   string  `json:"started"`
	FirstByte *string `json:"first_byte"`
	Finished  string  `json:"finished"`
	TTFB      *int64  `json:"ttfb_ms"`
	Total     int64   `json:"total_ms"`
}
type Request struct {
	Upstream      string         `json:"upstream"`
	API           string         `json:"api"`
	Method        string         `json:"method"`
	Path          string         `json:"path"`
	Model         *string        `json:"model"`
	Stream        *bool          `json:"stream"`
	Params        map[string]any `json:"params"`
	Tag           string         `json:"tag,omitempty"`
	BodySHA256    string         `json:"body_sha256"`
	BodyBytes     int64          `json:"body_bytes"`
	Modifications []string       `json:"modifications"`
}
type ProviderError struct {
	Type *string `json:"type"`
	Code any     `json:"code"`
}
type Response struct {
	Status            *int           `json:"status"`
	Complete          bool           `json:"complete"`
	APIStatus         *string        `json:"api_status"`
	ID                *string        `json:"id"`
	Model             *string        `json:"model"`
	Provider          *string        `json:"provider"`
	FinishReason      *string        `json:"finish_reason"`
	SystemFingerprint *string        `json:"system_fingerprint"`
	Error             *ProviderError `json:"error"`
	BodySHA256        string         `json:"body_sha256"`
	BodyBytes         int64          `json:"body_bytes"`
}
type Usage struct {
	Source       string `json:"source"`
	Input        *int64 `json:"input_tokens"`
	Output       *int64 `json:"output_tokens"`
	Cached       *int64 `json:"cached_input_tokens"`
	CacheWrite   *int64 `json:"cache_write_tokens"`
	CacheWrite1h *int64 `json:"cache_write_1h_tokens"`
	Reasoning    *int64 `json:"reasoning_tokens"`
}
type Cost struct {
	Source     string  `json:"source"`
	Amount     *string `json:"amount"`
	Currency   *string `json:"currency"`
	Estimate   *string `json:"estimate"`
	PriceTable *string `json:"price_table"`
}
type Receipt struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Instance Instance  `json:"instance"`
	Time     Timing    `json:"time"`
	Request  Request   `json:"request"`
	Response Response  `json:"response"`
	Usage    Usage     `json:"usage"`
	Cost     Cost      `json:"cost"`
	Findings []Finding `json:"findings"`
}

func Timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
func New(keyID string, started time.Time) *Receipt {
	return &Receipt{Type: "tabslip.receipt/v1", ID: "rcpt_" + ulid.Make().String(), Instance: Instance{keyID, version.Version}, Time: Timing{Started: Timestamp(started)}, Request: Request{Params: map[string]any{}, Modifications: []string{}}, Usage: Usage{Source: "unknown"}, Cost: Cost{Source: "unknown"}, Findings: []Finding{}}
}
func (r *Receipt) Find(code, level, text string) {
	for _, f := range r.Findings {
		if f.Code == code {
			return
		}
	}
	r.Findings = append(r.Findings, Finding{code, level, text})
}
func (r *Receipt) HasFindings() bool {
	for _, f := range r.Findings {
		if f.Code != "model_alias_resolved" {
			return true
		}
	}
	return false
}
func KeyID(pub ed25519.PublicKey) string {
	s := sha256.Sum256(pub)
	return "ts_" + hex.EncodeToString(s[:8])
}

type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}

func PAE(kind string, payload []byte) []byte {
	return append([]byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(kind), kind, len(payload))), payload...)
}
func Sign(r *Receipt, key ed25519.PrivateKey) ([]byte, error) {
	p, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(key, PAE(PayloadType, p))
	return json.Marshal(Envelope{PayloadType, base64.StdEncoding.EncodeToString(p), []Signature{{KeyID(key.Public().(ed25519.PublicKey)), base64.StdEncoding.EncodeToString(sig)}}})
}
func Verify(b []byte, pub ed25519.PublicKey) ([]byte, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	p, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return nil, err
	}
	if e.PayloadType != PayloadType || len(e.Signatures) != 1 || e.Signatures[0].KeyID != KeyID(pub) {
		return nil, fmt.Errorf("invalid envelope type or key id")
	}
	s, err := base64.StdEncoding.DecodeString(e.Signatures[0].Sig)
	if err != nil || !ed25519.Verify(pub, PAE(e.PayloadType, p), s) {
		return nil, fmt.Errorf("invalid signature")
	}
	var r Receipt
	if json.Unmarshal(p, &r) != nil || r.Instance.KeyID != KeyID(pub) {
		return nil, fmt.Errorf("invalid receipt key id")
	}
	return p, nil
}

// DisplayInput includes Anthropic's separately reported cache categories.
// Usage.Input remains the raw uncached count used for pricing and signed exports.
func (r *Receipt) DisplayInput() *int64 {
	if r.Request.API != "anthropic.messages" {
		return r.Usage.Input
	}
	if r.Usage.Input == nil || r.Usage.Cached == nil || r.Usage.CacheWrite == nil {
		return nil
	}
	total := *r.Usage.Input + *r.Usage.Cached + *r.Usage.CacheWrite
	return &total
}
