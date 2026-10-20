package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vincentsch/tollglass/internal/receipt"
)

const recovery = "Restore the original key and public key, or start with a new --data-dir."

func Load(dir, expected string, receiptsExist bool) (ed25519.PrivateKey, error) {
	path := filepath.Join(dir, "key.ed25519")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if expected != "" || receiptsExist {
			return nil, fmt.Errorf("private key is missing while the database has an identity or receipts. %s", recovery)
		}
		_, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return nil, e
		}
		der, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			return nil, e
		}
		if e = install(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); e != nil {
			if errors.Is(e, os.ErrExist) {
				return Load(dir, expected, receiptsExist)
			}
			return nil, fmt.Errorf("cannot create instance key")
		}
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read instance key. %s", recovery)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("invalid instance key. %s", recovery)
	}
	v, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid instance key. %s", recovery)
	}
	key, ok := v.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("instance key is not Ed25519. %s", recovery)
	}
	pub := key.Public().(ed25519.PublicKey)
	if expected != "" && expected != receipt.KeyID(pub) {
		return nil, fmt.Errorf("instance key does not match the database. %s", recovery)
	}
	der, _ := x509.MarshalPKIXPublicKey(pub)
	public := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	publicPath := filepath.Join(dir, "key.pub")
	old, e := os.ReadFile(publicPath)
	if e == nil {
		p, _ := pem.Decode(old)
		if p == nil {
			return nil, fmt.Errorf("invalid public key. %s", recovery)
		}
		pk, e := x509.ParsePKIXPublicKey(p.Bytes)
		k, ok := pk.(ed25519.PublicKey)
		if e != nil || !ok || !pub.Equal(k) {
			return nil, fmt.Errorf("public key does not match the private key. %s", recovery)
		}
	} else if errors.Is(e, os.ErrNotExist) {
		if e = install(publicPath, public); e != nil {
			return nil, fmt.Errorf("cannot create public key")
		}
	} else {
		return nil, fmt.Errorf("cannot read public key")
	}
	return key, nil
}
func install(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".key-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Linking the exclusive temp file publishes it atomically without replacing another startup's key.
	return os.Link(f.Name(), path)
}
