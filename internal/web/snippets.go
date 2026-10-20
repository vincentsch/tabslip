package web

import "github.com/vincentsch/tollglass/internal/upstream"

func (s *Server) snippets() []snippet {
	out := []snippet{}
	pythonKey, nodeKey := "", ""
	openaiPath, anthropicPath := "/openai/v1", "/anthropic"
	if s.Demo {
		openaiPath = "/demo/v1"
		anthropicPath = "/demo"
		pythonKey = `, api_key="demo"`
		nodeKey = ", apiKey: 'demo'"
	}
	for _, n := range upstream.Names(s.Upstreams) {
		u := s.BaseURL + "/" + n
		if s.Upstreams[n].API == "openai" {
			u += "/v1"
		}
		out = append(out, snippet{n + " base URL", u})
	}
	return append(out, []snippet{
		{"OpenAI Python", "from openai import OpenAI\nclient = OpenAI(base_url=\"" + s.BaseURL + openaiPath + "\"" + pythonKey + ")\nresponse = client.chat.completions.create(model=\"gpt-4.1\", messages=[{\"role\": \"user\", \"content\": \"Hello\"}])"},
		{"OpenAI Node", "import OpenAI from 'openai';\nconst client = new OpenAI({baseURL: '" + s.BaseURL + openaiPath + "'" + nodeKey + "});\nconst response = await client.chat.completions.create({model: 'gpt-4.1', messages: [{role: 'user', content: 'Hello'}]});"},
		{"Anthropic Python", "from anthropic import Anthropic\nclient = Anthropic(base_url=\"" + s.BaseURL + anthropicPath + "\"" + pythonKey + ")\nresponse = client.messages.create(model=\"claude-sonnet-4-6\", max_tokens=256, messages=[{\"role\": \"user\", \"content\": \"Hello\"}])"},
		{"Anthropic TypeScript", "import Anthropic from '@anthropic-ai/sdk';\nconst client = new Anthropic({baseURL: '" + s.BaseURL + anthropicPath + "'" + nodeKey + "});\nconst response = await client.messages.create({model: 'claude-sonnet-4-6', max_tokens: 256, messages: [{role: 'user', content: 'Hello'}]});"},
		{"curl", "curl " + s.BaseURL + openaiPath + "/chat/completions \\\n  -H \"Authorization: Bearer $OPENAI_API_KEY\" -H 'Content-Type: application/json' \\\n  -d '{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}'"},
		{"Environment variables", "export OPENAI_BASE_URL=" + s.BaseURL + openaiPath + "\nexport ANTHROPIC_BASE_URL=" + s.BaseURL + anthropicPath},
		{"Tag a call", "X-Tollglass-Tag: my-project\n\n# OpenAI Python\nextra_headers={\"X-Tollglass-Tag\": \"my-project\"}\n\n// OpenAI Node\n{headers: {'X-Tollglass-Tag': 'my-project'}}"},
		{"Set up with your coding agent", SetupPrompt},
	}...)
}
