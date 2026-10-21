package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/rungrad"
	"github.com/vincentsch/tabslip/internal/config"
)

const missingConfigFix = "Correct --config or TABSLIP_CONFIG, or create the selected YAML file."

func loadSettings(f *rungrad.Factory, cmd *cobra.Command) (config.Config, string, error) {
	c, missing, err := config.LoadWithMissing(f.ConfigPath())
	selected := cmd.Flags().Changed("config") || cmd.Root().PersistentFlags().Changed("config") || os.Getenv("TABSLIP_CONFIG") != ""
	if err != nil || !missing || !selected {
		return c, "", err
	}
	path := f.ConfigPath()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			path = filepath.Join("~", rel)
		}
	}
	return c, fmt.Sprintf("Selected config file %q is missing; using defaults.", path), nil
}

func warnMissingConfig(f *rungrad.Factory, message string) error {
	message += " " + missingConfigFix
	if f.Flags.JSON || f.Flags.JQ != "" || f.Flags.Template != "" {
		return json.NewEncoder(f.Stderr).Encode(map[string]string{"warning": message})
	}
	_, err := fmt.Fprintf(f.Stderr, "Warning: %s\n", message)
	return err
}
