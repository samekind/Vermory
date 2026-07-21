# OpenClaw Bundled Plugin Root Workaround

## Scope

This evidence covers the OpenClaw `2026.6.11` runtime used by the Vermory
integration. It addresses a repeated diagnostic warning about missing generated
channel setup modules when no channel is configured. It does not claim that
iMessage or Telegram are configured or qualified.

The change is deliberately outside the OpenClaw package. The Vermory macOS
installer now exports `OPENCLAW_BUNDLED_PLUGINS_DIR` from the generated wrapper
to the `dist/extensions` tree shipped by the same OpenClaw installation.

## Reproduction

The warning reproduced in a clean state with only `HOME`, `PATH`,
`OPENCLAW_STATE_DIR`, and `OPENCLAW_CONFIG_PATH` set:

```text
[channels] failed to load bundled channel setup entry imessage: missing generated module for bundled channel imessage
[channels] failed to load bundled channel setup entry telegram: missing generated module for bundled channel telegram
```

The command still exited successfully and returned the same channel summary:

```json
{"chat":{}}
```

The same result was reproduced on the ARM64 Mac mini. Setting the package-local
bundled plugin root removed both warnings while preserving the JSON result.

The behavior is consistent with the still-open upstream issue
[openclaw/openclaw#86039](https://github.com/openclaw/openclaw/issues/86039).
OpenClaw maintainers have not accepted a narrow source-level filter as the
canonical fix, so Vermory does not patch or replace OpenClaw files.

## Source And Artifact Identity

The implementation was committed as:

```text
307de4b2a5b7de4e36d2ed17bb871178f8533c18
```

Protected CI run `29863809191` passed both `test` and `sign-snapshot`.
The signed artifact was:

```text
artifact: vermory-pr-snapshot-307de4b2a5b7de4e36d2ed17bb871178f8533c18
artifact id: 8508506446
artifact sha256: 290f4fad161673fb3fc92c99bba8efd2eb96fbe31b5234207fff6de9f0bfd5c3
OpenClaw tgz sha256: 46ba8fec85ac83b09af5177dde0e9cacd35c25475e537cfa8d8f31f4a61f4b14
```

The release manifest was verified locally and the Sigstore bundle verified
with this identity:

```text
https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge
```

## Mac Mini Verification

The existing user-level OpenClaw installation was upgraded by the exact-head
installer without `sudo` and without Mac mini NewAPI. The loopback gateway
changed process identity:

```text
before: 15132
after: 30903
```

The generated wrapper contains the package-local root:

```text
OPENCLAW_BUNDLED_PLUGINS_DIR=$HOME/Library/Application Support/Vermory/openclaw-plugin/node_modules/openclaw/dist/extensions
```

Post-install checks passed:

- `channels list --json` produced no stderr warning and returned `{"chat":{}}`;
- `gateway health --json` returned `ok: true`;
- loaded gateway plugins were `memory-core` and `vermory`;
- gateway plugin errors were empty;
- Vermory runtime status was `loaded`;
- Vermory reported three typed hooks: `after_tool_call`, `agent_end`, and `before_prompt_build`;
- the live Vermory `dist/index.js` matched the signed package payload:
  `bdcd3e53b06bdde4ae42e834a861ada27f4a6139228608e3a500f6936183cb6f`.

## Non-Claims

- This does not qualify any real iMessage or Telegram transport.
- This does not repair OpenClaw upstream source code.
- This does not establish a public or non-loopback OpenClaw deployment.
- This does not use a provider model or an LLM judge; it verifies runtime
  loading, process lifecycle, diagnostics, and hook registration only.
