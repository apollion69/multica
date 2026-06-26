-- Agent task governance metadata for issue-level launch control,
-- tool-profile routing, lifecycle classification, and follow-up
-- cooldown/defer behavior. Defaults preserve existing queue behavior
-- until the service guard starts writing stricter values.
ALTER TABLE agent_task_queue
  ADD COLUMN launch_scope TEXT NOT NULL DEFAULT 'issue'
    CHECK (launch_scope IN ('issue', 'mention', 'comment', 'chat', 'autopilot', 'manual', 'verification')),
  ADD COLUMN governance_key TEXT,
  ADD COLUMN tool_profile TEXT NOT NULL DEFAULT 'legacy_auto',
  ADD COLUMN lifecycle_class TEXT NOT NULL DEFAULT 'queued'
    CHECK (lifecycle_class IN ('queued', 'dispatched', 'running', 'succeeded', 'failed', 'cancelled', 'stale_running', 'semantic_timeout', 'provider_limited', 'superseded')),
  ADD COLUMN failure_class TEXT
    CHECK (failure_class IS NULL OR failure_class IN ('provider_limit', 'session_limit', 'tool_denied', 'tool_failed', 'timeout', 'cancelled', 'auth', 'runtime_offline', 'unknown')),
  ADD COLUMN deferred_until TIMESTAMPTZ,
  ADD COLUMN superseded_by_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL;

UPDATE agent_task_queue
SET governance_key = COALESCE(issue_id::text, chat_session_id::text, id::text),
    launch_scope = CASE
      WHEN chat_session_id IS NOT NULL THEN 'chat'
      WHEN autopilot_run_id IS NOT NULL THEN 'autopilot'
      ELSE launch_scope
    END,
    tool_profile = 'legacy_auto',
    lifecycle_class = CASE
      WHEN status = 'queued' THEN 'queued'
      WHEN status = 'dispatched' THEN 'dispatched'
      WHEN status = 'running' THEN 'running'
      WHEN status = 'completed' THEN 'succeeded'
      WHEN status = 'failed' THEN 'failed'
      WHEN status = 'cancelled' THEN 'cancelled'
      ELSE NULL
    END
WHERE governance_key IS NULL
   OR tool_profile IS NULL
   OR lifecycle_class IS NULL;

CREATE INDEX idx_agent_task_queue_governance_active
  ON agent_task_queue(governance_key, status, created_at)
  WHERE status IN ('queued', 'dispatched', 'running');

CREATE INDEX idx_agent_task_queue_deferred_until
  ON agent_task_queue(deferred_until)
  WHERE deferred_until IS NOT NULL;
