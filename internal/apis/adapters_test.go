package apis

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tollglass/internal/receipt"
)

func parseFixture(t *testing.T, api, name string) (*receipt.Receipt, *Capture) {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	r := receipt.New("tg_test", time.Now())
	r.Request.API = api
	c := &Capture{Receipt: r, Keep: true}
	c.Parse(strings.NewReader(string(b)), strings.HasSuffix(name, "sse"))
	return r, c
}

func TestAdapterFixtureMappings(t *testing.T) {
	for _, name := range []string{"messages.json", "messages.sse", "messages-error.sse"} {
		t.Run(name, func(t *testing.T) {
			r, c := parseFixture(t, "anthropic.messages", name)
			expected := `{"source":"reported","input_tokens":100,"output_tokens":12,"cached_input_tokens":50,"cache_write_tokens":25,"cache_write_1h_tokens":10,"reasoning_tokens":null}`
			if name == "messages-error.sse" {
				expected = strings.Replace(expected, `"output_tokens":12`, `"output_tokens":1`, 1)
			}
			b, _ := json.Marshal(r.Usage)
			if string(b) != expected {
				t.Fatalf("usage %s", b)
			}
			if *r.Response.ID != "msg_example" || *r.Response.Model != "claude-sonnet-4-6" || *r.DisplayInput() != 175 {
				t.Fatal("message mapping")
			}
			if strings.HasSuffix(name, "sse") && !c.Done {
				t.Fatal("terminal event missing")
			}
			if name == "messages-error.sse" && (r.Response.Error == nil || r.Response.Error.Type == nil || *r.Response.Error.Type != "overloaded_error" || r.Response.Error.Code != nil || !has(r, "upstream_error")) {
				t.Fatal("terminal message error fields")
			}
			if name == "messages.sse" && (string(c.Text) != "Grüße" || *r.Response.FinishReason != "end_turn") {
				t.Fatal("stream content/stop reason")
			}
		})
	}
	for _, name := range []string{"responses.json", "responses-completed.sse", "responses-incomplete.sse", "responses-failed.sse", "responses-error.sse", "responses-nested-error.sse"} {
		t.Run(name, func(t *testing.T) {
			r, c := parseFixture(t, "openai.responses", name)
			b, _ := json.Marshal(r.Usage)
			expected := `{"source":"reported","input_tokens":23,"output_tokens":8,"cached_input_tokens":5,"cache_write_tokens":null,"cache_write_1h_tokens":null,"reasoning_tokens":3}`
			if name == "responses-error.sse" || name == "responses-nested-error.sse" {
				expected = `{"source":"unknown","input_tokens":null,"output_tokens":null,"cached_input_tokens":null,"cache_write_tokens":null,"cache_write_1h_tokens":null,"reasoning_tokens":null}`
				if r.Response.APIStatus == nil || *r.Response.APIStatus != "in_progress" {
					t.Fatal("created status")
				}
			}
			if string(b) != expected {
				t.Fatalf("usage %s", b)
			}
			if *r.Response.ID != "resp_example" || *r.Response.Model != "gpt-4.1-2025-04-14" {
				t.Fatal("response mapping")
			}
			if strings.HasSuffix(name, "sse") && !c.Done {
				t.Fatal("terminal event missing")
			}
			if name == "responses-failed.sse" && (r.Response.Error.Type != nil || r.Response.Error.Code != "server_error" || !has(r, "upstream_error") || *r.Response.APIStatus != "failed") {
				t.Fatal("nested error mapping")
			}
			if name == "responses-error.sse" && (r.Response.Error == nil || r.Response.Error.Type != nil || r.Response.Error.Code != json.Number("503")) {
				t.Fatal("top-level error mapping")
			}
			if name == "responses-nested-error.sse" && (r.Response.Error == nil || r.Response.Error.Type == nil || *r.Response.Error.Type != "invalid_request_error" || r.Response.Error.Code != "rate_limit_exceeded" || !has(r, "upstream_error")) {
				t.Fatal("nested standalone error mapping")
			}

			if name == "responses-incomplete.sse" && *r.Response.APIStatus != "incomplete" {
				t.Fatal("incomplete status")
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), "sensitive") {
				t.Fatal("error message leaked")
			}
		})
	}
	r, _ := parseFixture(t, "openai.embeddings", "embeddings.json")
	b, _ := json.Marshal(r.Usage)
	if string(b) != `{"source":"reported","input_tokens":8,"output_tokens":null,"cached_input_tokens":null,"cache_write_tokens":null,"cache_write_1h_tokens":null,"reasoning_tokens":null}` || *r.Response.Model != "text-embedding-3-small" {
		t.Fatal("embedding mapping", string(b))
	}
}

