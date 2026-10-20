"""Make one chat request using the project's OpenAI settings."""

import argparse
import os

from openai import OpenAI


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", help="Send X-Tollglass-Tag for this request only")
    args = parser.parse_args()
    client = OpenAI(
        api_key=os.environ["OPENAI_API_KEY"],
        base_url=os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1"),
        max_retries=0,
    )
    response = client.chat.completions.create(
        model="gpt-4.1",
        messages=[{"role": "user", "content": "Say hello in one short sentence."}],
        max_completion_tokens=64,
        extra_headers={"X-Tollglass-Tag": args.tag} if args.tag else {},
    )
    print(response.choices[0].message.content)


if __name__ == "__main__":
    main()
