DO $$
BEGIN
  INSERT INTO continuity_spaces (id, tenant_id, continuity_line, state) VALUES
    ('00000000-0000-0000-0000-000000000001', 'local', 'workspace', 'active'),
    ('00000000-0000-0000-0000-000000000002', 'local', 'workspace', 'active');

  INSERT INTO continuity_bindings (id, continuity_id, tenant_id, repo_root, binding_state) VALUES
    ('00000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000001', 'local', '/fixtures/web-checkout', 'confirmed'),
    ('00000000-0000-0000-0000-000000000032', '00000000-0000-0000-0000-000000000002', 'local', '/fixtures/ops-console', 'confirmed');

  INSERT INTO observations (id, tenant_id, continuity_id, operation_id, observation_kind, content, source_ref) VALUES
    ('00000000-0000-0000-0000-000000000011', 'local', '00000000-0000-0000-0000-000000000001', 'w02-source-v1', 'source_update', 'Use checkout_eta_v1 for the staged checkout release.', 'fixture:W02:release-notes-v1'),
    ('00000000-0000-0000-0000-000000000012', 'local', '00000000-0000-0000-0000-000000000001', 'w02-source-v2', 'source_update', 'Use checkout_eta_v2 for the staged checkout release.', 'fixture:W02:release-notes-v2'),
    ('00000000-0000-0000-0000-000000000013', 'local', '00000000-0000-0000-0000-000000000002', 'w02-ops-source', 'source_update', 'Run ops_exception_queue_refresh before handling incidents.', 'fixture:W02:ops-runbook');

  INSERT INTO governed_memories (id, tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content, supersedes_memory_id) VALUES
    ('00000000-0000-0000-0000-000000000021', 'local', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000011', 'fact', 'superseded', 'Use checkout_eta_v1 for the staged checkout release.', NULL),
    ('00000000-0000-0000-0000-000000000022', 'local', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000012', 'fact', 'active', 'Use checkout_eta_v2 for the staged checkout release.', '00000000-0000-0000-0000-000000000021'),
    ('00000000-0000-0000-0000-000000000023', 'local', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000013', 'fact', 'active', 'Run ops_exception_queue_refresh before handling incidents.', NULL);
END $$;
