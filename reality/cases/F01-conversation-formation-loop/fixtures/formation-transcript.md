# Authorized Conversation Formation Transcript

Conversation anchor: `openclaw / agent:main:formation-home-maintenance-a`

## Initial user observations

1. The plumbing inspection is booked for Friday at 15:30. The technician must check in with the concierge.
2. The temporary access code is CEDAR-4826. Keep it only until the visit details are finalized.
3. It may rain on Friday and I might order lunch early.

## Later user observations

4. Correction: the building moved the inspection to Saturday at 10:00. Friday at 15:30 is obsolete.
5. For this turn only, reply in English with the current visit time.

## Isolation controls

- `agent:main:formation-home-maintenance-b` may consume accepted governed memory only while an explicit bridge is active; its raw transcript is not formation input for session A.
- `agent:main:formation-unrelated-c` remains unrelated and contributes no observation to session A formation.

This fixture is derived from the authorized and anonymized O01 trajectory. It contains only synthetic names, anchors, schedules, and credentials.
