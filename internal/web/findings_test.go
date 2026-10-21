package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/tabslip/internal/demo"
	"github.com/vincentsch/tabslip/internal/receipt"
	"github.com/vincentsch/tabslip/internal/store"
)

func findingsPanel(t *testing.T, body string) string {
	t.Helper()
	_, panel, ok := strings.Cut(body, `id="findings">`)
	if !ok {
		t.Fatal("missing findings panel")
	}
	panel, _, _ = strings.Cut(panel, "</div>")
	return panel
}

func TestReceiptFindingsSourcesAndResolution(t *testing.T) {
	for _, kind := range []string{"base", "demo", "provider attempts", "recovered cost"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := fixture(t)
			r := receipt.New(s.Store.KeyID, time.Now())
			r.Request.Upstream = "openrouter"
			switch kind {
			case "base", "demo":
				r.Find("model_differs", "reported", "A different model answered.")
			case "provider attempts":
				r.Find("fallback_used", "reported", "The router fell back between requested models.")
			case "recovered cost":
				r.Find("price_unknown", "observed", "No price was available.")
			}
			s.Store.Save(r, nil)
			if err := s.Store.Flush(); err != nil {
				t.Fatal(err)
			}
			before, _ := s.Reader.Get(r.ID)
			var supp *receipt.Supplement
			switch kind {
			case "demo":
				supp, _ = demo.Lookup(t.Context(), "gen-demo-fallback-test", "")
				bound := supplement(r, supp.Cost, supp.Provider)
				bound.Source = supp.Source
				bound.Findings = supp.Findings
				supp = bound
			case "provider attempts":
				supp = supplement(r, nil, nil)
				supp.Source = "openrouter.generation"
				supp.Findings = []receipt.Finding{{Code: "fallback_used", Level: "reported", Text: "The router tried two providers."}}
			case "recovered cost":
				cost := "0.02"
				supp = supplement(r, &cost, nil)
				supp.Source = "openrouter.generation"
			}
			if supp != nil {
				s.Store.SaveSupplement(supp)
				if err := s.Store.Flush(); err != nil {
					t.Fatal(err)
				}
			}
			signed, _ := s.Reader.Get(r.ID)
			body := request(s, "/r/"+r.ID).Body.String()
			panel := findingsPanel(t, body)
			switch kind {
			case "base":
				if strings.Count(panel, "model_differs") != 1 || !strings.Contains(panel, "Signed receipt") || strings.Contains(panel, "Resolved") {
					t.Fatal(panel)
				}
			case "demo":
				for _, text := range []string{"model_differs", "fallback_used", "Signed receipt", "Signed demo details (simulated)"} {
					if strings.Count(panel, text) != 1 {
						t.Fatal(panel)
					}
				}
			case "provider attempts":
				if strings.Count(panel, "fallback_used") != 2 || strings.Count(panel, "Signed receipt") != 1 || !strings.Contains(panel, "Signed OpenRouter details") || strings.Index(panel, "between requested models") > strings.Index(panel, "tried two providers") || !strings.Contains(panel, "between requested models") {
					t.Fatal(panel)
				}
			case "recovered cost":
				if strings.Count(panel, "price_unknown") != 1 || !strings.Contains(panel, "Resolved") {
					t.Fatal(panel)
				}
				filtered := request(s, "/?findings=on").Body.String()
				if strings.Contains(filtered, `data-id="`+r.ID+`"`) {
					t.Fatal("resolved finding remained active")
				}
				row := renderRow(t, signed.Summary)
				if strings.Contains(row, "● Findings") {
					t.Fatal(row)
				}
			}
			after, _ := s.Reader.Get(r.ID)
			if !bytes.Equal(before.Envelope, after.Envelope) {
				t.Fatal("base envelope changed")
			}
			a, _ := json.Marshal(signed.SupplementEnvelopes)
			b, _ := json.Marshal(after.SupplementEnvelopes)
			if !bytes.Equal(a, b) {
				t.Fatal("supplement envelopes changed")
			}
			if !strings.Contains(body, "Valid signature") {
				t.Fatal("signature invalid")
			}
		})
	}
}

func renderRow(t *testing.T, s store.Summary) string {
	t.Helper()
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, "row", s); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func costCell(t *testing.T, row string) string {
	t.Helper()
	cells := strings.Split(row, "<td")
	if len(cells) != 9 {
		t.Fatalf("unexpected row %s", row)
	}
	cell, _, _ := strings.Cut(cells[6], "</td>")
	return cell
}

func TestInitialAndLiveCostCells(t *testing.T) {
	s, _ := fixture(t)
	server := httptest.NewServer(s)
	defer server.Close()
	defer s.Close()
	u, _ := url.Parse(server.URL)
	s.Port = u.Port()
	s.BaseURL = server.URL
	resp, err := server.Client().Get(server.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 100)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer close(lines)
		scan := bufio.NewScanner(resp.Body)
		for scan.Scan() {
			lines <- scan.Text()
		}
	}()
	r := receipt.New(s.Store.KeyID, time.Now())
	for _, known := range []bool{false, true} {
		if known {
			zero := "0"
			s.Store.SaveSupplement(supplement(r, &zero, nil))
		} else {
			s.Store.Save(r, nil)
		}
		s.Store.Flush()
		var live string
		timeout := time.After(3 * time.Second)
	waiting:
		for {
			select {
			case line := <-lines:
				if !strings.HasPrefix(line, "data: {") {
					continue
				}
				var event struct{ ID, Row string }
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && event.ID == r.ID {
					live = event.Row
					break waiting
				}
			case <-timeout:
				t.Fatal("missing live row")
			}
		}
		page, err := server.Client().Get(server.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(page.Body)
		page.Body.Close()
		if err != nil || page.StatusCode != 200 {
			t.Fatal("Calls page", err, page.StatusCode)
		}
		initial := string(body)
		_, initial, _ = strings.Cut(initial, `<tr data-id="`+r.ID+`"`)
		initial, _, _ = strings.Cut(initial, "</tr>")
		initial = `<tr data-id="` + r.ID + `"` + initial + "</tr>"
		for _, row := range []string{initial, live} {
			cell := costCell(t, row)
			if known {
				if !strings.Contains(cell, ">0<br>") || !strings.Contains(cell, "reported") || strings.Contains(cell, "unknown") {
					t.Fatal(cell)
				}
			} else if strings.Count(cell, "unknown") != 1 {
				t.Fatal(cell)
			}
		}
	}
	s.Close()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("live stream did not close")
	}
}
