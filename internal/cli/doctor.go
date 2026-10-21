package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tabslip/internal/config"
	"github.com/vincentsch/tabslip/internal/keys"
	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/store"
	"github.com/vincentsch/tabslip/internal/upstream"
)

type check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

var doctorTransport = func() *http.Transport { return &http.Transport{Proxy: nil} }

func doctorCommand(dir, listen *string, port *int) *rungrad.Command {
	c := command("doctor", "Check local setup and upstream reachability", func(f *rungrad.Factory, cmd *cobra.Command, _ []string) error {
		cfg, missing, err := loadSettings(f, cmd)
		if cmd.Flags().Changed("port") {
			cfg.Port = *port
		}
		if cmd.Flags().Changed("listen") {
			cfg.Listen = *listen
		}
		checks := []check{}
		if err == nil {
			err = config.Validate(cfg)
		}
		if err != nil {
			checks = append(checks, check{"config", "fail", err.Error(), "Correct the config YAML or select another --config file."})
		} else if missing != "" {
			checks = append(checks, check{"config", "warn", missing, missingConfigFix})
		} else {
			checks = append(checks, check{"config", "ok", "Config is valid.", ""})
		}
		checks = append(checks, localChecks(*dir)...)
		if err == nil {
			checks = append(checks, portChecks(cfg)...)
			transport := doctorTransport()
			defer transport.CloseIdleConnections()
			checks = append(checks, reachability(cfg, transport)...)
		}
		failed := false
		for _, c := range checks {
			failed = failed || c.Status == "fail"
		}
		err = f.WriteResult(checks, func(w io.Writer) {
			for _, c := range checks {
				fmt.Fprintf(w, "[%s] %s: %s\n", c.Status, c.Name, c.Message)
				if c.Fix != "" {
					fmt.Fprintf(w, "  Fix: %s\n", c.Fix)
				}
			}
		})
		if err != nil {
			return err
		}
		if failed {
			return verdictError(2)
		}
		return nil
	})
	c.Examples = []string{"tabslip doctor", "tabslip doctor --port 4546 --json"}
	return c
}
func writable(dir string) error {
	p := dir
	for {
		st, err := os.Stat(p)
		if err == nil {
			if !st.IsDir() {
				return fmt.Errorf("data location is not a directory")
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(p)
		if next == p {
			return err
		}
		p = next
	}
	f, err := os.CreateTemp(p, ".tabslip-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	err = f.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return err
	}
	return removeErr
}
func localChecks(dir string) []check {
	out := []check{}
	if err := writable(dir); err != nil {
		out = append(out, check{"data directory", "fail", "Data location is not writable.", "Choose a writable --data-dir or correct its permissions."})
	} else {
		out = append(out, check{"data directory", "ok", "Data location is writable.", ""})
	}
	_, pub, err := keys.Public(dir)
	db, dbErr := store.OpenReader(dir)
	if dbErr == nil {
		defer db.Close()
	}
	if os.IsNotExist(err) && os.IsNotExist(dbErr) { // An interrupted first start may have a private key but no public key.
		if _, e := os.Stat(filepath.Join(dir, "key.ed25519")); os.IsNotExist(e) {
			return append(out, check{"identity", "info", "Key and database are not created yet. Start Tabslip when ready.", ""})
		}
	}
	if err != nil {
		out = append(out, check{"identity", "fail", "Public key is missing or invalid.", "Restore the original key pair, or start with a new --data-dir."})
		return out
	}
	if err = keys.CheckPrivate(dir, pub); err != nil {
		out = append(out, check{"identity", "fail", err.Error(), "Restore the original key pair, or start with a new --data-dir."})
		return out
	}
	if os.IsNotExist(dbErr) {
		return append(out, check{"identity", "info", "Existing key pair is valid; the database is not created yet.", ""})
	}
	if dbErr != nil {
		return append(out, check{"identity", "fail", "Cannot read the database.", "Restore the database or use a new --data-dir."})
	}
	id, err := db.KeyID()
	if err != nil || id != receipt.KeyID(pub) {
		out = append(out, check{"identity", "fail", "Database and instance key do not match.", "Restore the original key pair or use a new --data-dir."})
	} else {
		out = append(out, check{"identity", "ok", "Database and key pair match: " + id, ""})
	}
	return out
}
func portChecks(cfg config.Config) []check {
	address := net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", address)
	if err == nil {
		ln.Close()
		return []check{{"port", "ok", fmt.Sprintf("Port %d is free.", cfg.Port), ""}}
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, e := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port)) + "/healthz")
	if e == nil {
		defer resp.Body.Close()
		var h struct {
			Tool, Version string
			KeyID         string `json:"key_id"`
			Unrecorded    int64  `json:"unrecorded_calls"`
		}
		if resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h) == nil && h.Tool == "tabslip" {
			out := []check{{"port", "ok", fmt.Sprintf("Tabslip %s is running on port %d with key %s.", h.Version, cfg.Port, h.KeyID), ""}}
			if h.Unrecorded > 0 {
				out = append(out, check{"recording", "warn", fmt.Sprintf("%d calls could not be recorded.", h.Unrecorded), "Check disk space and server diagnostics before sending more calls."})
			}
			return out
		}
	}
	return []check{{"port", "fail", fmt.Sprintf("Cannot use port %d; another program may be listening.", cfg.Port), fmt.Sprintf("Stop the other program or choose --port %d. Check --listen as well.", nextPort(cfg.Port))}}
}
func reachability(cfg config.Config, transport http.RoundTripper) []check {
	us, _ := upstream.Build(cfg.Upstreams)
	custom := map[string]bool{}
	for _, u := range cfg.Upstreams {
		custom[u.Name] = true
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	out := []check{}
	for _, name := range upstream.Names(us) {
		u := us[name]
		resp, err := client.Get(u.URL.String())
		if err != nil {
			status := "warn"
			if custom[name] {
				status = "fail"
			}
			out = append(out, check{"upstream " + name, status, "No HTTP answer within the reachability check.", "Check the upstream URL and network connection; for Ollama, start its local server."})
		} else {
			resp.Body.Close()
			out = append(out, check{"upstream " + name, "ok", fmt.Sprintf("HTTP %d received. Credentials were not checked.", resp.StatusCode), ""})
		}
	}
	return out
}
