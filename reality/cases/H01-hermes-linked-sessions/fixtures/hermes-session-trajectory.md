# Hermes session trajectory

This fixture represents two independent Hermes CLI sessions used for a thesis
submission task.

Session A records that the current upload bundle is
`thesis-defense-v7.zip`. The previously discussed
`thesis-defense-v6.zip` is obsolete. The user observation is explicitly
confirmed through Vermory after Hermes completes the turn.

Session B starts with a neutral message and its own transcript contains no
thesis bundle filename. It may receive the confirmed current filename only
while an explicit Vermory conversation link connects it to session A. A third
or unrelated session must remain isolated.

After the link is reversed, a fresh direct prepare request for session B must
not receive the filename from session A. Hermes may retain text already seen in
its own transcript, so post-reversal evidence must inspect a fresh Vermory
delivery rather than asking the same model session to forget its transcript.
