package statement

import (
	"github.com/vincentsch/tollglass/internal/store"
	"strings"
	"testing"
)

func TestExactTotalsAndUnknownCoverage(t *testing.T) {
	tot := &Total{}
	tot.Add(store.Summary{})
	if tot.Amount() != "unknown" || tot.Reported() != "unknown" || tot.Estimated() != "unknown" {
		t.Fatal("unknown displayed as zero")
	}
	a, b := "0.100000000000000001", "2e-18"
	tot.Add(store.Summary{Cost: &a, CostSource: "reported"})
	tot.Add(store.Summary{Cost: &b, CostSource: "estimated"})
	if tot.Amount() != "0.100000000000000003" || tot.Reported() != a || tot.Estimated() != "0.000000000000000002" || tot.Unknown != 1 || tot.Calls != 3 {
		t.Fatal(tot.Amount(), tot.Reported(), tot.Estimated(), tot)
	}
}

func TestCallsCostLabels(t *testing.T) {
	zero, reported, estimated := "0", "0.02", "0.03"
	calls := []store.Call{
		{Summary: store.Summary{CostSource: "unknown"}},
		{Summary: store.Summary{Cost: &zero, CostSource: "reported"}},
		{Summary: store.Summary{Cost: &reported, CostSource: "reported"}},
		{Summary: store.Summary{Cost: &estimated, CostSource: "estimated"}},
	}
	var b strings.Builder
	if err := Write(&b, calls, "", ""); err != nil {
		t.Fatal(err)
	}
	_, table, _ := strings.Cut(b.String(), `<table class="calls">`)
	for _, cell := range []string{"<td>unknown</td>", "<td>0<br>reported</td>", "<td>0.02<br>reported</td>", "<td>0.03<br>estimated</td>"} {
		if strings.Count(table, cell) != 1 {
			t.Fatal("missing cost cell", cell, table)
		}
	}
}
