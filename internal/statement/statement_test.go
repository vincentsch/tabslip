package statement

import (
	"github.com/vincentsch/tollglass/internal/store"
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
