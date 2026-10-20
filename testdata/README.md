# Chat response fixtures

`chat.json` and `chat.sse` adapt the JSON and streaming examples from the
[official Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create),
read on 2026-10-20. Model names use `gpt-4.1-2025-04-14` to test dated aliases.
The JSON adds a synthetic fingerprint and omits unused token details.
The stream adds `ü` for UTF-8 splits and a final usage event using the
reference's `stream_options.include_usage` contract. All counts are synthetic.
Tests construct large tool events, errors and compression locally. There are
no provider transcripts or working API credentials here.
