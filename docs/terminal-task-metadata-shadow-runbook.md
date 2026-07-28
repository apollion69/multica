# Terminal Task Metadata Shadow Runbook

## Scope and current state

This runbook covers the read-only Multica terminal-task metadata surface and
the disabled Day Evolve shadow consumer. It does not authorize a Multica
release or enable the consumer.

The source candidate is on branch `feat/terminal-task-metadata`:

- `726bd421f` adds the body-free API, CLI command, query, and consumer contract.
- `965df506d` regenerates the pinned sqlc output.

The live Multica service does not yet contain these commits. Shadow evidence
therefore remains `0/14` until a normal Multica release deploys the surface and
the checks below pass.

## Authoritative producer

The only producer is:

```text
GET /api/task-runs/terminal-metadata
multica issue terminal-tasks
```

The endpoint is workspace-scoped and returns terminal queue rows only:
`completed`, `failed`, or `cancelled` with a non-null `completed_at`. It orders
by `(completed_at, source_id)` and returns at most 100 records per page.

Every record has exactly these 12 fields:

| Field | Type |
|---|---|
| `attempt` | integer |
| `completed_at` | RFC3339Nano UTC string |
| `created_at` | RFC3339Nano UTC string |
| `dispatched_at` | RFC3339Nano UTC string or null |
| `is_leader_task` | boolean |
| `issue_id` | UUID |
| `kind` | `autopilot`, `chat`, `comment`, or `direct` |
| `max_attempts` | integer |
| `parent_task_id` | UUID or null |
| `source_id` | task UUID |
| `started_at` | RFC3339Nano UTC string or null |
| `status` | `completed`, `failed`, or `cancelled` |

Task text, comments, prompts, results, errors, messages, agent instructions,
metadata, labels, and attachments must never cross this boundary.

## Stable pagination

The first request omits all cursor flags:

```bash
multica issue terminal-tasks --limit 100
```

The response envelope has exactly `records`, `has_more`, `next_cursor`, and
`watermark`. The first non-empty page fixes the scan watermark. Every later
request must send both fields of the previous `next_cursor` and both fields of
that unchanged watermark:

```bash
multica issue terminal-tasks \
  --limit 100 \
  --after-completed-at "$AFTER_COMPLETED_AT" \
  --after-id "$AFTER_ID" \
  --watermark-completed-at "$WATERMARK_COMPLETED_AT" \
  --watermark-id "$WATERMARK_ID"
```

Never substitute issue timestamps or issue metadata for task lifecycle
timestamps. Reject a partial cursor, a changed watermark, a page over 100
records, a record with any extra field, a repeated `source_id` within or across
pages, or a `next_cursor` that differs from the last record.

## Shadow consumer

The existing consumer is intentionally split into two files:

- `projects/active/day-evolve/day_evolve/multica_source.py` is the disabled,
  I/O-free 12-field adapter. `MulticaSourceConfig.enabled` defaults to `False`.
- `projects/active/day-evolve/day_evolve/multica_shadow.py` is the bounded
  collector. It invokes only the CLI surface, fixes one watermark, rejects
  schema drift and cross-page UUID duplication, and feeds the existing adapter.

The shadow receipt contains only:

```text
page_count
record_count
unique_source_count
replay_sha256
watermark_completed_at
watermark_source_id
trust_layer_write_count
```

`trust_layer_write_count` must be zero. The shadow path must not write canon,
memory, skills, routing, issue state, or Multica data.

## Enable procedure

1. Release and deploy Multica with both candidate commits through the normal
   Multica release process.
2. Confirm `multica issue terminal-tasks --help` exists on the deployed CLI.
3. Run the focused producer and consumer tests listed below.
4. Run one explicit `collect_shadow()` invocation and retain only its receipt.
5. Publish a status contract before scheduling the collector:
   `/opt/second-brain/status/ai-services/multica-terminal-shadow.status.json`.
   It must include `run_state`, `emitted_at`, and
   `freshness_sla_minutes`, and be discovered by `ai.services.discovery`.
6. Schedule one read-only daily run. Do not import the adapter into the live
   Day Evolve learning pipeline during the shadow window.

## Daily acceptance and replay

Retain 14 consecutive daily receipts. A day passes only when:

- the command and adapter exit successfully;
- one fixed watermark is used for the whole scan;
- `record_count == unique_source_count`;
- the same captured pages replay to the same `replay_sha256`;
- `trust_layer_write_count == 0`;
- the status contract is fresh and reports success.

Any missing day, duplicate UUID, schema drift, replay mismatch, stale status,
or trust-layer write resets the consecutive count to zero.

## Disable and rollback

Disable the scheduled caller and read back that no new receipt or status
success is emitted. The adapter itself remains disabled by default and must not
be imported by the live pipeline.

If the producer must be removed before release, revert the candidate commits
in reverse order and rerun the focused tests. If it has been released, ship a
normal rollback release; do not alter task data or the database manually. The
endpoint and consumer are read-only, so rollback requires no data migration.

## Verification

From the Multica source repository:

```bash
cd server
go test ./internal/handler -run TerminalTaskMetadata -count=1
go test ./cmd/multica -run IssueTerminalTasks -count=1
```

From the Cursor workspace:

```bash
python3 -m pytest -q \
  projects/active/day-evolve/tests/test_multica_source.py \
  projects/active/day-evolve/tests/test_multica_shadow.py
```

These tests are necessary but do not replace the live release readback,
status-contract discovery, or the 14-day shadow gate.
