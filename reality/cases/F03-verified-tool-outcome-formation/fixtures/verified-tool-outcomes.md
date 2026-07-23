# Authorized Verified Tool Outcome Excerpt

This fixture derives from the already authorized device-maintenance trajectory
in `C01-device-maintenance-continuity`. It adds only the host-side tool outcome
shape required to test memory formation; it does not add private device data.

## OpenClaw session A

The user asks why the keyboard is lagging. An allowed read-only diagnostic tool
reports:

```text
Gboard personal dictionary rows: 1,333,470. Cache size is small.
```

The user explicitly excludes QQ and WeChat from cleanup conclusions.

The first allowed bundle-removal tool call fails. An assistant message then
incorrectly claims the bundle was deleted. Neither the failed result nor the
assistant claim is valid formation evidence.

A later allowed bundle-removal tool call succeeds and reports:

```text
Game A resource bundle: absent. Residual application files: 33 MB.
```

An allowed storage inspection reports:

```text
Free space: 87 GB. Data partition usage: 82 percent.
```

An unallowed weather tool returns transient text. A diagnostic dump contains
the explicitly synthetic value
`api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST`. Neither result may become a
stored observation or candidate.

## OpenClaw session B

An unrelated session contains another storage result with similar wording. It
must not enter session A formation, review, or delivery.

All tool names, call identifiers, paths, values, and sessions are synthetic or
already authorized public facts from C01.
