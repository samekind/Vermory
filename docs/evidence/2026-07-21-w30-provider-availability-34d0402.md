# W30 Exact-Head Provider Availability Check

Date: 2026-07-21 Asia/Shanghai

Status: both frozen direct SiliconFlow models rejected the check because the
provider account balance was insufficient; W30 v2 was not started

## Purpose

The W30 reader/judge reliability changes produced a new exact-head runtime.
The W30 contract requires that runtime to prove both frozen model routes before
it may create a fresh 1,000-task reader run. This check measures route
availability only. It is not an answer-quality or retrieval-quality run.

## Frozen Runtime

| Field | Value |
|---|---|
| Implementation revision | `34d04025c146b0ae83ef42956a9e557838fc0707` |
| Runtime version | `0.0.0-SNAPSHOT-34d0402` |
| Binary SHA-256 | `976d1e799a22aee572f8a784a3cd4f1ab1b61462a3910e6513f4f43ad5650191` |
| Source archive SHA-256 | `02fc191436c6679622c8e7dc3d45096241b593eee92d5d1447e99bbce3c6358c` |
| Host architecture | macOS ARM64 |
| LaunchAgent label | `org.vermory.w30.probe-final.34d0402` |
| Probe run ID | `w30-siliconflow-reader-judge-probe-20260721-final-v2` |
| Provider route | direct `https://api.siliconflow.cn/v1` |
| Reader model | `deepseek-ai/DeepSeek-V4-Flash` |
| Judge model | `Qwen/Qwen3-30B-A3B-Instruct-2507` |

The one-shot GUI LaunchAgent used the exact-head binary and the repository
Keychain wrapper. The credential did not enter the plist, command line,
report, logs, or Git. The request did not pass through NewAPI.

## Observed Result

The LaunchAgent ran once. Both model calls reached the direct provider and
returned the same response:

```text
403 Forbidden
code: 30001
message: Sorry, your account balance is insufficient
```

| Artifact | SHA-256 |
|---|---|
| Probe `report.json` | `73069055497e4058f27c45dd34a53bba74b87fce8aba568ba7d18652ace07f9b` |
| Probe `report.md` | `dc1f9308283a9e4692a1eb0ee039fc9384331d0532da5c9dd69275d470ead6c9` |
| LaunchAgent plist | `e1282802f1cf3bb531d3c80ceb22fa269d177384cccaf4387eaedc7282a5962d` |

The retained report correctly marked both per-model results as `error`. The
then-current `probe-provider` command nevertheless exited `0` after writing
the report. That exit-status defect is treated as a separate automation
failure and is covered by a regression test: any failed model now makes the
CLI return nonzero after preserving its report.

## Post-Fix Cross-Host Verification

Revision `e1449c36e9aaefe99d1ab3b47945902333637ccc` implemented the CLI
failure boundary and passed both protected pull-request jobs in CI run
`29855114425`. The signed snapshot was downloaded locally, verified, and then
transferred to the Mac mini over the managed FRP SSH route. The Mac mini did
not download or rebuild the payload.

| Artifact or runtime | SHA-256 / result |
|---|---|
| Signed release manifest | `79d6bb3678b84a79684a5553edd02dd0f4b7a83cec880c02eb85690297d89b50` |
| Sigstore bundle | `18a14e8f403f45e35ad5607732cb5eed391e0e4034cf533b6da69cdf068694dc` |
| Sigstore verification | `Verified OK` for the PR CI workflow identity and GitHub Actions issuer |
| Darwin ARM64 archive | `f6be94809027be01417e660e33612c12b833cefc848ff498f693a3dfb9ee772a` |
| Extracted ARM64 binary | `2d0c4baf06c4dfc4633bf336c55e83c9b0afc6fe75fed998d3c302cc03f7bd9a` |
| Exact source archive | `7f795d06d91b0a1ce2563acf0a283b4d3a1d3d6dec4e6bf7e5d128f89425bbe6` |
| Runtime identity | `0.0.0-SNAPSHOT-e1449c3`, revision `e1449c36e9aaefe99d1ab3b47945902333637ccc` |

The new one-shot LaunchAgent used label
`org.vermory.w30.probe-final.e1449c3` and run ID
`w30-siliconflow-reader-judge-probe-20260721-final-v3`. It ran once and
terminated with exit code `1`. Both per-model results still contained the same
provider-account `403` / code `30001`, while stderr contained the bounded
two-model failure summary.

| Retained post-fix artifact | SHA-256 |
|---|---|
| Probe `report.json` | `611c6527509b69b883ce3cd91b41cd030232e0578c4cfeb2ce2eb53131e5070f` |
| Probe `report.md` | `cb910cb73b34e3913b3fc9c14c8eae5fc9798d5fca647796355c28367be8c3bc` |
| Launch stdout | `cd3fa06d3cf543e6bd6843cff42e4deafc32122880b5a86d1b39ed9511e7134a` |
| Launch stderr | `ae6cb9c765c04495548606a2e1450444206d39cb32bc416eb8f31c719266ae31` |
| LaunchAgent plist | `8a4a242287ac0969c5ef77b4f59585233d3ab2aebfb94c524fef954fa4672eec` |

The post-fix credential-shape scan found zero matching files, and the
LaunchAgent was unloaded after inspection. The retained report still carried
the historical `ContextMesh` heading; a subsequent source-only correction
changes active probe reports to the Vermory product name without changing this
runtime result.

## Decision

- Do not create or start
  `longmemeval-s-full-domestic-vector-reader-qa-20260721-v2`.
- Keep the rejected W30 v1 artifact root unchanged and do not resume it.
- Do not start the custom judge or produce a paired score.
- A later W30 attempt requires sufficient direct-provider credit, a new
  exact-head runtime, a new run ID, a new artifact root, and a new LaunchAgent
  label.
- Continue provider-independent product, client, deployment, and release
  validation while the account state remains unavailable.

## Non-Claims

- Exit `0` from the old probe command is not provider-success evidence.
- This check does not rank DeepSeek, Qwen, SiliconFlow, or any provider.
- It says nothing about lexical versus vector context utility.
- It does not complete W29, W30, or the overall Vermory qualification.
