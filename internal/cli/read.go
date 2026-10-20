package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/store"
)

func command(use, short string, run func(*rungrad.Factory, *cobra.Command, []string) error) *rungrad.Command {
	return &rungrad.Command{Use: use, Short: short, Long: short + ". Content stays on your local machine.", Examples: []string{"tollglass " + strings.Fields(use)[0] + " --help"}, Related: []string{"tollglass receipts", "tollglass export", "tollglass verify"}, OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON}, Args: cobra.NoArgs, Run: run}
}
func localError(err error) error {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		return rungrad.NewError(5, "The requested file or receipt was not found. Check its path or id and --data-dir.")
	}
	return rungrad.NewError(2, "Cannot read local records. Check the data directory and file permissions.")
}
func filterFlags(cmd *cobra.Command, f *store.Filter, list bool) {
	cmd.Flags().StringVar(&f.Tag, "tag", "", "Select a tag exactly")
	cmd.Flags().StringVar(&f.Upstream, "upstream", "", "Select an upstream exactly")
	cmd.Flags().StringVar(&f.Model, "model", "", "Select the requested or reported model exactly")
	cmd.Flags().StringVar(&f.Since, "since", "", "Include calls from this date (RFC 3339 or YYYY-MM-DD, UTC)")
	cmd.Flags().StringVar(&f.Until, "until", "", "Exclude calls from this date onward")
	if list {
		cmd.Flags().IntVar(&f.Limit, "limit", 20, "Maximum number of receipts (0 for all)")
	}
}
func readCalls(dir string, filter store.Filter) ([]store.Call, error) {
	f, err := filter.Validate()
	if err != nil {
		return nil, rungrad.NewError(1, err.Error())
	}
	r, err := store.OpenReader(dir)
	if err != nil {
		return nil, localError(err)
	}
	defer r.Close()
	calls, err := r.Calls(f)
	if err != nil {
		return nil, localError(err)
	}
	return calls, nil
}
func receiptsCommand(dir *string) *rungrad.Command {
	var filter store.Filter
	c := command("receipts", "List recent receipts, newest first", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		calls, err := readCalls(*dir, filter)
		if err != nil {
			return err
		}
		rows := []store.Summary{}
		for _, c := range calls {
			rows = append(rows, c.Summary)
		}
		return f.WriteResult(rows, func(w io.Writer) {
			if len(rows) == 0 {
				fmt.Fprintln(w, "No receipts match these filters.")
			}
			for _, r := range rows {
				fmt.Fprintf(w, "%s  %s  %s → %s  %s USD (%s)\n", r.ID, r.Started, value(r.RequestedModel), value(r.ReportedModel), value(r.Cost), r.CostSource)
			}
		})
	})
	c.Configure = func(cmd *cobra.Command) { filterFlags(cmd, &filter, true) }
	c.Examples = []string{"tollglass receipts", "tollglass receipts --tag acme-shop --since 2026-10-01 --json"}
	return c
}
func showCommand(dir *string) *rungrad.Command {
	c := command("show <id>", "Show one signed receipt", func(f *rungrad.Factory, _ *cobra.Command, args []string) error {
		r, err := store.OpenReader(*dir)
		if err != nil {
			return localError(err)
		}
		defer r.Close()
		call, err := r.Get(args[0])
		if err != nil {
			return localError(err)
		}
		return f.WriteResult(call.Envelope, func(w io.Writer) { renderReceiptStyled(w, call, f.TerminalMode().Color) })
	})
	c.Args = cobra.ExactArgs(1)
	c.Examples = []string{"tollglass show rcpt_01JB8Y3Q6V9W2N4K8M0P5R7S1T", "tollglass show rcpt_01JB8Y3Q6V9W2N4K8M0P5R7S1T --json"}
	return c
}
func value[T any](p *T) string {
	if p == nil {
		return "unknown"
	}
	return fmt.Sprint(*p)
}

// Terminal control characters in provider text must not become terminal commands.
func terminal(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func wrap(s string, width int) []string {
	r := []rune(terminal(s))
	out := []string{}
	for len(r) > width {
		cut := width
		for i := width - 1; i > width/3; i-- {
			if strings.ContainsRune("/-. ", r[i]) {
				cut = i + 1
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	return append(out, string(r))
}
func renderReceipt(w io.Writer, c store.Call) { renderReceiptStyled(w, c, false) }
func renderReceiptStyled(w io.Writer, c store.Call, color bool) {
	const width = 40
	line := func(s string) {
		for _, part := range wrap(s, width) {
			fmt.Fprintf(w, "| %-*s |\n", width, part)
		}
	}
	sep := func() { fmt.Fprintln(w, "+"+strings.Repeat("-", width+2)+"+") }
	field := func(label, v string) {
		if len([]rune(label))+len([]rune(v))+1 <= width {
			line(label + strings.Repeat(" ", width-len([]rune(label))-len([]rune(v))) + v)
		} else {
			line(label)
			for _, p := range wrap(v, width-2) {
				line("  " + p)
			}
		}
	}
	r, s := c.Receipt, c.Summary
	sep()
	line("               TOLLGLASS")
	line(s.Started)
	line("RECEIPT " + r.ID)
	sep()
	for _, p := range [][2]string{{"UPSTREAM", s.Upstream}, {"API", s.API}, {"MODEL ASKED", value(s.RequestedModel)}, {"MODEL REPORTED", value(s.ReportedModel)}, {"PROVIDER", value(s.Provider)}, {"TAG", value(s.Tag)}} {
		field(p[0], p[1])
	}
	sep()
	for _, p := range [][2]string{{"TOKENS IN", value(s.Input)}, {"TOKENS OUT", value(s.Output)}, {"CACHED", value(r.Usage.Cached)}, {"REASONING", value(r.Usage.Reasoning)}, {"FIRST BYTE", value(s.TTFB) + " ms"}, {"TOTAL TIME", fmt.Sprint(s.Total) + " ms"}, {"STATUS", value(s.Status)}} {
		field(p[0], p[1])
	}
	sep()
	field("TOTAL", value(s.Cost)+" USD")
	line(s.CostSource)
	var supp *receipt.Supplement
	if len(c.Supplements) > 0 {
		supp = &c.Supplements[len(c.Supplements)-1]
	}
	_, _, findings := receipt.Combined(&r, supp)
	for i, v := range findings {
		if i == 3 {
			line(fmt.Sprintf("+ %d more", len(findings)-3))
			break
		}
		line("! " + v.Text)
	}
	for _, s := range c.Supplements {
		sep()
		line("SUPPLEMENT " + s.Source)
		field("PROVIDER", value(s.Provider))
		field("REGION", value(s.DataRegion))
		field("COST", value(s.Cost))
		for i, a := range s.Attempts {
			line(fmt.Sprintf("Attempt %d: %s (%s)", i+1, value(a.Provider), value(a.Status)))
		}
	}
	sep()
	if color {
		fmt.Fprint(w, "\x1b[32m")
	}
	line("[ SIGNED " + r.Instance.KeyID + " ]")
	if color {
		fmt.Fprint(w, "\x1b[0m")
	}
	line("Model and token values are reported by the provider.")
	sep()
}
