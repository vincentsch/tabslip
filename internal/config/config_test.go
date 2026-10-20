package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	c, e := Load(path)
	if e != nil || c.Port != 4545 || c.KeepContent {
		t.Fatal("defaults")
	}
	os.WriteFile(path, []byte("port: 4599\nupstreams:\n  - name: local-test\n    base_url: http://127.0.0.1:4598\n    api: openai\n    inject_usage: false\n"), 0600)
	c, e = Load(path)
	if e != nil || c.Port != 4599 || c.Upstreams[0].InjectUsage == nil || *c.Upstreams[0].InjectUsage {
		t.Fatal("custom YAML")
	}
	for _, b := range []string{"port: [bad", "port: 4545\n---\n[broken", "port: 70000", "unknown: true", "upstreams:\n - name: openai\n   base_url: http://localhost\n   api: openai"} {
		os.WriteFile(path, []byte(b), 0600)
		if c, e = Load(path); e == nil && Validate(c) == nil {
			t.Fatalf("invalid config accepted: %s", b)
		}
	}
}
