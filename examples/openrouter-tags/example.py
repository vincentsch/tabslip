"""Tag an OpenRouter chat request with its project name."""
import os
from openai import OpenAI

client = OpenAI(api_key=os.environ["OPENROUTER_API_KEY"],
                base_url=os.environ.get("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
                default_headers={"X-Tollglass-Tag": os.environ.get("TOLLGLASS_TAG", "my-project")},
                max_retries=0)
response = client.chat.completions.create(
    model=os.environ.get("OPENROUTER_MODEL", "openai/gpt-4.1"), max_completion_tokens=64,
    messages=[{"role": "user", "content": "Say hello in one short sentence."}])
print(response.choices[0].message.content)
print("Request id:", response.id)
