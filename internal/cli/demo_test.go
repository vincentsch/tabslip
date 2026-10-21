package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/rungrad/testutil"
	"github.com/vincentsch/tabslip/internal/demo"
)

func TestDemoSeedFailureKeepsRuntimeError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	app := NewApp()
	app.AddCommand(&rungrad.Command{Use: "seed-failure", Short: "Exercise a failed demo seed", Run: func(f *rungrad.Factory, _ *cobra.Command, _ []string) error {
		return runSeededDemo(f, dir, false, false, false, func(ctx context.Context, client *http.Client, _ string) error {
			return demo.Seed(ctx, client, upstream.URL)
		})
	}})
	result := testutil.Run(app, "seed-failure")
	if result.Exit != 2 || !strings.Contains(result.Stderr, "Cannot complete the demo calls.") || result.Stdout != "" {
		t.Fatalf("%+v", result)
	}
}
