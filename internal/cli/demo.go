package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/rungrad/output"
	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/demo"
	"github.com/vincentsch/tollglass/internal/enrich"
	"github.com/vincentsch/tollglass/internal/proxy"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
)

func demoCommand(dataDir *string) *rungrad.Command {
	return &rungrad.Command{Use: "demo", Short: "Try eight simulated AI calls offline", Long: "Start an isolated demo on a free local port. Calls and provider details are simulated. Stop with Ctrl+C; temporary demo data is removed on exit.", Examples: []string{"tollglass demo", "tollglass demo --data-dir ./demo-data --json", "tollglass demo --dry-run"}, Related: []string{"tollglass serve"}, Args: cobra.NoArgs, Mutates: true, OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON}, Run: func(f *rungrad.Factory, cmd *cobra.Command, _ []string) error {
		if f.Flags.DryRun {
			return f.WritePreview(output.DryRunPreview{Method: "START", Path: "127.0.0.1:0", Body: []output.Field{{Name: "action", Value: "would start an isolated offline demo and send eight simulated calls"}}})
		}
		dir := *dataDir
		explicit := cmd.Flags().Changed("data-dir") || cmd.Root().PersistentFlags().Changed("data-dir")
		if !explicit {
			var err error
			dir, err = os.MkdirTemp("", "tollglass-demo-")
			if err != nil {
				return rungrad.NewError(2, "Cannot create the temporary demo directory.")
			}
			defer os.RemoveAll(dir)
		}
		return runDemo(f, dir)
	}}
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func runDemo(f *rungrad.Factory, dir string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return rungrad.NewError(2, "Cannot listen on a free demo port.")
	}
	defer ln.Close()
	db, err := store.Open(dir, 0, nil)
	if err != nil {
		return rungrad.NewError(2, err.Error())
	}
	defer db.Close()
	target, _ := url.Parse("http://demo.invalid")
	c := config.Default()
	c.RetentionDays = 0
	worker := &enrich.Worker{Lookup: demo.Lookup, Save: db.SaveSupplement}
	defer worker.Wait()
	gateway := &proxy.Gateway{Upstreams: map[string]upstream.Upstream{"demo": {Name: "demo", URL: target, API: "openai"}}, Config: c, KeyID: db.KeyID, Store: db, Transport: demo.Transport{}, Demo: true, Enrich: worker.Schedule}
	server := &http.Server{Handler: gateway, ReadHeaderTimeout: 10 * time.Second}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(ln) }()
	defer func() { server.Close(); <-finished; gateway.Wait() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	page := "http://" + ln.Addr().String()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	if err = demo.Seed(ctx, client, page); err != nil {
		return rungrad.NewError(2, "Cannot complete the demo calls.")
	}
	gateway.Wait()
	worker.Wait()
	if err = db.Flush(); err != nil {
		return rungrad.NewError(2, "Cannot store the demo receipts.")
	}
	// Keep the examples ready for the receipt commands as they become available.
	commands := map[string]string{
		"export":     "tollglass export --data-dir " + shellQuote(dir) + " --out " + shellQuote(filepath.Join(dir, "demo.tollglass.json")),
		"key_export": "tollglass key export --data-dir " + shellQuote(dir) + " --out " + shellQuote(filepath.Join(dir, "tollglass.pub")),
		"verify":     "tollglass verify " + shellQuote(filepath.Join(dir, "demo.tollglass.json")) + " --key " + shellQuote(filepath.Join(dir, "tollglass.pub")),
	}
	result := map[string]any{"page_url": page, "base_urls": map[string]string{"openai": page + "/demo/v1", "anthropic": page + "/demo"}, "data_dir": dir, "key_id": db.KeyID, "commands": commands}
	if err = f.WriteResult(result, func(w io.Writer) {
		fmt.Fprintf(w, "Tollglass demo is running. All calls and providers are simulated.\n\n  Page        %s\n  OpenAI      %s/demo/v1\n  Anthropic   %s/demo\n  Data dir    %s\n\n", page, page, page, dir)
		for _, name := range []string{"export", "key_export", "verify"} {
			fmt.Fprintln(w, commands[name])
		}
		fmt.Fprintln(w, "\nPress Ctrl+C to stop. The local page and receipt commands are coming in a later source build.")
	}); err != nil {
		return err
	}
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if server.Shutdown(shutdown) != nil {
		server.Close()
	}
	return nil
}
