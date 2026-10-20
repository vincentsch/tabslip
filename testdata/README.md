# Chat response fixtures

`chat.json` and `chat.sse` adapt the JSON and streaming examples from the
[official Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create),
read on 2026-10-20. Model names use `gpt-4.1-2025-04-14` to test dated aliases.
The JSON adds a synthetic fingerprint and omits unused token details.
The stream adds `ü` for UTF-8 splits and a final usage event using the
reference's `stream_options.include_usage` contract. All counts are synthetic.
Tests construct large tool events, errors and compression locally. There are
no provider transcripts or working API credentials here.

## Messages, Responses and embeddings

These small examples adapt the official references, read on 2026-10-20:

- `messages.json`, `messages.sse`, `messages-error.sse`:
  [Messages streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)
  and [cache usage](https://platform.claude.com/docs/en/build-with-claude/prompt-caching).
  Model and IDs are synthetic. Usage is 100 uncached, 50 cache reads and
  25 cache writes (15 five-minute, 10 one-hour); final output is 12.
- `responses.json`, `responses-{completed,incomplete,failed,error}.sse`:
  [Responses events](https://developers.openai.com/api/reference/resources/responses/streaming-events).
  IDs, model, text and counts are synthetic; unrelated fields are omitted.
  Terminal variants exercise provider status, nested errors without a type,
  and a separate top-level error with a numeric code and null error type.
  Standalone-error streams now start with `response.created` in progress,
  empty output and null usage; no usage is invented before the error.
- `responses-nested-error.sse` adapts the nested shape reported in
  [the official .NET SDK issue tracker](https://github.com/openai/openai-dotnet/issues/881).
  Type, code and sensitive message markers are synthetic. Tests check both
  string and numeric nested codes. A separate synthetic sequence injects
  earlier cumulative usage solely to verify it survives a terminal error.
  Tests append a `[DONE]` trailer using the
  [OpenRouter Responses example](https://openrouter.ai/docs/api/reference/responses/basic-usage);
  this trailer must not override each API's own terminal-event requirement.
- `embeddings.json`:
  [Create embeddings](https://developers.openai.com/api/reference/resources/embeddings/methods/create).
  The vector is shortened to three synthetic values; model and counts are
  synthetic. Total tokens are deliberately present to check they are ignored.

The error messages contain only synthetic sensitive markers to test redaction.
Tests also remove usage, insert null cumulative fields and cut off terminal
frames to check missing metadata and incomplete captures.
