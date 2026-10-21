// Package statement renders unsigned, content-free spending statements.
package statement

import (
	_ "embed"
	"html/template"
	"io"
	"math/big"
	"sort"

	"github.com/vincentsch/tabslip/internal/store"
)

//go:embed tokens.css
var Tokens string

type Total struct {
	Calls               int
	Unknown             int
	reported, estimated big.Rat
	Known               int
}

func (t *Total) Add(s store.Summary) {
	t.Calls++
	if s.Cost == nil {
		t.Unknown++
		return
	}
	n, ok := new(big.Rat).SetString(*s.Cost)
	if !ok {
		t.Unknown++
		return
	}
	t.Known++
	if s.CostSource == "reported" {
		t.reported.Add(&t.reported, n)
	} else {
		t.estimated.Add(&t.estimated, n)
	}
}

// Decimal prints a finite decimal exactly, without rounding aggregate money.
func Decimal(n *big.Rat) string {
	d := new(big.Int).Set(n.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	a, b := 0, 0
	rem := new(big.Int)
	for {
		rem.Mod(d, two)
		if rem.Sign() != 0 {
			break
		}
		d.Div(d, two)
		a++
	}
	for {
		rem.Mod(d, five)
		if rem.Sign() != 0 {
			break
		}
		d.Div(d, five)
		b++
	}
	if b > a {
		a = b
	}
	return n.FloatString(a)
}
func (t *Total) Amount() string {
	if t.Known == 0 {
		return "unknown"
	}
	return Decimal(new(big.Rat).Add(&t.reported, &t.estimated))
}
func (t *Total) Reported() string {
	if t.Known == 0 {
		return "unknown"
	}
	return Decimal(&t.reported)
}
func (t *Total) Estimated() string {
	if t.Known == 0 {
		return "unknown"
	}
	return Decimal(&t.estimated)
}

type Group struct {
	Name  string
	Total *Total
}

func Groups(calls []store.Call, model bool) []Group {
	m := map[string]*Total{}
	for _, c := range calls {
		name := "(untagged)"
		if model {
			name = "unknown"
			if c.Summary.ReportedModel != nil {
				name = *c.Summary.ReportedModel
			} else if c.Summary.RequestedModel != nil {
				name = *c.Summary.RequestedModel
			}
		} else if c.Summary.Tag != nil {
			name = *c.Summary.Tag
		}
		if m[name] == nil {
			m[name] = &Total{}
		}
		m[name].Add(c.Summary)
	}
	names := []string{}
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	out := []Group{}
	for _, n := range names {
		out = append(out, Group{n, m[n]})
	}
	return out
}

var page = template.Must(template.New("statement").Funcs(template.FuncMap{"val": func(p *string) string {
	if p == nil {
		return "unknown"
	}
	return *p
}}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Tabslip statement</title><style>{{.CSS}}
*{box-sizing:border-box}body{margin:0;padding:32px 24px;background:var(--bg);color:var(--text);font:14px/1.5 ui-sans-serif,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}main{max-width:1200px;margin:auto}h1{font-size:28px;letter-spacing:-.04em;margin:0}h2{font-size:18px;margin-top:32px}p{color:var(--muted)}table{border-collapse:collapse;width:100%;font-variant-numeric:tabular-nums;background:var(--panel);table-layout:fixed}td,th{padding:9px 10px;border-bottom:1px solid var(--line);text-align:left;overflow-wrap:anywhere;vertical-align:top}th{color:var(--muted);font-weight:500}code{font:12px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;overflow-wrap:anywhere}.total{font-size:24px;color:var(--text)}@media(max-width:600px){body{padding:20px 16px}td,th{padding:7px 4px;font-size:12px}.calls th:nth-child(3),.calls td:nth-child(3){display:none}}@page{size:landscape;margin:12mm}
</style></head><body><main><h1>Tabslip statement</h1><p>{{.From}} → {{.To}}</p><p>Unsigned statement. No prompt or answer text is included.</p><p class="total">{{.Total.Amount}} USD</p><p>{{.Total.Calls}} calls · {{.Total.Unknown}} with unknown cost · Reported: {{.Total.Reported}} USD · Estimated: {{.Total.Estimated}} USD</p>
{{template "groups" .Tags}}{{template "groups" .Models}}
<h2>Calls</h2><table class="calls"><thead><tr><th>Receipt / started</th><th>Model / upstream</th><th>Tag / provider</th><th>Cost (USD)</th></tr></thead><tbody>{{range .Calls}}<tr><td><code>{{.Summary.ID}}</code><br>{{.Summary.Started}}</td><td>{{val .Summary.RequestedModel}} → {{val .Summary.ReportedModel}}<br>{{.Summary.Upstream}}</td><td>{{val .Summary.Tag}}<br>{{val .Summary.Provider}}</td><td>{{val .Summary.Cost}}{{if .Summary.Cost}}<br>{{.Summary.CostSource}}{{end}}</td></tr>{{else}}<tr><td colspan="4">No receipts match these filters.</td></tr>{{end}}</tbody></table><p>Model names, token counts and provider cost come from the provider. Tabslip did not check the model itself.</p></main></body></html>
{{define "groups"}}<h2>{{.Title}}</h2><table><thead><tr><th>Name</th><th>Calls / unknown</th><th>Total USD</th><th>Reported USD</th><th>Estimated USD</th></tr></thead><tbody>{{range .Groups}}<tr><td>{{.Name}}</td><td>{{.Total.Calls}} / {{.Total.Unknown}}</td><td>{{.Total.Amount}}</td><td>{{.Total.Reported}}</td><td>{{.Total.Estimated}}</td></tr>{{end}}</tbody></table>{{end}}`))

func Write(w io.Writer, calls []store.Call, from, to string) error {
	total := &Total{}
	for _, c := range calls {
		total.Add(c.Summary)
	}
	type groups struct {
		Title  string
		Groups []Group
	}
	return page.Execute(w, struct {
		CSS          template.CSS
		From, To     string
		Total        *Total
		Calls        []store.Call
		Tags, Models groups
	}{template.CSS(Tokens), from, to, total, calls, groups{"Totals by tag", Groups(calls, false)}, groups{"Totals by model", Groups(calls, true)}})
}
