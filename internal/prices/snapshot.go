package prices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vincentsch/tollglass/internal/receipt"
)

const SourceURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

type Info struct {
	Source  string `json:"source"`
	Version string `json:"version"`
	Date    string `json:"date"`
	SHA256  string `json:"sha256"`
}

func describe(t Table, b []byte, source string) Info {
	date := t.Version
	if len(date) >= 19 && date[:8] == "litellm-" {
		date = date[8:18]
	}
	return Info{source, t.Version, date, receipt.Digest(b)}
}
func Selected(dir string) (Table, Info, error) {
	b, err := os.ReadFile(filepath.Join(dir, "prices.json"))
	if os.IsNotExist(err) {
		return Embedded, describe(Embedded, snapshot, "embedded"), nil
	}
	if err != nil {
		return Table{}, Info{}, fmt.Errorf("cannot read price snapshot")
	}
	var t Table
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err = d.Decode(&t); err != nil || len(t.Models) == 0 || t.Version == "" {
		return Table{}, Info{}, fmt.Errorf("invalid price snapshot; restore prices.json or remove it to use embedded prices")
	}
	for _, r := range t.Models {
		if err = validRate(r); err != nil {
			return Table{}, Info{}, err
		}
	}
	return t, describe(t, b, "downloaded"), nil
}
func validRate(r Rate) error {
	for _, n := range []json.Number{r.Input, r.Output, r.Read, r.Write, r.Write1h} {
		if n != "" && Decimal(n) == nil {
			return fmt.Errorf("price table contains an invalid rate")
		}
	}
	return nil
}

// Download trims the upstream document while preserving decimal number text.
func Download(client *http.Client, url string, now time.Time) ([]byte, string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("cannot download prices; check your connection and try again")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("price source returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (20<<20)+1))
	if err != nil || len(raw) > 20<<20 {
		return nil, "", fmt.Errorf("cannot read price source or source is too large")
	}
	var source map[string]map[string]json.RawMessage
	if err = json.Unmarshal(raw, &source); err != nil {
		return nil, "", fmt.Errorf("invalid price source JSON")
	}
	t := Table{Version: "litellm-" + now.UTC().Format("2006-01-02T150405.000000000Z") + "-" + receipt.Digest(raw)[:12], Models: map[string]Rate{}}
	for name, row := range source {
		var mode string
		json.Unmarshal(row["mode"], &mode)
		if mode != "chat" && mode != "completion" && mode != "responses" && mode != "embedding" {
			continue
		}
		b, _ := json.Marshal(row)
		var rate Rate
		if err = json.Unmarshal(b, &rate); err != nil {
			return nil, "", fmt.Errorf("invalid price row")
		}
		if err = validRate(rate); err != nil {
			return nil, "", err
		}
		if rate.Provider == "" || rate.Input == "" {
			continue
		}
		t.Models[name] = rate
	}
	if len(t.Models) == 0 {
		return nil, "", fmt.Errorf("price source has no usable models")
	}
	b, err := json.MarshalIndent(t, "", "  ")
	return append(b, '\n'), t.Version, err
}

// Install publishes a named immutable snapshot before switching the startup table.
func Install(dir string, b []byte, name string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create price directory")
	}
	f, err := os.CreateTemp(dir, ".prices-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		return err
	}
	if err = os.Link(f.Name(), filepath.Join(dir, name+".json")); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "prices.json"))
}
