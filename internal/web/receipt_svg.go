package web

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/store"
)

// Wrap keeps every rune, preferring the separators common in model names.
func wrap(s string, width int) []string {
	r := []rune(s)
	out := []string{}
	for len(r) > width {
		cut := width
		for i := width - 1; i > width/2; i-- {
			if strings.ContainsRune("/-. ", r[i]) || unicode.IsSpace(r[i]) {
				cut = i + 1
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	return append(out, string(r))
}
func receiptSVG(c store.Call, valid bool) (template.HTML, string) {
	r := c.Receipt
	s := c.Summary
	lines := []string{"TABSLIP", r.Time.Started, "RECEIPT " + r.ID[:min(len(r.ID), 12)] + "…" + r.ID[max(0, len(r.ID)-8):], "──────────────────────────────────────"}
	add := func(label, v string) {
		if len([]rune(label))+len([]rune(v))+1 <= 38 {
			lines = append(lines, label+strings.Repeat(" ", 38-len([]rune(label))-len([]rune(v)))+v)
			return
		}
		lines = append(lines, label)
		for _, part := range wrap(v, 38) {
			lines = append(lines, strings.Repeat(" ", 38-len([]rune(part)))+part)
		}
	}
	add("UPSTREAM", s.Upstream)
	add("API", s.API)
	add("MODEL ASKED", value(s.RequestedModel))
	mark := "MODEL REPORTED"
	if value(s.RequestedModel) != value(s.ReportedModel) {
		mark += " ≠"
	}
	add(mark, value(s.ReportedModel))
	add("PROVIDER (REPORTED)", value(s.Provider))
	add("TAG", value(s.Tag))
	lines = append(lines, "──────────────────────────────────────")
	for _, v := range []struct{ name, val string }{{"TOKENS IN", number(s.Input)}, {"TOKENS OUT", number(s.Output)}, {"CACHED", number(r.Usage.Cached)}, {"REASONING", number(r.Usage.Reasoning)}, {"FIRST BYTE", number(s.TTFB) + " ms"}, {"TOTAL TIME", strconv.FormatInt(s.Total, 10) + " ms"}, {"STATUS", number(s.Status)}, {"TOTAL", costText(s.Cost, s.CostSource)}} {
		add(v.name, v.val)
	}
	var supp *receipt.Supplement
	if len(c.Supplements) > 0 {
		supp = &c.Supplements[len(c.Supplements)-1]
	}
	_, _, findings := receipt.Combined(&r, supp)
	for i, f := range findings {
		if i == 3 {
			add("FINDINGS", fmt.Sprintf("+ %d more", len(findings)-3))
			break
		}
		prefix := "! "
		if f.Code == "model_alias_resolved" {
			prefix = "i "
		}
		lines = append(lines, wrap(prefix+f.Text, 38)...)
	}
	stamp := "SIGNED"
	ink := "#146c43"
	if !valid {
		stamp = "INVALID"
		ink = "#ae2929"
	}
	// Reserve a block below the completed body, including descenders and the
	// rotated outer border. Sharp corners conservatively bound the rounded stamp.
	angle := 6 * math.Pi / 180
	stampAbove := 20*math.Cos(angle) + 120*math.Sin(angle) + 1
	stampBelow := 43*math.Cos(angle) + 120*math.Sin(angle) + 1
	lastBaseline := 30 + (len(lines)-1)*17
	y := lastBaseline + 4 + 12 + int(math.Ceil(stampAbove))
	// The first footer baseline is h-35; allow its 10-unit ascent and a gap.
	h := max(len(lines)*17+180, y+int(math.Ceil(stampBelow))+12+10+35)
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg class="paper" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="receipt-title" viewBox="0 0 340 %d"><title id="receipt-title">Receipt %s, %s by your Tabslip</title><path fill="#F4EFE4" d="M0 8`, h, html.EscapeString(r.ID), stamp)
	for x := 0; x < 340; x += 8 {
		fmt.Fprintf(&b, " L%d 0 L%d 8", x+4, x+8)
	}
	fmt.Fprintf(&b, " L340 %d", h-8)
	for x := 340; x > 0; x -= 8 {
		fmt.Fprintf(&b, " L%d %d L%d %d", x-4, h, x-8, h-8)
	}
	b.WriteString(` Z"/>`)
	for i, line := range lines {
		fmt.Fprintf(&b, `<text x="16" y="%d" fill="#26231D" font-family="ui-monospace,monospace" font-size="12" xml:space="preserve">%s</text>`, 30+i*17, html.EscapeString(line))
	}
	fmt.Fprintf(&b, `<g transform="rotate(-6 170 %d)" fill="%s"><rect x="50" y="%d" width="240" height="63" rx="8" fill="none" stroke="%s" stroke-width="2"/><rect x="54" y="%d" width="232" height="55" rx="6" fill="none" stroke="%s" stroke-dasharray="3 2"/><text x="170" y="%d" text-anchor="middle" font-family="monospace" font-size="24">%s</text><text x="170" y="%d" text-anchor="middle" font-family="monospace" font-size="11">%s</text></g>`, y, ink, y-20, ink, y-16, ink, y+8, stamp, y+28, html.EscapeString(r.Instance.KeyID))
	fmt.Fprintf(&b, `<text x="16" y="%d" fill="#26231D" font-size="10">Model and token values are reported</text><text x="16" y="%d" fill="#26231D" font-size="10">by the provider.</text></svg>`, h-35, h-20)
	return template.HTML(b.String()), strings.Join(lines, "\n") + "\n" + stamp + " by your Tabslip " + r.Instance.KeyID + "\nModel and token values are reported by the provider."
}

func costText(amount *string, source string) string {
	if amount == nil {
		return "unknown"
	}
	return *amount + " USD (" + source + ")"
}
