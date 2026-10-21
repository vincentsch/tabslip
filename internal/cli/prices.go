package cli

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tabslip/internal/prices"
)

func pricesCommand(dir *string) *rungrad.Command {
	c := command("prices", "Show the selected local price table", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		_, info, err := prices.Selected(*dir)
		if err != nil {
			return rungrad.NewError(2, err.Error())
		}
		return f.WriteResult(info, func(w io.Writer) {
			fmt.Fprintf(w, "%s prices: %s\nDate: %s\nSHA-256: %s\n", info.Source, info.Version, info.Date, info.SHA256)
		})
	})
	c.Examples = []string{"tabslip prices", "tabslip prices --json"}
	u := command("update", "Download and install a LiteLLM price snapshot", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		if f.Flags.DryRun {
			return f.WriteResult(map[string]any{"dry_run": true, "source": prices.SourceURL}, func(w io.Writer) {
				fmt.Fprintf(w, "Would download %s and install a price snapshot. No changes were made.\n", prices.SourceURL)
			})
		}
		fmt.Fprintf(f.Stderr, "Downloading prices from %s\n", prices.SourceURL)
		client := &http.Client{Timeout: 30 * time.Second}
		defer client.CloseIdleConnections()
		b, name, err := prices.Download(client, prices.SourceURL, time.Now())
		if err != nil {
			return rungrad.NewError(2, err.Error())
		}
		if err = prices.Install(*dir, b, name); err != nil {
			return rungrad.NewError(2, "Cannot install the price snapshot. Check data directory permissions and free space.")
		}
		_, info, err := prices.Selected(*dir)
		if err != nil {
			return rungrad.NewError(2, err.Error())
		}
		return f.WriteResult(struct {
			prices.Info
			Notice string `json:"notice"`
		}{info, "Restart a running Tabslip to use this snapshot. Already signed costs never change."}, func(w io.Writer) {
			fmt.Fprintf(w, "Installed %s\nRestart a running Tabslip to use this snapshot. Already signed costs never change.\n", name)
		})
	})
	u.Mutates = true
	u.Examples = []string{"tabslip prices update", "tabslip prices update --dry-run --json"}
	c.AddCommand(u)
	return c
}