func TestCumulativeUsagePreservesKnownFields(t *testing.T) {
	for _, api := range []string{"anthropic.messages", "openai.responses"} {
		t.Run(api, func(t *testing.T) {
			r := receipt.New("test", time.Now())
			r.Request.API = api
			c := &Capture{Receipt: r}
			values := []string{`{"usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":50,"cache_creation_input_tokens":25,"cache_creation":{"ephemeral_1h_input_tokens":10}}}`, `{"usage":{"input_tokens":null,"output_tokens":4,"cache_read_input_tokens":null,"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":null}}}`, `{"usage":{"output_tokens":null}}`}
			for _, v := range values {
				c.value([]byte(v), false)
			}
			if *r.Usage.Input != 100 || *r.Usage.Output != 4 {
				t.Fatal("cumulative usage lost or added")
			}
			if api == "anthropic.messages" && (*r.Usage.Cached != 50 || *r.Usage.CacheWrite != 25 || *r.Usage.CacheWrite1h != 10) {
				t.Fatal("cache usage lost")
			}
		})
	}
}

func TestMissingUsageAndTerminalFraming(t *testing.T) {
	for _, api := range []string{"anthropic.messages", "openai.responses", "openai.embeddings"} {
		r := receipt.New("test", time.Now())
		r.Request.API = api
		c := &Capture{Receipt: r}
		c.Parse(strings.NewReader(`{"model":"example"}`), false)
		Finish(r)
		if !has(r, "usage_missing") || r.Usage.Input != nil || r.Usage.Output != nil {
			t.Fatal("missing usage")
		}
	}
	for _, tc := range []struct{ api, body string }{
		{"anthropic.messages", `{"type":"error","error":{"type":"overloaded_error","message":"private"}}`},
		{"openai.responses", `{"type":"error","code":"server_error","message":"private"}`},
	} {
		for _, suffix := range []string{"", "\n", "\n\n"} {
			r := receipt.New("test", time.Now())
			r.Request.API = tc.api
			c := &Capture{Receipt: r}
			c.Parse(strings.NewReader("data: "+tc.body+suffix), true)
			if c.Done != (suffix == "\n\n") {
				t.Fatal("cutoff treated as terminal", tc.api, suffix)
			}
		}
	}
}

func TestResponsesNestedErrorCodeTypes(t *testing.T) {
	for _, code := range []string{`"server_error"`, `503`} {
		for _, stream := range []bool{false, true} {
			r := receipt.New("test", time.Now())
			r.Request.API = "openai.responses"
			c := &Capture{Receipt: r}
			body := `{"id":"resp_error","status":"failed","error":{"code":` + code + `,"message":"sensitive prompt"}}`
			if stream {
				body = "data: {\"type\":\"response.failed\",\"response\":" + body + "}\n\n"
			}
			c.Parse(strings.NewReader(body), stream)
			if r.Response.Error == nil || r.Response.Error.Type != nil || !has(r, "upstream_error") {
				t.Fatal("nested error lost")
			}
			b, _ := json.Marshal(r.Response.Error)
			if string(b) != `{"type":null,"code":`+code+`}` {
				t.Fatal("error field types", string(b))
			}
		}
	}
}

func TestCumulativeStreamUsageAndContent(t *testing.T) {
	for _, api := range []string{"anthropic.messages", "openai.responses"} {
		r := receipt.New("test", time.Now())
		r.Request.API = api
		c := &Capture{Receipt: r, Keep: true}
		values := []string{`{"usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":50,"cache_creation_input_tokens":25,"cache_creation":{"ephemeral_1h_input_tokens":10},"input_tokens_details":{"cached_tokens":50},"output_tokens_details":{"reasoning_tokens":2}}}`, `{"usage":{"input_tokens":null,"output_tokens":4,"cache_read_input_tokens":null,"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":null},"input_tokens_details":{"cached_tokens":null},"output_tokens_details":{"reasoning_tokens":null}}}`, `{"usage":{"output_tokens":null}}`}
		var stream string
		for i, v := range values {
			if api == "anthropic.messages" {
				if i == 0 {
					v = `{"type":"message_start","message":` + v + `}`
				} else {
					v = strings.Replace(v, `{"usage"`, `{"type":"message_delta","usage"`, 1)
				}
			} else {
				v = `{"type":"response.in_progress","response":` + v + `}`
			}
			stream += "data: " + v + "\n\n"
		}
		c.Parse(strings.NewReader(stream), true)
		if *r.Usage.Input != 100 || *r.Usage.Output != 4 || *r.Usage.Cached != 50 {
			t.Fatal("null replaced cumulative stream usage")
		}
		if api == "anthropic.messages" && (*r.Usage.CacheWrite != 25 || *r.Usage.CacheWrite1h != 10) {
			t.Fatal("cache write lost")
		}
		if api == "openai.responses" && *r.Usage.Reasoning != 2 {
			t.Fatal("reasoning lost")
		}
		// Content capture can continue after metadata parsing has stopped.
		delta := `{"type":"response.output_text.delta","delta":"later text"}`
		if api == "anthropic.messages" {
			delta = `{"type":"content_block_delta","delta":{"type":"text_delta","text":"later text"}}`
		}
		c.Parse(strings.NewReader("data: invalid\n\ndata: "+delta+"\n\n"), true)
		if string(c.Text) != "later text" || !c.Truncated || !has(r, "metadata_unavailable") {
			t.Fatal("local content after metadata failure")
		}
	}
}

