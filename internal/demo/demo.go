// Package demo provides simulated API responses entirely in process.
package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/vincentsch/tollglass/internal/receipt"
)

const Provider = "Demo provider (simulated)"

// Transport never dials a network address, including for generation details.
type Transport struct {
	// Delays enables simulated response latency. The zero value delivers immediately.
	Delays bool
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "demo.invalid" {
		return nil, fmt.Errorf("demo only supports its in-process upstream")
	}
	defer req.Body.Close()
	var v struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.NewDecoder(req.Body).Decode(&v); err != nil {
		return nil, err
	}
	id := "demo-" + ulid.Make().String()
	model := v.Model
	status := 200
	contentType := "application/json"
	var body []byte
	encode := func(value any) []byte { b, _ := json.Marshal(value); return b }
	if model == "demo/error" {
		status = 429
		body = []byte(`{"error":{"type":"rate_limit_error","code":"rate_limit","message":"Simulated rate limit."}}`)
	} else if req.URL.Path == "/v1/embeddings" {
		body = encode(map[string]any{"object": "list", "model": model, "data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}}}, "usage": map[string]int{"prompt_tokens": 20, "total_tokens": 20}})
	} else if req.URL.Path == "/v1/messages" {
		usage := map[string]any{"input_tokens": 100, "output_tokens": 12, "cache_read_input_tokens": 50, "cache_creation_input_tokens": 25, "cache_creation": map[string]int{"ephemeral_5m_input_tokens": 15, "ephemeral_1h_input_tokens": 10}}
		message := map[string]any{"id": id, "type": "message", "role": "assistant", "model": model, "content": []any{map[string]string{"type": "text", "text": "A short demo answer (simulated)."}}, "stop_reason": "end_turn", "stop_sequence": nil, "usage": usage}
		body = encode(message)
		if v.Stream {
			contentType = "text/event-stream"
			message["content"] = []any{}
			message["stop_reason"] = nil
			var b bytes.Buffer
			for _, event := range []map[string]any{
				{"type": "message_start", "message": message},
				{"type": "content_block_start", "index": 0, "content_block": map[string]string{"type": "text", "text": ""}},
				{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "text_delta", "text": "A short demo answer (simulated)."}},
				{"type": "content_block_stop", "index": 0},
				{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 12}},
				{"type": "message_stop"},
			} {
				fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", event["type"], encode(event))
			}
			body = b.Bytes()
		}
	} else if req.URL.Path == "/v1/chat/completions" {
		if model == "demo/large" {
			model = "demo/small"
			id = "gen-demo-fallback-" + id
		}
		usage := map[string]any{"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120, "cost": json.Number("0.000240")}
		response := map[string]any{"id": id, "object": "chat.completion", "model": model, "provider": Provider, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "A short demo answer (simulated)."}, "finish_reason": "stop"}}, "usage": usage}
		if v.Model == "demo/no-usage" {
			delete(response, "usage")
		}
		body = encode(response)
		if v.Stream {
			contentType = "text/event-stream"
			response["object"] = "chat.completion.chunk"
			response["choices"] = []any{map[string]any{"index": 0, "delta": map[string]string{"role": "assistant", "content": "A short demo answer (simulated)."}, "finish_reason": "stop"}}
			body = append([]byte("data: "), encode(response)...)
			body = append(body, []byte("\n\ndata: [DONE]\n\n")...)
		}
	} else {
		status = 404
		body = []byte(`{"error":{"type":"not_found"}}`)
	}
	var responseBody io.ReadCloser = io.NopCloser(bytes.NewReader(body))
	if t.Delays {
		profile := delayProfile(req.URL.Path, v.Model, v.Stream)
		if err := wait(req.Context(), nil, profile.first); err != nil {
			return nil, err
		}
		chunks := [][]byte{body}
		if contentType == "text/event-stream" {
			chunks = bytes.SplitAfter(body, []byte("\n\n"))
			if len(chunks[len(chunks)-1]) == 0 {
				chunks = chunks[:len(chunks)-1]
			}
		}
		interval := time.Duration(0)
		if len(chunks) > 1 {
			interval = (profile.total - profile.first) / time.Duration(len(chunks)-1)
		}
		responseBody = &delayedBody{ctx: req.Context(), chunks: chunks, interval: interval, closed: make(chan struct{})}
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: responseBody, ContentLength: int64(len(body)), Request: req}, nil
}

type timingProfile struct{ first, total time.Duration }

