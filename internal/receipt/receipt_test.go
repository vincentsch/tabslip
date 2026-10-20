package receipt

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestDSSESignatureAndNulls(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	r := New(KeyID(pub), time.Date(2026, 10, 20, 10, 20, 30, 123000000, time.UTC))
	b, e := Sign(r, key)
	if e != nil {
		t.Fatal(e)
	}
	p, e := Verify(b, pub)
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	json.Unmarshal(p, &v)
	for section, fields := range map[string][]string{"response": {"status", "model", "id", "error", "api_status", "system_fingerprint", "finish_reason", "provider"}, "usage": {"input_tokens", "output_tokens", "cached_input_tokens", "cache_write_tokens", "cache_write_1h_tokens", "reasoning_tokens"}, "cost": {"amount", "currency", "estimate", "price_table"}, "time": {"first_byte", "ttfb_ms"}} {
		m := v[section].(map[string]any)
		for _, k := range fields {
			x, ok := m[k]
			if !ok || x != nil {
				t.Fatalf("%s.%s not null", section, k)
			}
		}
	}
	var env Envelope
	json.Unmarshal(b, &env)
	sig, _ := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
	pae := []byte("DSSEv1 38 application/vnd.tollglass.receipt+json " + strconv.Itoa(len(p)) + " ")
	pae = append(pae, p...)
	if !ed25519.Verify(pub, pae, sig) {
		t.Fatal("signature does not use DSSE PAE")
	}
	p[len(p)-2] ^= 1
	env.Payload = base64.StdEncoding.EncodeToString(p)
	tampered, _ := json.Marshal(env)
	if _, e = Verify(tampered, pub); e == nil {
		t.Fatal("tamper accepted")
	}
	env.PayloadType = "wrong"
	tampered, _ = json.Marshal(env)
	if _, e = Verify(tampered, pub); e == nil {
		t.Fatal("wrong type accepted")
	}
}
