# Tenant isolation fixture

## Shared anchor pressure

Both tenants use the exact OpenClaw session key `agent:main:shared-anchor`. The identity-a continuity contains governed fact `TENANT-A-ONLY`. The identity-b continuity must remain independent despite matching channel and thread values.

## Authorization pressure

- client role may prepare and complete turns;
- client role may not inspect or mutate governed memory, Global Defaults, or bridges;
- operator role may perform explicit governance inside its own tenant;
- request JSON may not select tenant or role.

## PostgreSQL pressure

- omit tenant predicates under identity-a RLS context and observe only identity-a rows;
- repeat under identity-b and observe only identity-b rows;
- clear tenant context and observe no tenant rows or a fail-closed query;
- attempt identity-b foreign keys to identity-a continuity, observation, memory, delivery, and bridge IDs;
- reuse pooled connections in A/B/A/B order and concurrently without stale context.

The runtime role is non-owner, non-superuser, and does not have `BYPASSRLS`. It receives no privileges on legacy project/source/capsule tables.
