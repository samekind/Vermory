# Governance actions

1. Confirm the current visit time and concierge check-in requirement from session A.
2. Confirm the temporary access code only long enough to exercise deletion semantics.
3. Restart Vermory and OpenClaw before the first recall probe.
4. Link session B to session A explicitly; do not infer the link from similar wording.
5. Keep unrelated session C isolated.
6. Correct the appointment from Friday 15:30 to Saturday 10:00 and supersede the old time.
7. Delete `CEDAR-4826`, rebuild the projection, and verify exact, paraphrased, and related probes cannot retrieve it from fresh Vermory deliveries.
8. Set a thin Global Default for Chinese user-facing replies. A one-turn English request overrides it only for that turn.
9. Reverse the A-B link and verify future cross-session governed-memory retrieval stops without claiming that OpenClaw erased its own historical transcript.
