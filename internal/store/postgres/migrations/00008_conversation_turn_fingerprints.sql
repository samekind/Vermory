-- +goose Up
ALTER TABLE conversation_turns
  ADD COLUMN request_fingerprint TEXT NOT NULL DEFAULT '',
  ADD COLUMN answer_fingerprint TEXT NOT NULL DEFAULT '';

UPDATE conversation_turns turns
SET request_fingerprint = encode(digest(convert_to(observations.content, 'UTF8'), 'sha256'), 'hex')
FROM observations
WHERE observations.id = turns.user_observation_id;

UPDATE conversation_turns
SET answer_fingerprint = encode(digest(convert_to(answer, 'UTF8'), 'sha256'), 'hex')
WHERE answer <> '' AND answer <> '[redacted]';

ALTER TABLE conversation_turns
  ADD CONSTRAINT conversation_turns_request_fingerprint_check
  CHECK (length(request_fingerprint) = 64),
  ADD CONSTRAINT conversation_turns_answer_fingerprint_check
  CHECK (answer_fingerprint = '' OR length(answer_fingerprint) = 64);

-- +goose Down
ALTER TABLE conversation_turns
  DROP CONSTRAINT IF EXISTS conversation_turns_answer_fingerprint_check,
  DROP CONSTRAINT IF EXISTS conversation_turns_request_fingerprint_check,
  DROP COLUMN IF EXISTS answer_fingerprint,
  DROP COLUMN IF EXISTS request_fingerprint;
