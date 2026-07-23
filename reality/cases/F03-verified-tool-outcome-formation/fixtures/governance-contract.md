# Verified Tool Outcome Governance Contract

1. Capture only successful results from an explicitly configured OpenClaw tool
   allowlist.
2. Bind each result to the exact prepared turn, session, run, tool name, and
   tool call identifier.
3. Persist a bounded semantic result excerpt, never raw tool parameters or an
   unbounded result object.
4. Reject result text containing credential-like assignments before it reaches
   PostgreSQL, logs, provider input, review output, or artifacts.
5. Failed tools, unallowed tools, ordinary assistant text, and cross-session
   results are not eligible formation evidence.
6. Duplicate tool-result delivery is idempotent and cannot create another
   observation, schedule advance, provider call, or candidate.
7. Successful tool results are lower-authority observations. A model may form
   a reviewable candidate from an exact result quote, but it cannot activate,
   reject, correct, delete, bridge, or promote that memory.
8. Review output identifies the source as a tool result and names the tool,
   without exposing raw parameters, provider internals, or unrelated output.
9. Accepting the diagnostic, successful removal, and storage candidates makes
   them eligible for later delivery. The false assistant claim remains absent.
10. Forgetting one accepted tool-origin memory redacts its source observation,
    candidate evidence, projections, review output, and later deliveries within
    Vermory's covered surfaces.
