package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	rgconfig "github.com/vincentsch/rungrad/config"
	"github.com/vincentsch/rungrad/output"
	"github.com/vincentsch/tollglass/internal/config"
	"github.com/vincentsch/tollglass/internal/proxy"
	"github.com/vincentsch/tollglass/internal/store"
	"github.com/vincentsch/tollglass/internal/upstream"
	"github.com/vincentsch/tollglass/internal/version"
)

func NewApp() *rungrad.App {
	app := rungrad.New(rungrad.AppConfig{
		Name:    "tollglass",
		Short:   "Keep signed receipts for AI API calls",
		Long:    "Forward AI API calls and keep receipts signed by your Tollglass. Model names and usage come from the provider.",
		Version: version.String(),
		Resolution: &rungrad.ResolutionConfig{
			ConfigEnvVar: "TOLLGLASS_CONFIG",
			// rungrad resolves the path; Tollglass owns the YAML settings.
			LoadConfig: func(string) (rgconfig.Config, error) { return rgconfig.Config{}, nil },
		},
	})
	var dataDir, listen string
	var port int
	var keep bool
	root := app.Root()
	root.PersistentFlags().StringVar(&dataDir, "data-dir", config.DataDir(), "Directory for local receipts and the instance key")
	root.PersistentFlags().StringVar(&listen, "listen", "127.0.0.1", "Address to listen on")
	root.PersistentFlags().IntVar(&port, "port", 4545, "Port to listen on")
	root.PersistentFlags().BoolVar(&keep, "keep-content", false, "Keep request and answer content locally")
	command := &rungrad.Command{
		Use:         "serve",
		Short:       "Start the local AI gateway",
		Long:        "Forward API calls and store signed OpenAI chat receipts. Stop with Ctrl+C or SIGTERM. Content is off by default.",
		Examples:    []string{"tollglass serve", "tollglass serve --port 4546", "tollglass serve --dry-run --json"},
		Related:     []string{"tollglass --help"},
		Args:        cobra.NoArgs,
		Mutates:     true,
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			c, err := config.Load(f.ConfigPath())
			if err != nil {
				return rungrad.NewError(1, err.Error())
			}
			if cmd.Flags().Changed("listen") || root.PersistentFlags().Changed("listen") {
				c.Listen = listen
			}
			if cmd.Flags().Changed("port") || root.PersistentFlags().Changed("port") {
				c.Port = port
			}
			if cmd.Flags().Changed("keep-content") || root.PersistentFlags().Changed("keep-content") {
				c.KeepContent = keep
			}
			if err = config.Validate(c); err != nil {
				return rungrad.NewError(1, err.Error())
			}
			if f.Flags.DryRun {
				return f.WritePreview(output.DryRunPreview{Method: "START", Path: net.JoinHostPort(c.Listen, strconv.Itoa(c.Port)), Body: []output.Field{{Name: "action", Value: "would create the instance key and database, then start forwarding calls"}, {Name: "content", Value: strconv.FormatBool(c.KeepContent)}}})
			}
			return serve(f, c, dataDir)
		}}
	app.AddCommand(command)
	for _, cmd := range root.Commands() {
		if cmd.Name() == "serve" {
			root.RunE = cmd.RunE
			root.Args = cobra.NoArgs
			root.Annotations = cmd.Annotations
			root.Example = "  tollglass\n  tollglass --port 4546\n  tollglass --dry-run --json"
		}
	}
	return app
}
func serve(f *rungrad.Factory, c config.Config, dataDir string) error {
	address := net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
	ln, err := net.Listen("tcp", address)
	if err != nil {
		if isAddressInUse(err) {
			return rungrad.NewError(2, busyMessage(c.Port))
		}
		return rungrad.NewError(2, "Cannot listen on the selected address. Check --listen and --port.")
	}
	defer ln.Close()
	logger := log.New(f.Stderr, "", 0)
	db, err := store.Open(dataDir, c.RetentionDays, logger)
	if err != nil {
		return rungrad.NewError(2, err.Error())
	}
	defer db.Close()
	us, _ := upstream.Build(c.Upstreams)
	transport := proxy.Transport()
	defer transport.CloseIdleConnections()
	gateway := &proxy.Gateway{Upstreams: us, Config: c, KeyID: db.KeyID, Store: db, Transport: transport, Logger: logger}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Closing active sockets after the grace period still lets deferred capture finish.
	activeCtx, cancelActive := context.WithCancel(context.Background())
	defer cancelActive()
	server := &http.Server{Handler: gateway, ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return activeCtx }, ErrorLog: log.New(io.Discard, "", 0)}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(ln) }()
	page := "http://" + net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
	urls := map[string]string{}
	for _, name := range upstream.Names(us) {
		u := page + "/" + name
		if us[name].API == "openai" {
			u += "/v1"
		}
		urls[name] = u
	}
	if err = f.WriteResult(map[string]any{"page_url": page, "base_urls": urls, "key_id": db.KeyID, "data_dir": dataDir}, func(w io.Writer) {
		fmt.Fprint(w, "Tollglass is running.\n\n")
		fmt.Fprintf(w, "  Page        %s\n", page)
		labels := map[string]string{"openai": "OpenAI", "anthropic": "Anthropic", "openrouter": "OpenRouter", "ollama": "Ollama"}
		for _, n := range []string{"openai", "anthropic", "openrouter", "ollama"} {
			fmt.Fprintf(w, "  %-11s %s\n", labels[n], urls[n])
		}
		for _, n := range upstream.Names(us) {
			if _, ok := labels[n]; !ok {
				fmt.Fprintf(w, "  %-11s %s\n", n, urls[n])
			}
		}
		fmt.Fprintln(w)
		if c.KeepContent {
			fmt.Fprintln(w, "Content is stored locally for all calls.")
		} else if len(c.KeepContentTags) > 0 {
			fmt.Fprintln(w, "Content is stored locally for selected tags.")
		} else {
			fmt.Fprintln(w, "Content is not stored (hashes only).")
		}
		fmt.Fprintln(w, "Press Ctrl+C to stop.")
	}); err != nil {
		cancelActive()
		server.Close()
		<-finished
		return err
	}
	select {
	case err = <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return rungrad.NewError(2, "The gateway stopped unexpectedly.")
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		cancelActive()
		server.Close()
	}
	<-finished
	// Shutdown waits for handlers on a normal stop. After a forced close, wait for finalizers too.
	gateway.Wait()
	return nil
}

func isAddressInUse(err error) bool {
	// Winsock's WSAEADDRINUSE does not match Go's synthetic EADDRINUSE.
	return errors.Is(err, syscall.EADDRINUSE) ||
		(runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10048)))
}
func busyMessage(port int) string {
	endpoint := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Get(endpoint + "/healthz")
	if err == nil {
		defer resp.Body.Close()
		var h struct {
			Tool string `json:"tool"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h) == nil && h.Tool == "tollglass" {
			return "Tollglass is already running at " + endpoint + "."
		}
	}
	return fmt.Sprintf("Port %d is in use by another program. Start Tollglass with `--port %d`, or stop the other program.", port, nextPort(port))
}

func nextPort(port int) int {
	if port == 65535 {
		return 4546
	}
	return port + 1
}
