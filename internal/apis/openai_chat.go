package apis

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/vincentsch/tollglass/internal/receipt"
)

const Limit = 10 * 1024 * 1024

func Object(b []byte) (map[string]any, bool) {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&v) != nil || v == nil {
		return nil, false
	}
	var extra any
	return v, d.Decode(&extra) == io.EOF
}
func String(v any) *string {
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func integer(v any) *int64 {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	i, e := n.Int64()
	if e != nil || i < 0 {
		return nil
	}
	return &i
}
func Unavailable(r *receipt.Receipt) {
	r.Find("metadata_unavailable", "observed", "Some call metadata could not be decoded within the parsing limit.")
}
func Request(r *receipt.Receipt, b []byte) (map[string]any, bool) {
	if len(b) > Limit {
		Unavailable(r)
		return nil, false
	}
	v, ok := Object(b)
	if !ok {
		Unavailable(r)
		return nil, false
	}
	r.Request.Model = String(v["model"])
	if s, ok := v["stream"].(bool); ok {
		r.Request.Stream = &s
	}
	for _, k := range []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "max_output_tokens", "reasoning_effort", "seed", "tool_choice", "provider", "models"} {
		if v, ok := v[k]; ok {
			r.Request.Params[k] = v
		}
	}
	if _, ok := r.Request.Params["reasoning_effort"]; !ok {
		if x := object(v["reasoning"])["effort"]; x != nil {
			r.Request.Params["reasoning_effort"] = x
		}
	}
	if x := object(v["response_format"])["type"]; x != nil {
		r.Request.Params["response_format"] = x
	}
	if tools, ok := v["tools"].([]any); ok {
		r.Request.Params["tools"] = len(tools)
	}
	return v, true
}

type Chat struct {
	Receipt   *receipt.Receipt
	Done      bool
	Keep      bool
	Text      []byte
	Truncated bool
}

