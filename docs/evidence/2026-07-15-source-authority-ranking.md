# Source Authority Ranking Evidence

Date: 2026-07-15

Vermory now applies one explicit source-authority tie-break across workspace
lexical retrieval, linked conversation lexical retrieval, and profile-scoped
vector retrieval:

| Origin | Rank |
|---|---:|
| `user_correction` / `user_confirmation` | 4 |
| `source_update` | 3 |
| `bridge_promote` | 2 |
| other governed origin | 1 |

Relevance remains the primary ordering signal. Authority is evaluated after
exact match, full-text rank, and similarity/distance, so a less relevant user
correction does not automatically displace an unrelated result. When two
active facts have the same retrieval relevance, explicit user correction wins;
the decision is explainable from the origin observation and does not require a
model judge.

The PostgreSQL runtime tests create two active facts with identical content and
query relevance, one from a trusted source update and one from an explicit user
correction. Both lexical and vector retrieval return the correction first.
The test also verifies the result remains tenant/continuity scoped.

This is a deterministic authority policy, not a claim that source content is
factually true. Conflicting facts still require the existing correction,
candidate, or operator-governance flows; authority ranking does not auto-accept
model output or bypass lifecycle controls.
