package keys

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

func ParsePublic(b []byte) (ed25519.PublicKey, error) {
	p, rest := pem.Decode(b)
	if p == nil || p.Type != "PUBLIC KEY" || len(rest) != 0 {
		return nil, fmt.Errorf("invalid PEM public key; supply an Ed25519 public key")
	}
	v, err := x509.ParsePKIXPublicKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid public key")
	}
	k, ok := v.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key must be Ed25519")
	}
	return k, nil
}
func Public(dir string) ([]byte, ed25519.PublicKey, error) {
	b, err := os.ReadFile(filepath.Join(dir, "key.pub"))
	if err != nil {
		return nil, nil, err
	}
	k, err := ParsePublic(b)
	return b, k, err
}

// CheckPrivate only reads the existing private key, without repairing or creating files.
func CheckPrivate(dir string, pub ed25519.PublicKey) error {
	b, err := os.ReadFile(filepath.Join(dir, "key.ed25519"))
	if err != nil {
		return fmt.Errorf("private key is missing or unreadable; restore the original key or use a new --data-dir")
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return fmt.Errorf("invalid private key; restore the original key")
	}
	v, err := x509.ParsePKCS8PrivateKey(p.Bytes)
	if err != nil {
		return fmt.Errorf("invalid private key; restore the original key")
	}
	k, ok := v.(ed25519.PrivateKey)
	if !ok || !pub.Equal(k.Public()) {
		return fmt.Errorf("private and public keys differ; restore the original key pair")
	}
	return nil
}
