package cli

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tollglass/internal/keys"
	"github.com/vincentsch/tollglass/internal/receipt"
)

func payload(e receipt.Envelope) (json.RawMessage, error) {
	b, err := base64.StdEncoding.DecodeString(e.Payload)
	return json.RawMessage(b), err
}

// Verdict errors carry an exit status after the complete verification result was printed.
type verdictError int

func (e verdictError) Error() string { return "" }
func (e verdictError) ExitCode() int { return int(e) }
func verifyCommand(dir *string) *rungrad.Command {
	var keyPath, expected string
	c := command("verify <file>", "Check signatures and signer identity offline", func(f *rungrad.Factory, _ *cobra.Command, args []string) error {
		if keyPath != "" && expected != "" {
			return rungrad.NewError(1, "Use either --key or --key-id, not both.")
		}
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return localError(err)
		}
		var b receipt.Bundle
		var probe struct {
			Type string `json:"type"`
		}
		json.Unmarshal(raw, &probe)
		single := probe.Type == ""
		var pub ed25519.PublicKey
		if single && keyPath == "" {
			return rungrad.NewError(1, "A single envelope needs --key with its public key file.")
		}
		if keyPath != "" {
			kb, e := os.ReadFile(keyPath)
			if e != nil {
				return localError(e)
			}
			pub, err = keys.ParsePublic(kb)
			if err != nil {
				return rungrad.NewError(2, err.Error())
			}
		}
		if single {
			b = receipt.Bundle{Type: "tollglass.bundle/v1", KeyID: receipt.KeyID(pub), Receipts: []json.RawMessage{raw}}
		} else {
			if err = json.Unmarshal(raw, &b); err != nil {
				return invalidFile(f, "malformed bundle")
			}
			bundled, e := keys.ParsePublic([]byte(b.PublicKey))
			if e != nil {
				return invalidFile(f, "invalid bundle public key")
			}
			if keyPath == "" {
				pub = bundled
			} else if !pub.Equal(bundled) { // Still inspect envelopes with the explicitly selected key.
				b.KeyID = "mismatched bundle public key"
			}
		}
		result := receipt.VerifyEnvelopes(b, pub)
		id := receipt.KeyID(pub)
		switch {
		case !result.SignaturesValid:
			result.Signer = "Not confirmed: invalid signatures or records prevent signer confirmation."
		case keyPath != "":
			result.SignerConfirmed = true
			result.Signer = "Trusted: the key matches the key you gave with --key."
		case expected != "":
			result.SignerConfirmed = expected == id
			if result.SignerConfirmed {
				result.Signer = "Trusted: the signer matches --key-id."
			} else {
				result.Signer = "Not confirmed: signer mismatch; expected " + expected + ", got " + id + "."
			}
		default:
			_, local, e := keys.Public(*dir)
			result.SignerConfirmed = e == nil && local.Equal(pub)
			if result.SignerConfirmed {
				result.Signer = "Trusted: the signer matches the existing local instance key."
			} else {
				result.Signer = "Not confirmed: ask the sender for their public key or key id through another channel. Bundle signer: " + id + "."
			}
		}
		return printVerification(f, result)
	})
	c.Args = cobra.ExactArgs(1)
	c.Configure = func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&keyPath, "key", "", "PEM public key confirmed through another channel")
		cmd.Flags().StringVar(&expected, "key-id", "", "Expected signer key id")
	}
	c.Examples = []string{"tollglass verify calls.tollglass.json --key tollglass.pub", "tollglass verify calls.tollglass.json --key-id tg_6f1c2a9b04d3e871 --json"}
	return c
}
func invalidFile(f *rungrad.Factory, reason string) error {
	return printVerification(f, receipt.Verification{SignaturesValid: false, Signer: "Not confirmed: invalid file.", Claims: receipt.ClaimsLimit, Envelopes: []receipt.EnvelopeResult{{Name: "file", Error: reason}}})
}
func printVerification(f *rungrad.Factory, r receipt.Verification) error {
	err := f.WriteResult(r, func(w io.Writer) {
		valid := 0
		for _, e := range r.Envelopes {
			if e.Valid {
				valid++
			} else {
				fmt.Fprintf(w, "INVALID %s: %s\n", e.Name, e.Error)
			}
		}
		fmt.Fprintf(w, "Signatures: %d of %d envelopes valid.\nSigner: %s\nLimits: %s\n", valid, len(r.Envelopes), r.Signer, r.Claims)
	})
	if err != nil {
		return err
	}
	if !r.SignaturesValid {
		return verdictError(10)
	}
	if !r.SignerConfirmed {
		return verdictError(11)
	}
	return nil
}
