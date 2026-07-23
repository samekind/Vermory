# Workspace Source Revision

The `release-console` workspace has two independent release facts.

An early release note used this verification command:

```text
npm run release:verify -- --legacy
```

The canonical release manifest was later revised. Its current command is:

```text
pnpm exec release:verify --mode locked
```

The canonical manifest revision replaces only the old release verification
command. It does not replace the independent API timeout requirement, which
remains `800 ms`.

The next coder must create `release-source-check.md` from current governed
context. The artifact must contain the current verification command and the
unchanged timeout. It must not contain the stale command.
