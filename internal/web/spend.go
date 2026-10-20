package web

import (
	"bytes"
	"fmt"
	"html/template"
	"math/big"
	"sort"
	"time"

	"github.com/vincentsch/tollglass/internal/statement"
	"github.com/vincentsch/tollglass/internal/store"
)

type spendGroup struct {
	Name                        string
	Total                       *statement.Total
	Input, Output               int64
	InputUnknown, OutputUnknown int
	Share                       string
}
type spendTable struct {
	Title string
	Rows  []*spendGroup
}
type spendData struct {
	Tiles  []spendTable
	Tables []spendTable
	Days   []*spendGroup
	Chart  template.HTML
}

func (s *Server) spend(f store.Filter, now time.Time) (spendData, error) {
	f.Limit = 0
	f.Cursor = ""
	f.CursorID = ""
	f.Upstream = ""
	f.Model = ""
	f.Search = ""
	f.Status = ""
	f.FindingsOnly = false
	rows, err := s.Reader.Summaries(f)
	if err != nil {
		return spendData{}, err
	}
	out := spendData{}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	add := func(g *spendGroup, v store.Summary) {
		g.Total.Add(v)
		if v.Input == nil {
			g.InputUnknown++
		} else {
			g.Input += *v.Input
		}
		if v.Output == nil {
			g.OutputUnknown++
		} else {
			g.Output += *v.Output
		}
	}
	for _, t := range []struct {
		name string
		days int
	}{{"Today (UTC)", 1}, {"7 days", 7}, {"30 days", 30}} {
		g := &spendGroup{Name: t.name, Total: &statement.Total{}}
		start := today.AddDate(0, 0, 1-t.days)
		for _, r := range rows {
			date, _ := time.Parse(time.RFC3339Nano, r.Started)
			if !date.Before(start) && date.Before(today.AddDate(0, 0, 1)) {
				add(g, r)
			}
		}
		out.Tiles = append(out.Tiles, spendTable{Title: t.name, Rows: []*spendGroup{g}})
	}
	for i := 29; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		g := &spendGroup{Name: date, Total: &statement.Total{}}
		for _, r := range rows {
			if len(r.Started) >= 10 && r.Started[:10] == date {
				add(g, r)
			}
		}
		out.Days = append(out.Days, g)
	}
	models := map[string]int{}
	for _, r := range rows {
		models[modelName(r)]++
	}
	for _, kind := range []string{"model", "provider", "upstream", "tag", "served by"} {
		groups := map[string]*spendGroup{}
		for _, r := range rows {
			name := ""
			switch kind {
			case "model":
				name = modelName(r)
			case "provider":
				name = value(r.Provider)
			case "upstream":
				name = r.Upstream
			case "tag":
				name = value(r.Tag)
			case "served by":
				name = modelName(r) + " · " + value(r.Provider)
			}
			if groups[name] == nil {
				groups[name] = &spendGroup{Name: name, Total: &statement.Total{}}
			}
			add(groups[name], r)
			if kind == "served by" {
				groups[name].Share = fmt.Sprintf("%.1f%% of %s calls", 100*float64(groups[name].Total.Calls)/float64(models[modelName(r)]), modelName(r))
			}
		}
		names := []string{}
		for n := range groups {
			names = append(names, n)
		}
		sort.Strings(names)
		t := spendTable{Title: "By " + kind}
		if kind == "served by" {
			t.Title = "Served by (reported providers)"
		}
		for _, n := range names {
			t.Rows = append(t.Rows, groups[n])
		}
		out.Tables = append(out.Tables, t)
	}
	out.Chart = chart(out.Days)
	return out, nil
}
func modelName(s store.Summary) string {
	if s.ReportedModel != nil {
		return *s.ReportedModel
	}
	return value(s.RequestedModel)
}
func chart(days []*spendGroup) template.HTML {
	var b bytes.Buffer
	b.WriteString(`<svg class="chart" role="img" aria-labelledby="chart-title" viewBox="0 0 900 190"><title id="chart-title">Daily cost for 30 UTC days. The table below includes exact totals and unknown coverage.</title>`)
	max := new(big.Rat)
	for _, d := range days {
		n, ok := new(big.Rat).SetString(d.Total.Amount())
		if ok && n.Cmp(max) > 0 {
			max = n
		}
	}
	for i, d := range days {
		height := 0.0
		if n, ok := new(big.Rat).SetString(d.Total.Amount()); ok && max.Sign() > 0 {
			ratio := new(big.Rat).Quo(n, max)
			height, _ = ratio.Float64()
			height *= 150
		}
		if height < 2 && d.Total.Calls > 0 {
			height = 2
		}
		amount := d.Total.Amount()
		if amount != "unknown" {
			amount += " USD"
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%.2f" width="20" height="%.2f" rx="2"><title>%s: %d calls; %s; reported %s; estimated %s; %d unknown</title></rect>`, i*30+5, 165-height, height, d.Name, d.Total.Calls, amount, d.Total.Reported(), d.Total.Estimated(), d.Total.Unknown)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func (g *spendGroup) InputText() string {
	if g.InputUnknown == g.Total.Calls {
		return "unknown"
	}
	return fmt.Sprint(g.Input)
}
func (g *spendGroup) OutputText() string {
	if g.OutputUnknown == g.Total.Calls {
		return "unknown"
	}
	return fmt.Sprint(g.Output)
}
