# GPT-6 Sol and Luna support

The built-in catalog includes `gpt-6-sol` and `gpt-6-luna` alongside Astra.
Existing default models and the Reserve fallback remain unchanged.

## Capability sources

The client snapshot was checked against Codex 0.156.0 on 2026-09-23:

| Codex subscription model | Default effort | Supported efforts | Context / maximum |
| --- | --- | --- | --- |
| GPT-6 Sol | medium | low, medium, high, xhigh, max, ultra | 272,000 / 872,000 |
| GPT-6 Luna | medium | low, medium, high, xhigh, max | 272,000 / 872,000 |

Both use Responses Lite and multi-agent v2. The embedded JSON is shared by
Rust and Go. Remote client metadata takes precedence; missing shipped GPT-6
entries are restored when an older remote catalog is refreshed.

Namespaced API routes use public API capabilities: `none` through `max`, a
1,050,000-token context window and 128,000 maximum output tokens. Responses
supports reasoning with tools. With Chat Completions, function calling for
these models requires `reasoning_effort: "none"`.

Sources: [Sol](https://developers.openai.com/api/docs/models/gpt-6-sol),
[Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
[Codex release notes](https://learn.chatgpt.com/docs/changelog).

## Upgrade behavior

Generated default catalogs receive the two models once. User-customized lists
and previously removed entries are preserved. Startup rebuilds stale managed
catalogs while retaining saved context and reasoning overrides; it does not
change the configured default model. Unknown or provider-owned model lists do
not acquire undeclared upstream models. Wakeup presets have a separate additive
migration. Model IDs stay unchanged when routed through provider gateways.

## Cost estimates

Standard USD per million tokens (input / cache read / cache write / output):

- Sol: 2 / 0.20 / 2.50 / 10.
- Luna: 0.10 / 0.01 / 0.125 / 0.50.

Above 272,000 input tokens, input and cache rates double and output rates
increase by 50%. Fast rates double; Flex rates halve. Cache writes use the
canonical token breakdown when available. These are API-based estimates,
not a statement of subscription credit charges.

Source: [API pricing](https://developers.openai.com/api/docs/pricing).
