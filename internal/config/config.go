package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vincentsch/tabslip/internal/upstream"
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
	c, _, err := LoadWithMissing(path)
	return c, err
}

// LoadWithMissing reports an absent file while keeping first-run defaults.
func LoadWithMissing(path string) (Config, bool, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, true, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("cannot read config file")
	}
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if err = d.Decode(&c); err != nil && err != io.EOF {
		return c, false, fmt.Errorf("config file is malformed; check its YAML settings")
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, false, fmt.Errorf("config file must contain one valid YAML document")
	}
	return c, false, nil
}
func Validate(c Config) error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	switch c.Listen {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0", "::":
	default:
		return fmt.Errorf("unsupported listen address %q; use loopback (127.0.0.1, localhost or ::1), or --listen 0.0.0.0 (IPv6: --listen ::) to expose the proxy and keep the page on loopback", c.Listen)
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("retention_days cannot be negative")
	}
	_, err := upstream.Build(c.Upstreams)
	return err
}
func DataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "tabslip")
	}
	h, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(h, "Library", "Application Support", "tabslip")
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "tabslip")
	}
	return filepath.Join(h, ".local", "share", "tabslip")
}

// SaveSettings replaces only settings owned by the local page. Other YAML nodes,
// including custom upstreams and comments, survive the atomic replacement.
// A startup override leaves the existing keep_content node untouched.
func SaveSettings(path string, c Config, keepContentLocked bool) error {
	var doc yaml.Node
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot read config file")
	}
	if len(b) == 0 {
		b = []byte("{}\n")
	}
	if yaml.Unmarshal(b, &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config file is malformed; check its YAML settings")
	}
	for _, v := range []struct {
		name  string
		value any
	}{{"keep_content", c.KeepContent}, {"keep_content_tags", c.KeepContentTags}, {"retention_days", c.RetentionDays}} {
		if keepContentLocked && v.name == "keep_content" {
			continue
		}
		var node yaml.Node
		if err = node.Encode(v.value); err != nil {
			return err
		}
		m := doc.Content[0]
		found := false
		for i := 0; i < len(m.Content); i += 2 {
			if m.Content[i].Value == v.name {
				m.Content[i+1] = &node
				found = true
				break
			}
		}
		if !found {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: v.name}, &node)
		}
	}
	b, err = yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("cannot encode settings")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("cannot create config directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tabslip-config-*")
	if err != nil {
		return fmt.Errorf("cannot create config file")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("cannot save config file; check directory permissions and free space")
	}
	return nil
}
