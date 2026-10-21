package web

import (
	"encoding/xml"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/vincentsch/tollglass/internal/receipt"
)

func TestReceiptStampClearance(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		findings                        int
		longValues, wrappedTotal, valid bool
	}{
		{name: "no findings", valid: true},
		{name: "one finding", findings: 1, valid: true},
		{name: "two findings invalid", findings: 2},
		{name: "three findings with wrapped values", findings: 3, longValues: true, valid: true},
		{name: "additional findings", findings: 5, valid: true},
		{name: "wrapped total invalid", wrappedTotal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := fixture(t)
			c, err := s.Reader.Get(r.ID)
			if err != nil {
				t.Fatal(err)
			}
			c.Receipt.Findings = nil
			for i := 0; i < tc.findings; i++ {
				text := fmt.Sprintf("Finding %d.", i+1)
				if tc.longValues && i == tc.findings-1 {
					text = "The provider reports that a fallback was used for this request (simulated)."
				}
				c.Receipt.Findings = append(c.Receipt.Findings, receipt.Finding{Code: "fallback_used", Level: "reported", Text: text})
			}
			if tc.longValues {
				long := "provider/long-model-name-with-a-dated-version-2026-10-21"
				provider := "A provider with a long reported service and region name"
				tag := strings.Repeat("client-", 9)
				c.Summary.RequestedModel, c.Summary.ReportedModel = &long, &long
				c.Summary.Provider, c.Summary.Tag = &provider, &tag
			}
			if tc.wrappedTotal {
				cost := "0.100000000000000001"
				c.Summary.Cost, c.Summary.CostSource = &cost, "reported"
			}
			svg, accessible := receiptSVG(c, tc.valid)
			type textNode struct {
				Y    float64 `xml:"y,attr"`
				Size float64 `xml:"font-size,attr"`
				Text string  `xml:",chardata"`
			}
			var doc struct {
				ViewBox string `xml:"viewBox,attr"`
				Path    struct {
					D string `xml:"d,attr"`
				} `xml:"path"`
				Texts []textNode `xml:"text"`
				Stamp struct {
					Transform string `xml:"transform,attr"`
					Rects     []struct {
						X      float64 `xml:"x,attr"`
						Y      float64 `xml:"y,attr"`
						Width  float64 `xml:"width,attr"`
						Height float64 `xml:"height,attr"`
						Stroke float64 `xml:"stroke-width,attr"`
					} `xml:"rect"`
					Texts []textNode `xml:"text"`
				} `xml:"g"`
			}
			if err := xml.Unmarshal([]byte(svg), &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Stamp.Rects) == 0 || len(doc.Stamp.Texts) != 2 {
				t.Fatal("missing stamp")
			}
			var angle, cx, cy float64
			if n, err := fmt.Sscanf(doc.Stamp.Transform, "rotate(%f %f %f)", &angle, &cx, &cy); err != nil || n != 3 {
				t.Fatal("invalid rotation", doc.Stamp.Transform)
			}
			rect := doc.Stamp.Rects[0]
			if rect.Width != 240 || rect.Height != 63 || rect.Stroke != 2 || angle != -6 {
				t.Fatal("stamp geometry changed")
			}
			top, bottom := math.Inf(1), math.Inf(-1)
			for _, x := range []float64{rect.X, rect.X + rect.Width} {
				for _, y := range []float64{rect.Y, rect.Y + rect.Height} {
					rotatedY := cy + (x-cx)*math.Sin(angle*math.Pi/180) + (y-cy)*math.Cos(angle*math.Pi/180)
					top, bottom = math.Min(top, rotatedY-rect.Stroke/2), math.Max(bottom, rotatedY+rect.Stroke/2)
				}
			}
			var body, footer []textNode
			for _, node := range doc.Texts {
				switch node.Size {
				case 12:
					body = append(body, node)
				case 10:
					footer = append(footer, node)
				}
			}
			if len(body) == 0 || len(footer) != 2 {
				t.Fatal("missing body or footer")
			}
			var bodyText []string
			for _, node := range body {
				if len([]rune(node.Text)) > 38 {
					t.Fatal("body exceeds receipt width")
				}
				bodyText = append(bodyText, node.Text)
			}
			stamp := "SIGNED"
			if !tc.valid {
				stamp = "INVALID"
			}
			if doc.Stamp.Texts[0].Text != stamp || doc.Stamp.Texts[1].Text != c.Receipt.Instance.KeyID {
				t.Fatal("stamp content changed")
			}
			expected := strings.Join(bodyText, "\n") + "\n" + stamp + " by your Tollglass " + c.Receipt.Instance.KeyID + "\nModel and token values are reported by the provider."
			if accessible != expected {
				t.Fatal("accessible text content or order differs")
			}
			last := body[len(body)-1]
			if tc.wrappedTotal && (strings.TrimSpace(last.Text) != "0.100000000000000001 USD (reported)" || body[len(body)-2].Text != "TOTAL") {
				t.Fatal("fixture did not wrap TOTAL", last.Text)
			}
			if tc.longValues && last.Text != "(simulated)." {
				t.Fatal("fixture did not wrap last finding", last.Text)
			}
			if tc.findings > 3 && !strings.Contains(last.Text, "+ 2 more") {
				t.Fatal("missing additional-findings summary")
			}
			bodyGap, footerGap := top-(last.Y+4), footer[0].Y-10-bottom
			t.Logf("body clearance %.3f; footer clearance %.3f", bodyGap, footerGap)
			if bodyGap < 12 {
				t.Errorf("body clearance %.3f < 12", bodyGap)
			}
			if footerGap < 12 {
				t.Errorf("footer clearance %.3f < 12", footerGap)
			}
			var vx, vy, width, height float64
			if n, err := fmt.Sscanf(doc.ViewBox, "%f %f %f %f", &vx, &vy, &width, &height); err != nil || n != 4 {
				t.Fatal("invalid viewBox")
			}
			cutEdge := height - 8
			if !strings.Contains(doc.Path.D, fmt.Sprintf(" L340 %g", cutEdge)) {
				t.Fatal("missing bottom cut edge")
			}
			if top < 8 || bottom > cutEdge || footer[len(footer)-1].Y+4 > cutEdge || width != 340 {
				t.Fatal("stamp or footer outside paper")
			}
		})
	}
}
