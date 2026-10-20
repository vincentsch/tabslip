package cli

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tollglass/internal/keys"
	"github.com/vincentsch/tollglass/internal/receipt"
	"github.com/vincentsch/tollglass/internal/statement"
	"github.com/vincentsch/tollglass/internal/store"
)

const noContent = "No prompt or answer text is included."

type exportSummary struct {
	Count     int           `json:"count"`
	Range     receipt.Range `json:"range"`
	Tags      []string      `json:"tags"`
	Models    []string      `json:"models"`
	Upstreams []string      `json:"upstreams"`
	Content   string        `json:"content"`
	Format    string        `json:"format"`
	Out       string        `json:"out"`
	DryRun    bool          `json:"dry_run"`
}

func summarize(calls []store.Call, filter store.Filter, format, out string, dry bool) exportSummary {
	s := exportSummary{Count: len(calls), Range: receipt.Range{From: filter.Since, To: filter.Until}, Content: noContent, Format: format, Out: out, DryRun: dry}
	tags, models, upstreams := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range calls {
		r := c.Summary
		if r.Tag != nil {
			tags[*r.Tag] = true
		}
		if r.RequestedModel != nil {
			models[*r.RequestedModel] = true
		}
		if r.ReportedModel != nil {
			models[*r.ReportedModel] = true
		}
		upstreams[r.Upstream] = true
	}
	sorted := func(m map[string]bool) []string {
		a := []string{}
		for k := range m {
			a = append(a, k)
		}
		sort.Strings(a)
		return a
	}
	s.Tags, s.Models, s.Upstreams = sorted(tags), sorted(models), sorted(upstreams)
	if len(calls) > 0 {
		if s.Range.From == "" {
			s.Range.From = calls[len(calls)-1].Summary.Started
		}
		if s.Range.To == "" {
			s.Range.To = calls[0].Summary.Started
		}
	}
	return s
}
func printSummary(w io.Writer, s exportSummary) {
	fmt.Fprintf(w, "%d receipts · %s → %s\nTags: %s\nModels: %s\nUpstreams: %s\n%s\n", s.Count, s.Range.From, s.Range.To, strings.Join(s.Tags, ", "), strings.Join(s.Models, ", "), strings.Join(s.Upstreams, ", "), s.Content)
}
func available(path string) error {
	if path == "" {
		return rungrad.NewError(1, "--out is required. Choose a new output file.")
	}
	_, err := os.Lstat(path)
	if err == nil {
		return rungrad.NewError(2, "Output file already exists. Choose another --out path.")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return rungrad.NewError(2, "Cannot inspect output path. Check its permissions.")
	}
	return nil
}

// Publish complete files without exposing partial output or overwriting existing paths.
func writeNew(path string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tollglass-export-*")
	if err != nil {
		return rungrad.NewError(2, "Cannot create output file. Check its directory and permissions.")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = write(f); err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Link(f.Name(), path)
	}
	if err != nil {
		return rungrad.NewError(2, "Cannot write output file. Check permissions, free space and whether it already exists.")
	}
	return nil
}
func exportCommand(dir *string) *rungrad.Command {
	var filter store.Filter
	var out, format string
	c := command("export", "Export content-free receipts and spending statements", func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		if err := available(out); err != nil {
			return err
		}
		switch format {
		case "bundle", "jsonl", "csv", "html":
		default:
			return rungrad.NewError(1, "--format must be bundle, jsonl, csv or html")
		}
		calls, err := readCalls(*dir, filter)
		if err != nil {
			return err
		}
		summary := summarize(calls, filter, format, out, f.Flags.DryRun)
		var public []byte
		var keyID string
		if format == "bundle" {
			b, k, e := keys.Public(*dir)
			if e != nil {
				return localError(e)
			}
			public, keyID = b, receipt.KeyID(k)
		}
		if f.Flags.DryRun {
			return f.WriteResult(summary, func(w io.Writer) {
				printSummary(w, summary)
				fmt.Fprintf(w, "Would write %s to %s. No changes were made.\n", format, out)
			})
		}
		if !f.Flags.JSON && f.Flags.JQ == "" && f.Flags.Template == "" && !f.Flags.Quiet {
			printSummary(f.Stdout, summary)
		}
		err = writeNew(out, func(w io.Writer) error { return writeExport(w, format, calls, summary, string(public), keyID) })
		if err != nil {
			return err
		}
		return f.WriteResult(summary, func(w io.Writer) { fmt.Fprintf(w, "Wrote %s to %s.\n", format, out) })
	})
	c.Mutates = true
	c.Configure = func(cmd *cobra.Command) {
		filterFlags(cmd, &filter, false)
		cmd.Flags().StringVar(&out, "out", "", "New output file (required)")
		cmd.Flags().StringVar(&format, "format", "bundle", "bundle, jsonl, csv or html")
	}
	c.Examples = []string{"tollglass export --out calls.tollglass.json", "tollglass export --format html --tag acme-shop --out statement.html --dry-run"}
	return c
}

var csvColumns = []string{"id", "started", "tag", "upstream", "api", "requested_model", "reported_model", "provider", "status", "input_tokens", "output_tokens", "cost", "cost_source", "ttfb_ms", "total_ms", "findings"}

func csvValue[T any](v *T) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(*v)
}
func writeExport(w io.Writer, format string, calls []store.Call, summary exportSummary, public, keyID string) error {
	switch format {
	case "bundle":
		b := receipt.Bundle{Type: "tollglass.bundle/v1", Created: receipt.Timestamp(time.Now()), PublicKey: public, KeyID: keyID, Range: summary.Range, Receipts: []json.RawMessage{}, Supplements: []json.RawMessage{}}
		for _, c := range calls {
			b.Receipts = append(b.Receipts, c.Envelope)
			b.Supplements = append(b.Supplements, c.SupplementEnvelopes...)
		}
		return receipt.WriteBundle(w, b)
	case "jsonl":
		for _, c := range calls { // Raw decoded payloads preserve all signed categories and decimal number text.
			var e receipt.Envelope
			if err := json.Unmarshal(c.Envelope, &e); err != nil {
				return err
			}
			p, err := payload(e)
			if err != nil {
				return err
			}
			supp := []json.RawMessage{}
			for _, raw := range c.SupplementEnvelopes {
				if err = json.Unmarshal(raw, &e); err != nil {
					return err
				}
				s, err := payload(e)
				if err != nil {
					return err
				}
				supp = append(supp, s)
			}
			if err = json.NewEncoder(w).Encode(struct {
				Receipt     json.RawMessage   `json:"receipt"`
				Supplements []json.RawMessage `json:"supplements"`
			}{p, supp}); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write(csvColumns); err != nil {
			return err
		}
		for _, c := range calls {
			s := c.Summary
			if err := cw.Write([]string{s.ID, s.Started, csvValue(s.Tag), s.Upstream, s.API, csvValue(s.RequestedModel), csvValue(s.ReportedModel), csvValue(s.Provider), csvValue(s.Status), csvValue(s.Input), csvValue(s.Output), csvValue(s.Cost), s.CostSource, csvValue(s.TTFB), fmt.Sprint(s.Total), strings.Join(s.Findings, ";")}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case "html":
		return statement.Write(w, calls, summary.Range.From, summary.Range.To)
	}
	return fmt.Errorf("unknown export format")
}
