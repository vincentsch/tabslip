package apis

func (c *Capture) messages(v map[string]any, stream bool) {
	kind, _ := v["type"].(string)
	if c.providerError(object(v["error"])) && stream && kind == "error" {
		c.Done = true
	}
	if stream {
		c.messagesContent(v)
		switch kind {
		case "message_stop":
			c.Done = true
		case "message_start":
			v = object(v["message"])
		}
	}
	r := c.Receipt
	replaceString(&r.Response.ID, v["id"])
	replaceString(&r.Response.Model, v["model"])
	replaceString(&r.Response.FinishReason, v["stop_reason"])
	replaceString(&r.Response.FinishReason, object(v["delta"])["stop_reason"])
	if u := object(v["usage"]); u != nil {
		r.Usage.Source = "reported"
		replaceInt(&r.Usage.Input, u["input_tokens"])
		replaceInt(&r.Usage.Output, u["output_tokens"])
		replaceInt(&r.Usage.Cached, u["cache_read_input_tokens"])
		replaceInt(&r.Usage.CacheWrite, u["cache_creation_input_tokens"])
		split := object(u["cache_creation"])
		replaceInt(&r.Usage.CacheWrite1h, split["ephemeral_1h_input_tokens"])
	}
}

func (c *Capture) messagesContent(v map[string]any) {
	kind, _ := v["type"].(string)
	switch kind {
	case "content_block_start":
		if text := String(object(v["content_block"])["text"]); text != nil {
			c.appendText(*text)
		}
	case "content_block_delta":
		delta := object(v["delta"])
		for _, key := range []string{"text", "partial_json", "thinking"} {
			if text := String(delta[key]); text != nil {
				c.appendText(*text)
			}
		}
	}
}