func delayProfile(path, model string, stream bool) timingProfile {
	first, total := 40, 40
	if stream {
		total = 160
	}
	switch {
	case path == "/v1/chat/completions" && model == "demo/small" && stream:
		first, total = 300, 1200
	case path == "/v1/chat/completions" && model == "demo/small":
		first, total = 250, 250
	case path == "/v1/embeddings" && model == "demo/embedding" && !stream:
		first, total = 200, 200
	case path == "/v1/messages" && model == "demo/messages" && stream:
		first, total = 400, 1500
	case path == "/v1/messages" && model == "demo/messages":
		first, total = 450, 450
	case path == "/v1/chat/completions" && model == "demo/large" && !stream:
		first, total = 900, 900
	case path == "/v1/chat/completions" && model == "demo/error" && !stream:
		first, total = 200, 200
	case path == "/v1/chat/completions" && model == "demo/no-usage" && !stream:
		first, total = 350, 350
	}
	return timingProfile{time.Duration(first) * time.Millisecond, time.Duration(total) * time.Millisecond}
}

func wait(ctx context.Context, closed <-chan struct{}, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-closed:
		return io.ErrClosedPipe
	case <-timer.C:
		return ctx.Err()
	}
}

// Each read ends at an event boundary so the gateway can flush events as they arrive.
type delayedBody struct {
	ctx      context.Context
	chunks   [][]byte
	interval time.Duration
	pending  bool
	closed   chan struct{}
	once     sync.Once
}

func (b *delayedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	select {
	case <-b.closed:
		return 0, io.ErrClosedPipe
	default:
	}
	if len(b.chunks) == 0 {
		return 0, io.EOF
	}
	if b.pending {
		if err := wait(b.ctx, b.closed, b.interval); err != nil {
			return 0, err
		}
		b.pending = false
	}
	n := copy(p, b.chunks[0])
	b.chunks[0] = b.chunks[0][n:]
	if len(b.chunks[0]) == 0 {
		b.chunks = b.chunks[1:]
		b.pending = true
	}
	return n, nil
}
func (b *delayedBody) Close() error { b.once.Do(func() { close(b.closed) }); return nil }

func Lookup(_ context.Context, id, _ string) (*receipt.Supplement, error) {
	if !strings.HasPrefix(id, "gen-demo-fallback-") {
		return nil, nil
	}
	first := "Demo unavailable provider (simulated)"
	provider := Provider
	failed, ok := 503, 200
	prompt, completion := int64(102), int64(21)
	cost := "0.000123"
	region := "simulated"
	return &receipt.Supplement{Source: "demo.generation", Provider: &provider, Attempts: []receipt.Attempt{{Provider: &first, Status: &failed}, {Provider: &provider, Status: &ok}}, NativeTokens: receipt.NativeTokens{Prompt: &prompt, Completion: &completion}, Cost: &cost, DataRegion: &region, Findings: []receipt.Finding{{Code: "fallback_used", Level: "reported", Text: "The demo router tried 2 providers. The answer came from Demo provider (simulated)."}}}, nil
}

// Seed uses the public gateway paths, so demo receipts exercise real capture and signing.
func Seed(ctx context.Context, client *http.Client, base string) error {
	calls := []struct{ path, body, tag string }{
		{"chat/completions", `{"model":"demo/small","messages":[{"role":"user","content":"Hello"}]}`, "acme-shop"},
		{"chat/completions", `{"model":"demo/small","stream":true,"messages":[{"role":"user","content":"Hello"}]}`, "acme-shop"},
		{"embeddings", `{"model":"demo/embedding","input":"Hello"}`, "acme-shop"},
		{"messages", `{"model":"demo/messages","max_tokens":32,"messages":[{"role":"user","content":"Hello"}]}`, "acme-shop"},
		{"messages", `{"model":"demo/messages","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"Hello"}]}`, "blog-tool"},
		{"chat/completions", `{"model":"demo/large","messages":[{"role":"user","content":"Hello"}]}`, "blog-tool"},
		{"chat/completions", `{"model":"demo/error","messages":[{"role":"user","content":"Hello"}]}`, "blog-tool"},
		{"chat/completions", `{"model":"demo/no-usage","messages":[{"role":"user","content":"Hello"}]}`, "blog-tool"},
	}
	for _, call := range calls {
		req, err := http.NewRequestWithContext(ctx, "POST", base+"/demo/v1/"+call.path, strings.NewReader(call.body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tollglass-Tag", call.tag)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 && !(strings.Contains(call.body, "demo/error") && resp.StatusCode == 429) {
			return fmt.Errorf("demo call failed")
		}
	}
	return nil
}
