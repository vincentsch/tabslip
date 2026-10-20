package upstream

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type Config struct {
	Name        string `yaml:"name"`
	BaseURL     string `yaml:"base_url"`
	API         string `yaml:"api"`
	InjectUsage *bool  `yaml:"inject_usage"`
}
type Upstream struct {
	Name        string
	URL         *url.URL
	API         string
	InjectUsage bool
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func Build(custom []Config) (map[string]Upstream, error) {
	out := map[string]Upstream{}
	for _, c := range []Config{{Name: "openai", BaseURL: "https://api.openai.com", API: "openai"}, {Name: "anthropic", BaseURL: "https://api.anthropic.com", API: "anthropic"}, {Name: "openrouter", BaseURL: "https://openrouter.ai/api", API: "openai"}, {Name: "ollama", BaseURL: "http://localhost:11434", API: "openai"}} {
		u, _ := url.Parse(c.BaseURL)
		out[c.Name] = Upstream{c.Name, u, c.API, c.API == "openai" && c.Name != "openrouter"}
	}
	for _, c := range custom {
		if _, ok := out[c.Name]; ok {
			return nil, fmt.Errorf("upstream name %q is reserved or repeated", c.Name)
		}
		switch c.Name {
		case "setup", "spend", "r", "events", "settings":
			return nil, fmt.Errorf("upstream name %q is reserved for the local page; choose another name", c.Name)
		}
		if !namePattern.MatchString(c.Name) || c.Name == "healthz" {
			return nil, fmt.Errorf("invalid upstream name")
		}
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/v1") {
			return nil, fmt.Errorf("upstream %s needs an http(s) base URL without credentials, query or /v1", c.Name)
		}
		if c.API != "openai" && c.API != "anthropic" {
			return nil, fmt.Errorf("upstream %s api must be openai or anthropic", c.Name)
		}
		inject := c.API == "openai"
		if c.InjectUsage != nil {
			inject = inject && *c.InjectUsage
		}
		out[c.Name] = Upstream{c.Name, u, c.API, inject}
	}
	return out, nil
}
func Names(m map[string]Upstream) []string {
	n := make([]string, 0, len(m))
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
