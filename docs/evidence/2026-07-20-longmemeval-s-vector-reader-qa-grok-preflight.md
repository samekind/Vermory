# LongMemEval-S Vector Reader QA Grok Preflight

Date: 2026-07-20 Asia/Shanghai

Status: formal W29 dataset execution not started because the required Grok
provider failed preflight authentication

## Scope

W29 freezes a contemporaneous comparison between
`vermory_lexical_k10` and `vermory_vector_k10` with the same isolated Grok
reader and custom judge used by W15. Provider substitution is forbidden for
that execution. This record preserves the failed preflight instead of
relabeling another provider as W29.

## Verified Runtime

| Field | Value |
|---|---|
| Implementation revision | `ffcdcfeb9d7955e5b48d32442985752230cb9b6a` |
| Protected CI | `test` and `sign-snapshot` passed on the exact head |
| Binary SHA-256 | `c5d58fd29b845109b74c2715edc7bfd842ee4fe727d1ecf469ae4dca864ca49b` |
| Source archive SHA-256 | `9a5b02811ee5850a6f0a95377bbbe431c67dcc97714797899b457253043a4d34` |
| Grok CLI | `0.2.101 (5bc4b5dfadcf)` |
| Isolated wrapper SHA-256 | `906ba73d9006122b38a34b482859b05c202c557620cac6bc9e0e37a4860d06f7` |
| Isolated config SHA-256 | `0cd9013bd759fa6ff44a77b52890d2551229081203ddb341dc082bb774eb5753` |
| Final inspect SHA-256 | `720c54804586f17db7c1b701fb3e21978d7e9317af5860d3a5323606400312f4` |

The isolated state used mode-`0700` HOME and GROK_HOME directories. Only the
current `auth.json`, `agent_id`, and an operator-owned compatibility config
were present before model execution. The final inspect reported zero project
instructions, plugins, skills, MCP servers, hooks, marketplaces, LSP servers,
and enabled Cursor, Claude, or Codex compatibility cells.

## Probe Result

The reader probe used the frozen W29 model
`grok-composer-2.5-fast` under a one-shot LaunchAgent with the exact no-memory,
no-search, no-plan, no-subagent, one-turn, and sentinel tool-deny boundary.

| Gate | Result |
|---|---|
| LaunchAgent runs | `1` |
| Last exit code | `1` |
| Provider calls reaching a model answer | `0` |
| Failure class | `provider_auth_unavailable` |
| Probe output SHA-256 | `70e7bcf2f16b2e8cf01b58b8f46ebc65de3c578c5844da5ac19a3719a4af8945` |
| Probe stderr SHA-256 | `ed2e0acba42dba1ccb342f5d9e88f67ea45ef9b5f327308c627553f13f71f650` |
| Probe debug SHA-256 | `7c0254da7e55922ab46b0b22c7df61f64794188fd09621fe09471f7a154b99e4` |

The provider rejected the current OAuth state before generating an answer.
The required judge probe and the 1,000 reader plus 1,000 judge dataset calls
were therefore not started. The formal W29 artifact root remained empty.

## Decision

- Retain the probe as an unavailable-provider result.
- Do not resume it into a successful run.
- Keep the W29 Grok execution open until fresh authentication is available.
- Evaluate the same frozen lexical/vector contexts through separately named
  direct-provider executions without treating them as W29 substitutes.

## Non-Claims

- This result says nothing about lexical versus vector answer quality.
- It is not a Grok model-quality result.
- It does not invalidate W28 retrieval evidence.
- A SiliconFlow, Cursor, Codex, mock, or other run cannot complete W29.
