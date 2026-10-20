package apis

func (c *Capture) responses(v map[string]any, stream bool) {
	if stream {
		c.responsesContent(v)
		kind, _ := v["type"].(string)
		switch kind {
		case "error":
			// Providers send nested errors as well as the documented flat code.
			e := object(v["error"])
			if e == nil {
				e = map[string]any{"code": v["code"]}
			}
			c.providerError(e)
			c.Done = true
			return
		case "response.output_text.delta", "response.function_call_arguments.delta", "response.reasoning_text.delta":
			return
		case "response.completed", "response.incomplete", "response.failed":
			c.Done = true
		}
		v = object(v["response"])
	}
	r := c.Receipt
	replaceString(&r.Response.ID, v["id"])
	replaceString(&r.Response.Model, v["model"])
	replaceString(&r.Response.APIStatus, v["status"])
	c.providerError(object(v["error"]))
	if r.Response.APIStatus != nil && *r.Response.APIStatus == "failed" {
		r.Find("upstream_error", "observed", "The provider reported a failed response.")
	}
	if u := object(v["usage"]); u != nil {
		r.Usage.Source = "reported"
		replaceInt(&r.Usage.Input, u["input_tokens"])
		replaceInt(&r.Usage.Output, u["output_tokens"])
		replaceInt(&r.Usage.Cached, object(u["input_tokens_details"])["cached_tokens"])
		replaceInt(&r.Usage.Reasoning, object(u["output_tokens_details"])["reasoning_tokens"])
	}
}

func (c *Capture) responsesContent(v map[string]any) {
	switch v["type"] {
	case "response.output_text.delta", "response.function_call_arguments.delta", "response.reasoning_text.delta":
		if text := String(v["delta"]); text != nil {
			c.appendText(*text)
		}
	}
}
