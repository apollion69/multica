package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunIssueTerminalTasksForwardsStableCursorAndPrintsJSON(t *testing.T) {
	wantQuery := url.Values{
		"after_completed_at":     {"2026-07-27T12:00:00.123Z"},
		"after_id":               {"11111111-1111-1111-1111-111111111111"},
		"limit":                  {"25"},
		"watermark_completed_at": {"2026-07-27T13:00:00.456Z"},
		"watermark_id":           {"22222222-2222-2222-2222-222222222222"},
	}
	wantPage := map[string]any{
		"records":     []any{},
		"has_more":    false,
		"next_cursor": nil,
		"watermark": map[string]any{
			"completed_at": "2026-07-27T13:00:00.456Z",
			"source_id":    "22222222-2222-2222-2222-222222222222",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/task-runs/terminal-metadata" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query(); got.Encode() != wantQuery.Encode() {
			t.Fatalf("query = %q, want %q", got.Encode(), wantQuery.Encode())
		}
		_ = json.NewEncoder(w).Encode(wantPage)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "workspace-1")
	t.Setenv("MULTICA_TOKEN", "test-token")

	cmd := &cobra.Command{Use: "terminal-tasks"}
	cmd.Flags().Int("limit", 100, "")
	cmd.Flags().String("after-completed-at", "", "")
	cmd.Flags().String("after-id", "", "")
	cmd.Flags().String("watermark-completed-at", "", "")
	cmd.Flags().String("watermark-id", "", "")
	_ = cmd.Flags().Set("limit", "25")
	_ = cmd.Flags().Set("after-completed-at", "2026-07-27T12:00:00.123Z")
	_ = cmd.Flags().Set("after-id", "11111111-1111-1111-1111-111111111111")
	_ = cmd.Flags().Set("watermark-completed-at", "2026-07-27T13:00:00.456Z")
	_ = cmd.Flags().Set("watermark-id", "22222222-2222-2222-2222-222222222222")

	out, err := captureStdout(t, func() error { return runIssueTerminalTasks(cmd, nil) })
	if err != nil {
		t.Fatalf("runIssueTerminalTasks: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if got["has_more"] != false || got["next_cursor"] != nil {
		t.Fatalf("unexpected page envelope: %#v", got)
	}
}
