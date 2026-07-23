# Browser Web Chat Lifecycle Qualification

Date: 2026-07-22

Runtime case: `W32-browser-webchat-lifecycle`

Status: `20 / 20 PASS`

## Boundary Exercised

W32 qualifies Vermory's loopback Web Chat as a real browser application rather
than only an HTTP handler. The accepted run used Google Chrome 150 against the
same-origin application served by `vermory web-chat`, backed by a dedicated
PostgreSQL 17 database migrated from schema 1 through 23.

The provider was the repository's deterministic provider with model label
`w32-browser-contract`. That choice isolates browser, continuity, idempotency,
and governance behavior. It is not presented as a model-quality run; existing
Grok and DeepSeek evidence covers real-model consumption separately.

The complete machine-readable result is retained in the
[W32 lifecycle snapshot](snapshots/2026-07-22-browser-webchat-lifecycle.json).

## Accepted Browser Trajectory

The Chrome run exercised this user-visible flow:

```text
load same-origin application
-> create a conversation
-> send a turn
-> receive one assistant answer
-> remember the user observation
-> send a fresh turn and consume that current memory
-> refresh
-> recover the same thread, 2 user turns, 2 assistant turns, and 1 memory

create another conversation
-> send an unrelated travel-planning turn
-> receive no backup memory
-> switch back
-> recover the original conversation and its 2 memories

review deterministic formation candidates
-> accept one durable schedule
-> reject one temporary instruction
-> correct the drive serial from W32-ALPHA to W32-BETA
-> forget the corrected drive-serial memory
-> send a fresh turn
-> receive the retained Sunday schedule
-> do not receive W32-BETA
```

The browser exposed no tenant ID, continuity ID, memory ID, observation ID,
provider key, or database field in the normal interface. Those identifiers
remained server-side protocol data.

## Retry And Replay

Two distinct failure shapes were accepted.

### Service unavailable before receipt

The already-loaded page stayed open while the Web Chat process was stopped.
Chrome recorded the first `POST /v1/chat/turn` as
`net::ERR_CONNECTION_REFUSED`. Before issuing that request, the page had
persisted one pending message and its operation ID. The failed state contained
one user row, zero assistant rows, an explicit connection failure, and a
`Retry` action.

After the process restarted, Retry sent the same message with the exact same
operation ID. The request completed with one user observation and one assistant
observation; no phantom assistant had been created during failure.

### Response lost after server completion

Chrome then injected a one-time response-loss fault after a successful
`POST /v1/chat/turn` had reached the server. The application retained the
original operation ID and showed a recoverable failed state. Retry sent an
identical request body.

The first server response reported `replayed:false`; the retry reported
`replayed:true`. Both responses returned identical turn, delivery, user
observation, and assistant observation IDs. PostgreSQL contained one turn row,
one user observation, and one assistant observation for the operation. The
rendered result contained two total user rows and two total assistant rows for
the two intentional turns, not a duplicated fifth row.

## Responsive And Security Checks

The application was visually and structurally inspected at:

- desktop `1440x900`;
- mobile `390x844` with device scale factor 2 and touch emulation.

At desktop width, conversations, transcript, composer, and memory review stay
within one viewport. The transcript and side rails scroll internally; the
composer does not move below the viewport when history grows. At mobile width,
Chat, Conversations, and Memory are separate tab views, and the composer
remains visible without horizontal overflow.

The browser loaded HTML, CSS, and JavaScript from the same origin. The handler
sets a restrictive Content Security Policy, disables cross-origin referrers,
and enables `nosniff`. The final clean reload produced no console errors or
warnings. A final mobile Lighthouse snapshot scored 100 for accessibility,
best practices, SEO, and agentic browsing, with 29 checks passed and zero
failed. Those scores qualify page structure and accessibility only; they are
not performance or model-quality measurements.

## Database Assertions

The final PostgreSQL snapshot recorded:

| Assertion | Result |
|---|---:|
| Conversation continuities | 3 |
| Rows for the response-loss operation | 1 |
| Distinct user observations for that operation | 1 |
| Distinct assistant observations for that operation | 1 |
| Active memories after forgetting | 1 |
| Deleted memories | 1 |
| Rejected candidates | 1 |
| Fresh context contains `W32-BETA` | false |
| Fresh context contains `Sunday at 02:00` | true |

## Preserved Failures

Six implementation failures were found and corrected before acceptance:

1. A brand-new local thread initially called inspection routes before the
   server-side continuity existed. Their valid `404` responses were incorrectly
   rendered as service unavailability. The UI now keeps a new thread local
   until its first successful turn.
2. JavaScript properties ending in uppercase `ID` produced attributes such as
   `data-observation-i-d`, so the first confirmation omitted its observation ID
   and received `400`. All dataset properties now use the canonical `...Id`
   mapping, with a regression test covering thread, message, observation,
   memory, and candidate attributes.
3. The initial application shell used only `min-height`. A long transcript grew
   the document to 1,349 pixels in a 900-pixel viewport and moved the composer
   below the fold. The shell now has a fixed dynamic-viewport height and
   internal scroll regions; the accepted run measured document height exactly
   900 pixels and composer bottom at 861 pixels.
4. Switching away from a failed-send thread initially left that thread's text
   in the shared composer element. Server continuity remained isolated, but a
   user could have submitted the stale draft into another thread. Thread
   selection now restores only the target thread's own pending message and
   otherwise clears the composer.
5. A completed asynchronous send initially cleared the shared composer even if
   the user had switched to another thread while the request was running; the
   same issue could render an old thread's failure state in the new thread.
   Completion and failure updates now mutate the originating thread and update
   the visible transcript or composer only while that thread remains active.
6. Conversation inspection initially had no request generation guard. Rapidly
   switching from thread A to thread B could let a slower response for A arrive
   last and replace B's visible transcript and memory. Every refresh now carries
   both its originating thread and a monotonic request generation; stale success
   and failure responses are discarded before they mutate visible state.

Chrome DevTools' full Offline emulation navigated the inspected tab to Chrome's
network error page, so it was rejected as a send-lifecycle setup. The accepted
network-failure gate instead stopped the loopback service after the application
was loaded, producing a real refused request while preserving the page state.

## Claim Boundary

W32 qualifies the real Chrome browser lifecycle, same-origin delivery,
conversation isolation, refresh recovery, persisted operation retry, replay
deduplication, candidate review, correction, forgetting, responsive layout,
and fresh-turn deletion behavior for the loopback Web Chat application.

It does not claim:

- real-model quality from the deterministic W32 provider;
- a publicly exposed or multi-user browser deployment;
- browser support beyond the qualified Chrome version;
- that browser local storage is memory authority;
- that a browser transcript is erased when a governed memory is forgotten.

PostgreSQL remains the memory authority. Browser storage contains only thread
labels, selection, and a pending operation needed to recover uncertain sends.
