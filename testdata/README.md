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

## OpenRouter

`openrouter-generation.json` adapts the raw `data` wrapper and selected fields
from the [official generation reference](https://openrouter.ai/docs/api/api-reference/generations/get-generation)
(read 2026-10-20). The non-null `provider_responses` structure comes from the
[official generation metadata schema](https://github.com/OpenRouterTeam/skills/blob/main/skills/openrouter-generations/SKILL.md#response-schemas),
which shows `provider_name` and HTTP `status` inside the raw `data` wrapper.
This fixture extends that example to two synthetic attempts. The API reference
also supplies native token counts and region, and demonstrates a null list.
IDs, statuses and attempts are synthetic; unused fields test that private
metadata is discarded. Tests also replace the attempt list with null.

`openrouter-chat.sse` adapts [OpenRouter streaming](https://openrouter.ai/docs/api/reference/streaming)
and [usage accounting](https://openrouter.ai/docs/guides/guides/usage-accounting).
IDs, answer and counts are synthetic. Top-level `provider` is optional and
appears only in the first event here. Tests remove it entirely to prove that
absent providers stay null. `usage.cost` is a JSON number whose decimal text
must survive signing, including reported zero.
