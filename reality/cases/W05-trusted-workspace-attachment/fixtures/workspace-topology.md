# Trusted Workspace Attachment Topology

This synthetic topology represents two filesystem namespaces. Paths are
deliberately reusable across hosts so path text alone cannot identify a
workspace globally.

In namespace `workstation-alpha`, `/work/Vermory` is the governed primary Git
checkout. `/work/Vermory/internal/runtime` is a nested working directory inside
that checkout. Codex, Grok, and Cursor all operate on this same checkout.

`/archive/Vermory` is an unrelated repository with the same basename.
`/worktrees/Vermory-resolver` is a Git worktree of the primary checkout, but it
is not an alias until a trusted operator explicitly adopts it.

The primary checkout later moves from `/work/Vermory` to `/srv/Vermory`. The new
path is not current until a trusted operator explicitly rebinds the continuity.

`/mirrors/Vermory` has a matching remote URL but remains ambiguous because a
remote URL, repository title, or semantic similarity is not binding authority.

In namespace `workstation-beta`, `/work/Vermory` is unrelated to the alpha
checkout even though the path text and basename are identical.

The Vermory MCP server runs on a separate host and cannot inspect either
workstation filesystem. A trusted workstation-side probe must resolve the Git
root and provide a bounded attachment when the MCP process starts. The model
cannot provide or replace that attachment.
