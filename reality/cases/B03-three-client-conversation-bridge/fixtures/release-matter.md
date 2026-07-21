# Three-client release matter

The Web Chat conversation confirms that the current release bundle is
`vermory-v8.tgz`.

The Web Chat thread also contains `WEB_CHAT_RAW_ONLY`, which is local chatter
and must not be copied into Hermes or OpenClaw context. A separate Web Chat
thread contains `UNRELATED_RELEASE_MATTER` and must remain isolated.

Hermes and OpenClaw use separate session keys even though their human-facing
matter name is the same. The only allowed cross-client connection is an
explicit Vermory bridge.
