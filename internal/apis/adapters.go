package apis

import (
	"encoding/json"

	"github.com/vincentsch/tabslip/internal/receipt"
)

// Endpoint selects metadata extraction without changing forwarding behavior.
func Endpoint(api, method, path string) string {
	if method != "POST" {
		return ""
	}
	if api == "anthropic" && path == "/v1/messages" {
		return "anthropic.messages"
	}
	if api == "openai" {
		switch path {
		case "/v1/chat/completions":
			return "openai.chat"
		case "/v1/responses":
			return "openai.responses"
		case "/v1/embeddings":
			return "openai.embeddings"
		}
	}
	return ""
}

func (c *Capture) providerError(e map[string]any) bool {
	if e == nil {
		return false
	}
	var code any
	switch x := e["code"].(type) {
	case string, json.Number:
		code = x
	}
	c.Receipt.Response.Error = &receipt.ProviderError{Type: String(e["type"]), Code: code}
	c.Receipt.Find("upstream_error", "observed", "The provider returned an error.")
	return true
}

func replaceString(dst **string, v any) {
	if s := String(v); s != nil {
		*dst = s
	}
}
func replaceInt(dst **int64, v any) {
	if n := integer(v); n != nil {
		*dst = n
	}
}

func (c *Capture) content(v map[string]any) {
	switch c.Receipt.Request.API {
	case "anthropic.messages":
		c.messagesContent(v)
	case "openai.responses":
		c.responsesContent(v)
	default:
		c.chatContent(v)
	}
}
