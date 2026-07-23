-- +goose Up
CREATE TABLE conversation_turns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  operation_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('in_progress', 'completed', 'failed')),
  user_observation_id UUID NOT NULL REFERENCES observations(id),
  delivery_id UUID REFERENCES memory_deliveries(id),
  assistant_observation_id UUID REFERENCES observations(id),
  answer TEXT NOT NULL DEFAULT '',
  provider_model TEXT NOT NULL DEFAULT '',
  failure_code TEXT NOT NULL DEFAULT '',
  failure_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);

CREATE INDEX conversation_turns_continuity_idx
  ON conversation_turns (tenant_id, continuity_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS conversation_turns;
