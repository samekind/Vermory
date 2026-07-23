# Authorized Automatic Review Governance

1. Complete the first OpenClaw turn through the ordinary integration path.
2. Enqueue one durable request without calling or waiting for the formation
   provider in the completion request.
3. Run the fixed-tenant worker and form reviewable bundle, deadline, and office
   candidates from the exact user observation.
4. Verify no candidate is active before review.
5. List pending candidates through the direct OpenClaw `/vermory memories`
   command using a separate operator token.
6. Accept the bundle and deadline and reject the office candidate.
7. Complete the correction turn, run the worker, and accept one deadline
   update. Verify Tuesday is superseded by Wednesday at 12:00.
8. Verify a fresh OpenClaw turn receives the bundle and Wednesday deadline but
   not Tuesday or B-412.
9. Forget the deadline through `/vermory forget` and verify it disappears from
   review, delivery, search, history, audit payload, and projections covered by
   Vermory.
10. Complete one separate Hermes turn, run its schedule, and verify its raw
    observation and pending candidate remain isolated from OpenClaw session A.
11. Replay completion and require one logical schedule with no extra provider
    call.
12. Stop the worker during provider execution or return provider failure;
    require retryable state and no processed-cursor advance.
13. Verify client tokens cannot list or govern candidates and the model has no
    governance tool.
