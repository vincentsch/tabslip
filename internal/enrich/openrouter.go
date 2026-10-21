// Package enrich fetches optional provider details after the client response.
package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/vincentsch/tabslip/internal/prices"
	"github.com/vincentsch/tabslip/internal/receipt"
)

type Lookup func(context.Context, string, string) (*receipt.Supplement, error)
type Worker struct {
	Lookup Lookup
	Save   func(*receipt.Supplement)
	tasks  sync.WaitGroup
}

func (w *Worker) Schedule(r *receipt.Receipt, authorization string) {
	if r.Response.ID == nil {
		return
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return
	}
	id, responseID, digest := r.ID, *r.Response.ID, receipt.Digest(payload)
	w.tasks.Add(1)
	go func() {
		defer w.tasks.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s, err := w.Lookup(ctx, responseID, authorization)
		if err != nil || s == nil {
			return
		}
		s.Type, s.ReceiptID, s.ReceiptSHA256 = "tabslip.supplement/v1", id, digest
		s.Fetched, s.Currency = receipt.Timestamp(time.Now()), "USD"
		w.Save(s)
	}()
}
func (w *Worker) Wait() { w.tasks.Wait() }

// OpenRouter retries only the generation lookup, never the original API call.
func OpenRouter(transport http.RoundTripper) Lookup {
	return func(ctx context.Context, id, authorization string) (*receipt.Supplement, error) {
		client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
			}
			req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/generation?id="+url.QueryEscape(id), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", authorization)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			b, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == 200 && len(b) <= 1<<20 {
				if s, err := Parse(b); err == nil {
					return s, nil
				}
			}
		}
		return nil, fmt.Errorf("generation details unavailable")
	}
}
func Parse(b []byte) (*receipt.Supplement, error) {
	var v struct {
		Data *struct {
			Cost     json.Number `json:"total_cost"`
			Provider *string     `json:"provider_name"`
			Attempts []struct {
				Provider *string `json:"provider_name"`
				Status   *int    `json:"status"`
			} `json:"provider_responses"`
			Prompt     *int64  `json:"native_tokens_prompt"`
			Completion *int64  `json:"native_tokens_completion"`
			Region     *string `json:"data_region"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	if v.Data == nil {
		return nil, fmt.Errorf("missing generation data")
	}
	d := v.Data
	s := &receipt.Supplement{Source: "openrouter.generation", Provider: d.Provider, Attempts: []receipt.Attempt{}, NativeTokens: receipt.NativeTokens{Prompt: d.Prompt, Completion: d.Completion}, Cost: prices.Decimal(d.Cost), DataRegion: d.Region, Findings: []receipt.Finding{}}
	for _, a := range d.Attempts {
		s.Attempts = append(s.Attempts, receipt.Attempt{Provider: a.Provider, Status: a.Status})
	}
	if len(s.Attempts) > 1 {
		provider := "an unknown provider"
		if s.Provider != nil {
			provider = *s.Provider
		}
		s.Findings = append(s.Findings, receipt.Finding{Code: "fallback_used", Level: "reported", Text: fmt.Sprintf("OpenRouter tried %d providers. The answer came from %s.", len(s.Attempts), provider)})
	}
	return s, nil
}
