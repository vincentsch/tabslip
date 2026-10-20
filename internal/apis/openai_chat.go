package apis

import "github.com/vincentsch/tollglass/internal/prices"

func (c *Capture) chat(v map[string]any, stream bool) {
	r := c.Receipt
	for k, p := range map[string]**string{"id": &r.Response.ID, "model": &r.Response.Model, "provider": &r.Response.Provider, "system_fingerprint": &r.Response.SystemFingerprint} {
		if s := String(v[k]); s != nil {
			*p = s
		}
	}
	c.providerError(object(v["error"]))
	if u := object(v["usage"]); u != nil {
		r.Usage.Source = "reported"
		if amount := prices.Decimal(u["cost"]); amount != nil {
			usd := "USD"
			r.Cost.Source, r.Cost.Amount, r.Cost.Currency = "reported", amount, &usd
		}
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
}
func (c *Capture) chatContent(v map[string]any) {
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
