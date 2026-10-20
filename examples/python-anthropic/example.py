"""Make one Messages request using the project's Anthropic settings."""
import argparse
import os
from anthropic import Anthropic

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--tag", help="Tag this request only")
args = parser.parse_args()
client = Anthropic(api_key=os.environ["ANTHROPIC_API_KEY"],
                   base_url=os.environ.get("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
                   max_retries=0)
response = client.messages.create(
    model=os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-4-6"), max_tokens=64,
    messages=[{"role": "user", "content": "Say hello in one short sentence."}],
    extra_headers={"X-Tollglass-Tag": args.tag} if args.tag else {})
for block in response.content:
    if block.type == "text":
        print(block.text)
