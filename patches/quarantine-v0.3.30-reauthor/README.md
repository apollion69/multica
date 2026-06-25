# Quarantined patches — need re-authoring against v0.3.30

**Why quarantined (2026-06-25):** these three local patches do **not** replay onto upstream
`v0.3.30`. The active series `0001..0008` applies cleanly; appending these caused
`multica-upgrade.sh` to HALT at `git apply --check` of 0009 (fail-safe — no prod damage, but no
upgrade either). Moving them here unblocks the upgrade path and preserves them for re-authoring.

**Root cause (verified):** the fork branch `feat/smtp-relay-support` has merge-base **v0.2.13**
with `v0.3.30` — it branched long ago and was never rebased onto v0.3.x. `server/internal/daemon/daemon.go`
diverged ~2300 lines: `func (d *Daemon) runTask` moved from line **1043** (fork) to **~3372** (v0.3.30)
**and changed signature** — v0.3.30 added a `slot int` param:
`runTask(ctx, task, provider string, slot int, taskLog *slog.Logger)`. So these patches' context lines
no longer exist where the hunks expect them. `git apply --3way` fails (`does not match index` — the
pre-image blobs aren't in the v0.3.30 tree); GNU `patch --fuzz=3` "applies" 0009/0010 but only by
misplacing hunks 2600+ lines away (wrong functions) — **not** a valid port.

## What each patch does (intent — re-author these behaviors onto v0.3.30)

| Patch | Files | Intent |
|---|---|---|
| `0009-…-codex-task-shell-env-set` | daemon.go (@@1043 runTask, @@1467), daemon_test.go, codex_sandbox.go (@@142) | In `runTask` codex branch: set `agentEnv["HOME"]=env.RootDir`, call new `ensureCodexHomeAlias(rootDir, codexHome)` (symlink `<rootDir>/.codex` → CODEX_HOME), and `execenv.EnsureCodexTaskShellEnvConfig(<codexHome>/config.toml, agentEnv)` so the codex task shell inherits the env. |
| `0010-…-codex-copy-roles` | codex_home.go, execenv_test.go | Extend `codexCopiedFiles` / `prepareCodexHomeWithOpts` to copy role files into the per-task codex home. |
| `0011-…-codex-issue-snapshot-prompt` | daemon.go (@@979 runTask), prompt.go, context.go, execenv.go, handler/daemon.go, runtime_config.go, daemon_test.go | Seed an issue snapshot into the task prompt. **Hardest** — hunks rejected on prompt.go/daemon.go/handler/daemon.go/runtime_config.go even with fuzz; needs a real port. |

## How to re-author (for Codex — owns this runtime code)

1. Build the base: `git worktree add --detach <wt> v0.3.30 && cd <wt>` then `git apply` the active
   `patches/0001..0008`.
2. Hand-port each behavior above into v0.3.30's `runTask` (note the new `slot int` signature) and the
   sibling files. Compile with `go build ./...` + `go test ./internal/daemon/...` (a Go toolchain is
   required — none on the WSL host, so this must run where Go exists, e.g. the build container / lx1).
3. Re-generate the patch files: commit each ported change, `git format-patch` against the base8 commit,
   and drop the new `0009/0010/0011` back into `patches/`.
4. Verify the full series replays: fresh `v0.3.30` worktree → `git apply --check patches/*.patch` all OK,
   then `multica-upgrade.sh` (dry rehearsal).

Audited by Claude (D-663). The original drifted patch files are preserved in this directory verbatim.
