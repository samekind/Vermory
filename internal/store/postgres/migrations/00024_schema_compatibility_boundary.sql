-- +goose Up

-- +goose StatementBegin
CREATE FUNCTION vermory_auth.schema_version()
RETURNS BIGINT
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
  SELECT COALESCE(max(version_id) FILTER (WHERE is_applied), 0)
  FROM public.goose_db_version
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION vermory_auth.schema_version() FROM PUBLIC;

-- +goose Down

DROP FUNCTION vermory_auth.schema_version();
