DROP INDEX IF EXISTS idx_agent_task_queue_deferred_until;
DROP INDEX IF EXISTS idx_agent_task_queue_governance_active;

ALTER TABLE agent_task_queue
  DROP COLUMN IF EXISTS superseded_by_task_id,
  DROP COLUMN IF EXISTS deferred_until,
  DROP COLUMN IF EXISTS failure_class,
  DROP COLUMN IF EXISTS lifecycle_class,
  DROP COLUMN IF EXISTS tool_profile,
  DROP COLUMN IF EXISTS governance_key,
  DROP COLUMN IF EXISTS launch_scope;
