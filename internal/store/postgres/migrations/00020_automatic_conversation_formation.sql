-- +goose Up
CREATE TABLE conversation_formation_schedules (
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  requested_through_sequence BIGINT NOT NULL DEFAULT 0 CHECK (requested_through_sequence >= 0),
  processed_through_sequence BIGINT NOT NULL DEFAULT 0 CHECK (processed_through_sequence >= 0),
  schedule_state TEXT NOT NULL DEFAULT 'idle'
    CHECK (schedule_state IN ('idle', 'pending', 'running', 'retry_wait')),
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_attempt_at TIMESTAMPTZ,
  window_start_sequence BIGINT,
  window_end_sequence BIGINT,
  window_observation_ids JSONB NOT NULL DEFAULT '[]'::jsonb
    CHECK (jsonb_typeof(window_observation_ids) = 'array'),
  window_fingerprint TEXT NOT NULL DEFAULT ''
    CHECK (window_fingerprint = '' OR window_fingerprint ~ '^[0-9a-f]{64}$'),
  active_operation_id TEXT NOT NULL DEFAULT '' CHECK (octet_length(active_operation_id) <= 512),
  last_run_id UUID,
  last_status TEXT NOT NULL DEFAULT ''
    CHECK (last_status IN ('', 'completed', 'abstained', 'failed')),
  last_failure_code TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_failure_code) <= 128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, continuity_id),
  CONSTRAINT conversation_formation_schedules_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT conversation_formation_schedules_tenant_run_fk
    FOREIGN KEY (tenant_id, continuity_id, last_run_id)
    REFERENCES source_formation_runs (tenant_id, continuity_id, id)
    ON DELETE SET NULL (last_run_id),
  CHECK (processed_through_sequence <= requested_through_sequence),
  CHECK (
    (schedule_state = 'running'
      AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL
      AND window_start_sequence IS NOT NULL
      AND window_end_sequence IS NOT NULL
      AND window_start_sequence <= window_end_sequence
      AND jsonb_array_length(window_observation_ids) > 0
      AND length(window_fingerprint) = 64
      AND btrim(active_operation_id) <> '')
    OR
    (schedule_state <> 'running'
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND window_start_sequence IS NULL
      AND window_end_sequence IS NULL
      AND window_observation_ids = '[]'::jsonb
      AND window_fingerprint = ''
      AND active_operation_id = '')
  ),
  CHECK (
    (schedule_state = 'idle'
      AND processed_through_sequence = requested_through_sequence
      AND next_attempt_at IS NULL)
    OR
    (schedule_state = 'pending'
      AND processed_through_sequence < requested_through_sequence
      AND next_attempt_at IS NOT NULL)
    OR
    (schedule_state = 'running'
      AND processed_through_sequence < requested_through_sequence
      AND next_attempt_at IS NULL)
    OR
    (schedule_state = 'retry_wait'
      AND processed_through_sequence < requested_through_sequence
      AND next_attempt_at IS NOT NULL)
  )
);

CREATE INDEX conversation_formation_schedules_claim_idx
  ON conversation_formation_schedules (tenant_id, next_attempt_at, updated_at)
  WHERE schedule_state IN ('pending', 'retry_wait', 'running');

ALTER TABLE conversation_formation_schedules ENABLE ROW LEVEL SECURITY;

CREATE POLICY conversation_formation_schedules_tenant_isolation
  ON conversation_formation_schedules
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS conversation_formation_schedules_tenant_isolation
  ON conversation_formation_schedules;
DROP TABLE IF EXISTS conversation_formation_schedules;
