package apis

import (
	"github.com/vincentsch/tollglass/internal/receipt"
	"strings"
	"testing"
	"time"
)

func has(r *receipt.Receipt, s string) bool {
	for _, f := range r.Findings {
		if f.Code == s {
			return true
		}
	}
	return false
}
func TestFallbackAndCumulativeUsage(t *testing.T) {
	r := receipt.New("tg_test", time.Now())
	_, ok := Request(r, []byte(`{"model":"first","models":["first","second"],"reasoning_effort":"low","reasoning":{"effort":"high"}}`))
	if !ok || r.Request.Params["reasoning_effort"] != "low" {
		t.Fatal("request mapping")
	}
	c := &Chat{Receipt: r}
	c.Parse(strings.NewReader("data: {\"model\":\"second\",\"provider\":\"Example\",\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\r\n\r\ndata: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n"), true)
	Finish(r)
	if !has(r, "fallback_used") || !has(r, "model_differs") || *r.Usage.Input != 5 || *r.Usage.Output != 7 || !c.Done {
		t.Fatal("stream mapping")
	}
}

func TestInvalidEventStopsMetadataButKeepsFramingAndLocalContent(t *testing.T) {
	r := receipt.New("tg_test", time.Now())
	c := &Chat{Receipt: r, Keep: true}
	c.Parse(strings.NewReader("data: {\"model\":\"known\",\"usage\":{\"prompt_tokens\":3}}\n\ndata: invalid\n\ndata: {\"model\":\"changed\",\"usage\":{\"prompt_tokens\":100},\"choices\":[{\"delta\":{\"content\":\"later text\"}}]}\n\ndata: [DONE]\n\n"), true)
	if !has(r, "metadata_unavailable") || r.Response.Model == nil || *r.Response.Model != "known" || r.Usage.Input == nil || *r.Usage.Input != 3 || !c.Done || !c.Truncated || string(c.Text) != "later text" {
		t.Fatal("metadata failure stopped framing or local content")
	}
}