func TestReasoningEffortUsesEndpointPrecedence(t *testing.T) {
	for _, api := range []string{"openai.chat", "openai.responses"} {
		for _, tc := range []struct{ body, want string }{
			{`{"reasoning_effort":"low","reasoning":{"effort":"high"}}`, map[bool]string{true: "high", false: "low"}[api == "openai.responses"]},
			{`{"reasoning_effort":"low"}`, "low"},
			{`{"reasoning":{"effort":"high"}}`, "high"},
		} {
			r := receipt.New("test", time.Now())
			r.Request.API = api
			if _, ok := Request(r, []byte(tc.body)); !ok || r.Request.Params["reasoning_effort"] != tc.want {
				t.Fatalf("%s: got %v, want %s", api, r.Request.Params["reasoning_effort"], tc.want)
			}
		}
	}
}

func TestStreamSentinelsDoNotReplaceProviderTerminalEvents(t *testing.T) {
	for _, tc := range []struct{ api, file string }{
		{"openai.responses", "responses-completed.sse"},
		{"openai.responses", "responses-incomplete.sse"},
		{"openai.responses", "responses-failed.sse"},
		{"anthropic.messages", "messages.sse"},
	} {
		raw, err := os.ReadFile("../../testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		r := receipt.New("test", time.Now())
		r.Request.API = tc.api
		c := &Capture{Receipt: r}
		c.Parse(strings.NewReader(string(raw)+"data: [DONE]\n\n"), true)
		if !c.Done || has(r, "metadata_unavailable") {
			t.Fatal("trailing sentinel corrupted metadata", tc.file)
		}
	}
	for _, api := range []string{"openai.chat", "openai.responses", "anthropic.messages"} {
		r := receipt.New("test", time.Now())
		r.Request.API = api
		c := &Capture{Receipt: r}
		c.Parse(strings.NewReader("data: [DONE]\n\n"), true)
		if c.Done != (api == "openai.chat") || has(r, "metadata_unavailable") {
			t.Fatal("sentinel changed completion contract", api)
		}
	}
}

func TestTerminalEventsRemainObservableAfterMetadataFailure(t *testing.T) {
	for _, tc := range []struct{ api, terminal string }{
		{"openai.responses", `{"type":"response.completed","response":{"model":"changed","usage":{"input_tokens":100}}}`},
		{"openai.responses", `{"type":"response.incomplete"}`},
		{"openai.responses", `{"type":"response.failed"}`},
		{"openai.responses", `{"type":"error","code":503,"message":"sensitive"}`},
		{"anthropic.messages", `{"type":"message_stop"}`},
		{"anthropic.messages", `{"type":"error","error":{"type":"overloaded_error","message":"sensitive"}}`},
	} {
		for _, failure := range []string{"not-json", strings.Repeat("x", Limit+1)} {
			for _, suffix := range []string{"\n\n", ""} {
				r := receipt.New("test", time.Now())
				r.Request.API = tc.api
				c := &Capture{Receipt: r}
				c.value([]byte(`{"model":"known","usage":{"input_tokens":3}}`), false)
				c.Parse(strings.NewReader("data: "+failure+"\n\ndata: "+tc.terminal+suffix), true)
				if c.Done != (suffix != "") || !has(r, "metadata_unavailable") || r.Response.Model == nil || *r.Response.Model != "known" || *r.Usage.Input != 3 {
					t.Fatal("terminal framing or frozen metadata lost", tc.api)
				}
			}
		}
	}
}

func TestStandaloneResponsesErrorsPreservePriorUsage(t *testing.T) {
	for _, errorBody := range []string{
		`{"type":"error","code":503,"message":"sensitive"}`,
		`{"type":"error","error":{"type":"invalid_request_error","code":"rate_limit_exceeded","message":"sensitive"}}`,
		`{"type":"error","error":{"type":"invalid_request_error","code":429,"message":"sensitive"}}`,
	} {
		r := receipt.New("test", time.Now())
		r.Request.API = "openai.responses"
		c := &Capture{Receipt: r}
		c.Parse(strings.NewReader("data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":23,\"output_tokens\":8}}}\n\ndata: "+errorBody+"\n\n"), true)
		if !c.Done || !has(r, "upstream_error") || *r.Usage.Input != 23 || *r.Usage.Output != 8 {
			t.Fatal("prior usage lost")
		}
		expected := `{"type":null,"code":503}`
		if strings.Contains(errorBody, `"code":"rate_limit_exceeded"`) {
			expected = `{"type":"invalid_request_error","code":"rate_limit_exceeded"}`
		}
		if strings.Contains(errorBody, `"code":429`) {
			expected = `{"type":"invalid_request_error","code":429}`
		}
		b, _ := json.Marshal(r.Response.Error)
		if string(b) != expected {
			t.Fatal("safe error fields", string(b))
		}
	}
}
