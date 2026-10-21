package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tabslip/internal/keys"
	"github.com/vincentsch/tabslip/internal/receipt"
)

func keyCommand(dir *string) *rungrad.Command {
	c := command("key", "Show the existing instance public key", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		b, k, err := keys.Public(*dir)
		if err != nil {
			return localError(err)
		}
		h := sha256.Sum256(k)
		r := struct {
			PublicKey   string `json:"public_key"`
			KeyID       string `json:"key_id"`
			Fingerprint string `json:"fingerprint"`
		}{string(b), receipt.KeyID(k), hex.EncodeToString(h[:])}
		return f.WriteResult(r, func(w io.Writer) { fmt.Fprintf(w, "Key id: %s\nSHA-256: %s\n%s", r.KeyID, r.Fingerprint, b) })
	})
	c.Examples = []string{"tabslip key", "tabslip key --json"}
	var out string
	ex := command("export", "Export the existing PEM public key", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		if err := available(out); err != nil {
			return err
		}
		b, k, err := keys.Public(*dir)
		if err != nil {
			return localError(err)
		}
		r := map[string]any{"out": out, "key_id": receipt.KeyID(k), "dry_run": f.Flags.DryRun}
		if f.Flags.DryRun {
			return f.WriteResult(r, func(w io.Writer) { fmt.Fprintf(w, "Would write the public key to %s. No changes were made.\n", out) })
		}
		if err = writeNew(out, func(w io.Writer) error { _, err := w.Write(b); return err }); err != nil {
			return err
		}
		return f.WriteResult(r, func(w io.Writer) { fmt.Fprintf(w, "Wrote public key to %s.\n", out) })
	})
	ex.Mutates = true
	ex.Configure = func(cmd *cobra.Command) { cmd.Flags().StringVar(&out, "out", "", "New PEM output file (required)") }
	ex.Examples = []string{"tabslip key export --out tabslip.pub", "tabslip key export --out tabslip.pub --dry-run"}
	c.AddCommand(ex)
	return c
}
