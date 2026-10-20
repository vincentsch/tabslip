// Command gen refreshes the embedded snapshot from the reviewed source revision.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func generate() error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get("https://raw.githubusercontent.com/BerriAI/litellm/4818b767d8b4c4e5fa9885ab953d6f5cfe7c7ca5/model_prices_and_context_window.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("price source returned %d", resp.StatusCode)
	}
	var source map[string]map[string]any
	dec := json.NewDecoder(io.LimitReader(resp.Body, 20<<20))
	dec.UseNumber()
	if err := dec.Decode(&source); err != nil {
		return err
	}
	models := map[string]map[string]any{}
	for name, row := range source {
		mode, _ := row["mode"].(string)
		if mode != "chat" && mode != "completion" && mode != "responses" && mode != "embedding" {
			continue
		}
		trimmed := map[string]any{}
		for _, field := range []string{"litellm_provider", "input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost", "cache_creation_input_token_cost", "cache_creation_input_token_cost_above_1hr"} {
			if value, ok := row[field]; ok {
				trimmed[field] = value
			}
		}
		models[name] = trimmed
	}
	b, err := json.MarshalIndent(struct {
		Version string `json:"version"`
		Models  any    `json:"models"`
	}{"litellm-2026-10-20-4818b76", models}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("prices.json", append(b, '\n'), 0644)
}
