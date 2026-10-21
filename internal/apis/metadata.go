package apis

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/vincentsch/tabslip/internal/receipt"
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
	if _, ok := r.Request.Params["reasoning_effort"]; !ok || r.Request.API == "openai.responses" {
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
