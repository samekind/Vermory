-- +goose Up

ALTER TABLE governed_memories
  ADD COLUMN valid_from TIMESTAMPTZ,
  ADD COLUMN valid_until TIMESTAMPTZ,
  ADD CONSTRAINT governed_memories_validity_interval_check
    CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_until > valid_from);

ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_lifecycle_status_check,
  ADD CONSTRAINT governed_memories_lifecycle_status_check
    CHECK (lifecycle_status IN (
      'proposed', 'active', 'superseded', 'rejected', 'archived', 'deleted'
    ));

ALTER TABLE memory_deliveries
  ADD COLUMN eligibility_as_of TIMESTAMPTZ;

UPDATE memory_deliveries
SET eligibility_as_of = created_at
WHERE eligibility_as_of IS NULL;

ALTER TABLE memory_deliveries
  ALTER COLUMN eligibility_as_of SET DEFAULT clock_timestamp(),
  ALTER COLUMN eligibility_as_of SET NOT NULL;

ALTER TABLE memory_retrieval_runs
  ADD COLUMN eligibility_as_of TIMESTAMPTZ;

UPDATE memory_retrieval_runs
SET eligibility_as_of = created_at
WHERE eligibility_as_of IS NULL;

ALTER TABLE memory_retrieval_runs
  ALTER COLUMN eligibility_as_of SET DEFAULT clock_timestamp(),
  ALTER COLUMN eligibility_as_of SET NOT NULL;

CREATE FUNCTION memory_is_eligible(
  lifecycle TEXT,
  content TEXT,
  valid_from TIMESTAMPTZ,
  valid_until TIMESTAMPTZ,
  as_of TIMESTAMPTZ
)
RETURNS BOOLEAN
LANGUAGE SQL
IMMUTABLE
PARALLEL SAFE
AS $$
  SELECT lifecycle = 'active'
    AND content <> '[redacted]'
    AND (valid_from IS NULL OR as_of >= valid_from)
    AND (valid_until IS NULL OR as_of < valid_until)
$$;

CREATE TABLE memory_eligibility_operations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  operation_id TEXT NOT NULL
    CHECK (btrim(operation_id) <> '' AND octet_length(operation_id) <= 128),
  action TEXT NOT NULL CHECK (action IN ('set_validity', 'archive')),
  request_fingerprint TEXT NOT NULL
    CHECK (request_fingerprint ~ '^[0-9a-f]{64}$'),
  previous_lifecycle_status TEXT NOT NULL CHECK (previous_lifecycle_status IN (
    'proposed', 'active', 'superseded', 'rejected', 'archived', 'deleted'
  )),
  previous_valid_from TIMESTAMPTZ,
  previous_valid_until TIMESTAMPTZ,
  result_lifecycle_status TEXT NOT NULL CHECK (result_lifecycle_status IN (
    'proposed', 'active', 'superseded', 'rejected', 'archived', 'deleted'
  )),
  result_valid_from TIMESTAMPTZ,
  result_valid_until TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id),
  CONSTRAINT memory_eligibility_operations_previous_interval_check
    CHECK (
      previous_valid_from IS NULL OR previous_valid_until IS NULL
      OR previous_valid_until > previous_valid_from
    ),
  CONSTRAINT memory_eligibility_operations_result_interval_check
    CHECK (
      result_valid_from IS NULL OR result_valid_until IS NULL
      OR result_valid_until > result_valid_from
    ),
  CONSTRAINT memory_eligibility_operations_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT memory_eligibility_operations_tenant_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE CASCADE
);

CREATE INDEX memory_eligibility_operations_tenant_memory_idx
  ON memory_eligibility_operations (tenant_id, continuity_id, memory_id, created_at DESC, id);

ALTER TABLE memory_eligibility_operations ENABLE ROW LEVEL SECURITY;

CREATE POLICY memory_eligibility_operations_tenant_isolation
  ON memory_eligibility_operations
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

REVOKE ALL ON TABLE memory_eligibility_operations FROM PUBLIC;

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM governed_memories
    WHERE lifecycle_status = 'archived'
       OR valid_from IS NOT NULL
       OR valid_until IS NOT NULL
  ) OR EXISTS (
    SELECT 1 FROM memory_eligibility_operations
  ) THEN
    RAISE EXCEPTION 'cannot downgrade while memory eligibility state exists';
  END IF;
END
$$;
-- +goose StatementEnd

DROP POLICY IF EXISTS memory_eligibility_operations_tenant_isolation
  ON memory_eligibility_operations;
DROP TABLE IF EXISTS memory_eligibility_operations;
DROP FUNCTION IF EXISTS memory_is_eligible(
  TEXT, TEXT, TIMESTAMPTZ, TIMESTAMPTZ, TIMESTAMPTZ
);

ALTER TABLE memory_retrieval_runs
  DROP COLUMN eligibility_as_of;

ALTER TABLE memory_deliveries
  DROP COLUMN eligibility_as_of;

ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_lifecycle_status_check,
  ADD CONSTRAINT governed_memories_lifecycle_status_check
    CHECK (lifecycle_status IN ('proposed', 'active', 'superseded', 'rejected', 'deleted')),
  DROP CONSTRAINT governed_memories_validity_interval_check,
  DROP COLUMN valid_until,
  DROP COLUMN valid_from;
