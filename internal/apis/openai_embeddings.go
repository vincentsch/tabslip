package apis

func (c *Capture) embeddings(v map[string]any) {
	replaceString(&c.Receipt.Response.Model, v["model"])
	c.providerError(object(v["error"]))
	if u := object(v["usage"]); u != nil {
		c.Receipt.Usage.Source = "reported"
		replaceInt(&c.Receipt.Usage.Input, u["prompt_tokens"])
	}
}
