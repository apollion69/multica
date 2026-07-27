package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"
)

func createTerminalMetadataFixture(t *testing.T) (string, []string, []time.Time) {
	t.Helper()
	ctx := context.Background()
	var agentID, runtimeID, issueID string
	if err := testPool.QueryRow(ctx,
		`SELECT id, runtime_id FROM agent WHERE workspace_id = $1 AND runtime_id IS NOT NULL LIMIT 1`,
		testWorkspaceID,
	).Scan(&agentID, &runtimeID); err != nil {
		t.Fatalf("get test agent: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (
			workspace_id, title, status, priority, creator_id,
			creator_type, number, position
		)
		VALUES ($1, 'terminal metadata', 'done', 'none', $2, 'member', 987654, 0)
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	ids := make([]string, 0, 2)
	completed := make([]time.Time, 0, 2)
	for index, status := range []string{"completed", "failed"} {
		var taskID string
		at := base.Add(time.Duration(index) * time.Minute)
		if err := testPool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (
				agent_id, issue_id, status, created_at, dispatched_at,
				started_at, completed_at, attempt, max_attempts, is_leader_task,
				runtime_id
			)
			VALUES ($1, $2, $3, $4, $4, $4, $4, 1, 3, false, $5)
			RETURNING id
		`, agentID, issueID, status, at, runtimeID).Scan(&taskID); err != nil {
			t.Fatalf("create terminal task: %v", err)
		}
		ids = append(ids, taskID)
		completed = append(completed, at)
	}
	return agentID, ids, completed
}

func getTerminalMetadata(t *testing.T, query url.Values) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest(
		http.MethodGet,
		"/api/task-runs/terminal-metadata?"+query.Encode(),
		nil,
	)
	testHandler.ListTerminalTaskMetadata(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListTerminalTaskMetadata status=%d body=%s", w.Code, w.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func TestListTerminalTaskMetadataIsBodyFreeAndWatermarked(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	agentID, taskIDs, completed := createTerminalMetadataFixture(t)
	first := getTerminalMetadata(t, url.Values{
		"limit":                  {"1"},
		"after_completed_at":     {completed[0].Add(-time.Second).Format(time.RFC3339Nano)},
		"after_id":               {"00000000-0000-0000-0000-000000000000"},
		"watermark_completed_at": {completed[1].Format(time.RFC3339Nano)},
		"watermark_id":           {taskIDs[1]},
	})

	if got := fmt.Sprint(first["has_more"]); got != "true" {
		t.Fatalf("has_more=%s, want true", got)
	}
	records := first["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("first page records=%d, want 1", len(records))
	}
	record := records[0].(map[string]any)
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wantKeys := []string{
		"attempt", "completed_at", "created_at", "dispatched_at",
		"is_leader_task", "issue_id", "kind", "max_attempts",
		"parent_task_id", "source_id", "started_at", "status",
	}
	if fmt.Sprint(keys) != fmt.Sprint(wantKeys) {
		t.Fatalf("record keys=%v, want %v", keys, wantKeys)
	}

	next := first["next_cursor"].(map[string]any)
	watermark := first["watermark"].(map[string]any)
	var lateID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (
			agent_id, issue_id, status, created_at, completed_at, runtime_id
		)
		SELECT $1, issue_id, 'completed', now(), now() + interval '1 minute', runtime_id
		FROM agent_task_queue WHERE id = $2
		RETURNING id
	`, agentID, taskIDs[0]).Scan(&lateID); err != nil {
		t.Fatalf("create post-watermark task: %v", err)
	}

	second := getTerminalMetadata(t, url.Values{
		"limit":                  {"10"},
		"after_completed_at":     {fmt.Sprint(next["completed_at"])},
		"after_id":               {fmt.Sprint(next["source_id"])},
		"watermark_completed_at": {fmt.Sprint(watermark["completed_at"])},
		"watermark_id":           {fmt.Sprint(watermark["source_id"])},
	})
	secondRecords := second["records"].([]any)
	if len(secondRecords) != 1 {
		t.Fatalf("second page records=%d, want 1", len(secondRecords))
	}
	gotID := secondRecords[0].(map[string]any)["source_id"]
	if gotID == lateID || gotID == record["source_id"] {
		t.Fatalf(
			"unstable or duplicate second-page source_id=%v late=%s first=%v expected=%s",
			gotID,
			lateID,
			record["source_id"],
			taskIDs[1],
		)
	}
}

func TestListTerminalTaskMetadataRejectsPartialCursor(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	w := httptest.NewRecorder()
	req := newRequest(
		http.MethodGet,
		"/api/task-runs/terminal-metadata?after_id=00000000-0000-4000-8000-000000000001",
		nil,
	)
	testHandler.ListTerminalTaskMetadata(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("partial cursor status=%d, want 400", w.Code)
	}
}