func (c *Chat) appendText(s string) {
	if !c.Keep {
		return
	}
	n := Limit - len(c.Text)
	if len(s) > n {
		s = s[:n]
		c.Truncated = true
	}
	c.Text = append(c.Text, s...)
}
func (c *Chat) value(b []byte, stream bool) bool {
	v, ok := Object(b)
	if !ok {
		Unavailable(c.Receipt)
		return false
	}
	r := c.Receipt
	for k, p := range map[string]**string{"id": &r.Response.ID, "model": &r.Response.Model, "provider": &r.Response.Provider, "system_fingerprint": &r.Response.SystemFingerprint} {
		if s := String(v[k]); s != nil {
			*p = s
		}
	}
	if e := object(v["error"]); e != nil {
		var code any
		switch x := e["code"].(type) {
		case string, json.Number:
			code = x
		}
		r.Response.Error = &receipt.ProviderError{Type: String(e["type"]), Code: code}
		r.Find("upstream_error", "observed", "The provider returned an error.")
	}
	if u := object(v["usage"]); u != nil {
		r.Usage.Source = "reported"
		for _, f := range []struct {
			v any
			p **int64
		}{{u["prompt_tokens"], &r.Usage.Input}, {u["completion_tokens"], &r.Usage.Output}, {object(u["prompt_tokens_details"])["cached_tokens"], &r.Usage.Cached}, {object(u["completion_tokens_details"])["reasoning_tokens"], &r.Usage.Reasoning}} {
			if x := integer(f.v); x != nil {
				*f.p = x
			}
		}
	}
	if a, ok := v["choices"].([]any); ok {
		for _, choice := range a {
			ch := object(choice)
			if f := String(ch["finish_reason"]); f != nil {
				r.Response.FinishReason = f
			}

		}
	}
	if stream {
		c.content(v)
	}
	return true
}
func (c *Chat) content(v map[string]any) {
	if !c.Keep {
		return
	}
	if a, ok := v["choices"].([]any); ok {
		for _, choice := range a {
			delta := object(object(choice)["delta"])
			if text := String(delta["content"]); text != nil {
				c.appendText(*text)
			}
			if calls, ok := delta["tool_calls"].([]any); ok {
				for _, call := range calls {
					if text := String(object(object(call)["function"])["arguments"]); text != nil {
						c.appendText(*text)
					}
				}
			}
		}
	}
}
func (c *Chat) Parse(src io.Reader, stream bool) {
	if !stream {
		b, err := io.ReadAll(io.LimitReader(src, Limit+1))
		if c.Keep {
			n := len(b)
			if n > Limit {
				n = Limit
				c.Truncated = true
			}
			c.Text = append(c.Text, b[:n]...)
		}
		if err != nil || len(b) > Limit {
			Unavailable(c.Receipt)
			return
		}
		c.value(b, false)
		return
	}
	reader := bufio.NewReaderSize(src, 32*1024)
	var event []byte
	size := 0
	overflow := false
	metadata := true
	unavailable := func() {
		Unavailable(c.Receipt)
		metadata = false
		if c.Keep {
			c.Truncated = true
		}
	}
	dispatch := func() {
		if overflow || len(event) == 0 {
			return
		}
		b := bytes.TrimSpace(event)
		if bytes.Equal(b, []byte("[DONE]")) {
			c.Done = true
			return
		}
		if metadata {
			if !c.value(b, true) {
				unavailable()
			}
		} else if c.Keep {
			if v, ok := Object(b); ok {
				c.content(v)
			}
		}
	}
	for {
		var full []byte
		var err error
		var part []byte
		lineBytes := 0
		for {
			part, err = reader.ReadSlice('\n')
			size += len(part)
			lineBytes += len(part)
			if size > Limit {
				if !overflow {
					unavailable()
				}
				overflow = true
				event = nil
				full = nil
			}
			if !overflow {
				full = append(full, part...)
			}
			if err != bufio.ErrBufferFull {
				break
			}
		}
		full = bytes.TrimSuffix(bytes.TrimSuffix(full, []byte("\n")), []byte("\r"))
		blank := (lineBytes == 1 && bytes.Equal(part, []byte("\n"))) || (lineBytes == 2 && bytes.Equal(part, []byte("\r\n")))
		if blank {
			dispatch()
			event = nil
			size = 0
			overflow = false
		} else if !overflow && bytes.HasPrefix(full, []byte("data:")) {
			data := bytes.TrimPrefix(bytes.TrimPrefix(full, []byte("data:")), []byte(" "))
			if len(event) > 0 {
				event = append(event, '\n')
			}
			event = append(event, data...)
		}
		if err != nil {
			if err != io.EOF {
				unavailable()
			}
			return
		}
	}
}

var aliasSuffix = regexp.MustCompile(`^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{4}|[0-9]{3}|v[0-9]+(?:\.[0-9]+)*)$`)

func Finish(r *receipt.Receipt) {
	if r.Usage.Source == "unknown" {
		r.Find("usage_missing", "observed", "The provider did not report token usage.")
	}
	if r.Request.Model != nil && r.Response.Model != nil && *r.Request.Model != *r.Response.Model {
		a, b := *r.Request.Model, *r.Response.Model
		suffix, ok := strings.CutPrefix(b, a+"-")
		if ok && aliasSuffix.MatchString(suffix) {
			r.Find("model_alias_resolved", "reported", "The provider reports a version of the requested model.")
		} else {
			r.Find("model_differs", "reported", "You asked for "+a+". The provider reports "+b+".")
		}
	}
	if models, ok := r.Request.Params["models"].([]any); ok && r.Response.Model != nil {
		for i, m := range models {
			if s, ok := m.(string); ok && s == *r.Response.Model {
				if i > 0 {
					r.Find("fallback_used", "reported", "The provider reports a later model from the requested fallback list.")
				}
				break
			}
		}
	}
}
