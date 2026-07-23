-- +goose Up
ALTER TABLE conversation_turns
  DROP CONSTRAINT conversation_turns_status_check,
  ADD COLUMN operation_protocol TEXT NOT NULL DEFAULT 'bounded_v1',
  ADD COLUMN attempt_id UUID,
  ADD COLUMN lease_generation BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN lease_expires_at TIMESTAMPTZ,
  ADD COLUMN last_heartbeat_at TIMESTAMPTZ,
  ADD COLUMN checkpoint_sequence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN checkpoint_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN checkpoint_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN checkpoint_updated_at TIMESTAMPTZ,
  ADD COLUMN cancellation_code TEXT NOT NULL DEFAULT '',
  ADD COLUMN cancellation_message TEXT NOT NULL DEFAULT '';

ALTER TABLE conversation_turns
  ADD CONSTRAINT conversation_turns_status_check
    CHECK (status IN ('in_progress', 'completed', 'failed', 'cancelled')),
  ADD CONSTRAINT conversation_turns_operation_protocol_check
    CHECK (operation_protocol IN ('bounded_v1', 'leased_v1')),
  ADD CONSTRAINT conversation_turns_lease_generation_check
    CHECK (lease_generation >= 0),
  ADD CONSTRAINT conversation_turns_checkpoint_sequence_check
    CHECK (checkpoint_sequence >= 0),
  ADD CONSTRAINT conversation_turns_checkpoint_payload_check
    CHECK (jsonb_typeof(checkpoint_payload) = 'object'),
  ADD CONSTRAINT conversation_turns_checkpoint_fingerprint_check
    CHECK (checkpoint_fingerprint = '' OR checkpoint_fingerprint ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT conversation_turns_protocol_state_check
    CHECK (
      (
        operation_protocol = 'bounded_v1'
        AND status <> 'cancelled'
        AND attempt_id IS NULL
        AND lease_generation = 0
        AND lease_expires_at IS NULL
        AND last_heartbeat_at IS NULL
        AND checkpoint_sequence = 0
        AND checkpoint_payload = '{}'::jsonb
        AND checkpoint_fingerprint = ''
        AND checkpoint_updated_at IS NULL
      )
      OR
      (
        operation_protocol = 'leased_v1'
        AND attempt_id IS NOT NULL
        AND lease_generation >= 1
        AND last_heartbeat_at IS NOT NULL
        AND (
          (status = 'in_progress' AND lease_expires_at IS NOT NULL)
          OR
          (status <> 'in_progress' AND lease_expires_at IS NULL)
        )
      )
    ),
  ADD CONSTRAINT conversation_turns_checkpoint_state_check
    CHECK (
      (
        checkpoint_sequence = 0
        AND checkpoint_payload = '{}'::jsonb
        AND checkpoint_fingerprint = ''
        AND checkpoint_updated_at IS NULL
      )
      OR
      (
        checkpoint_sequence > 0
        AND length(checkpoint_fingerprint) = 64
        AND checkpoint_updated_at IS NOT NULL
      )
    ),
  ADD CONSTRAINT conversation_turns_cancellation_state_check
    CHECK (
      (
        status = 'cancelled'
        AND operation_protocol = 'leased_v1'
        AND btrim(cancellation_code) <> ''
        AND octet_length(cancellation_code) <= 128
        AND octet_length(cancellation_message) <= 512
      )
      OR
      (
        status <> 'cancelled'
        AND cancellation_code = ''
        AND cancellation_message = ''
      )
    );

-- +goose Down
ALTER TABLE conversation_turns
  DROP CONSTRAINT conversation_turns_cancellation_state_check,
  DROP CONSTRAINT conversation_turns_checkpoint_state_check,
  DROP CONSTRAINT conversation_turns_protocol_state_check,
  DROP CONSTRAINT conversation_turns_checkpoint_fingerprint_check,
  DROP CONSTRAINT conversation_turns_checkpoint_payload_check,
  DROP CONSTRAINT conversation_turns_checkpoint_sequence_check,
  DROP CONSTRAINT conversation_turns_lease_generation_check,
  DROP CONSTRAINT conversation_turns_operation_protocol_check,
  DROP CONSTRAINT conversation_turns_status_check,
  DROP COLUMN cancellation_message,
  DROP COLUMN cancellation_code,
  DROP COLUMN checkpoint_updated_at,
  DROP COLUMN checkpoint_fingerprint,
  DROP COLUMN checkpoint_payload,
  DROP COLUMN checkpoint_sequence,
  DROP COLUMN last_heartbeat_at,
  DROP COLUMN lease_expires_at,
  DROP COLUMN lease_generation,
  DROP COLUMN attempt_id,
  DROP COLUMN operation_protocol,
  ADD CONSTRAINT conversation_turns_status_check
    CHECK (status IN ('in_progress', 'completed', 'failed'));
