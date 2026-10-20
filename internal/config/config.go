package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vincentsch/tollglass/internal/upstream"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen           string            `yaml:"listen"`
	Port             int               `yaml:"port"`
	KeepContent      bool              `yaml:"keep_content"`
	KeepContentTags  []string          `yaml:"keep_content_tags"`
	RetentionDays    int               `yaml:"retention_days"`
	EnrichOpenrouter bool              `yaml:"enrich_openrouter"`
	Upstreams        []upstream.Config `yaml:"upstreams"`
}

func Default() Config {
	return Config{Listen: "127.0.0.1", Port: 4545, RetentionDays: 90, EnrichOpenrouter: true}
}
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("cannot read config file")
	}
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if err = d.Decode(&c); err != nil && err != io.EOF {
		return c, fmt.Errorf("config file is malformed; check its YAML settings")
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("config file must contain one valid YAML document")
	}
	return c, nil
}
func Validate(c Config) error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if c.Listen == "" {
		return fmt.Errorf("listen address cannot be empty")
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("retention_days cannot be negative")
	}
	_, err := upstream.Build(c.Upstreams)
	return err
}
func DataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "tollglass")
	}
	h, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(h, "Library", "Application Support", "tollglass")
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "tollglass")
	}
	return filepath.Join(h, ".local", "share", "tollglass")
}
