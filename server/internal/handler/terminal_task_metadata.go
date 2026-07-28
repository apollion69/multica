package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const maxTerminalTaskMetadataPage = 100

type terminalTaskMetadataCursor struct {
	CompletedAt string `json:"completed_at"`
	SourceID    string `json:"source_id"`
}

type terminalTaskMetadataRecord struct {
	Attempt      int32   `json:"attempt"`
	CompletedAt  string  `json:"completed_at"`
	CreatedAt    string  `json:"created_at"`
	DispatchedAt *string `json:"dispatched_at"`
	IsLeaderTask bool    `json:"is_leader_task"`
	IssueID      string  `json:"issue_id"`
	Kind         string  `json:"kind"`
	MaxAttempts  int32   `json:"max_attempts"`
	ParentTaskID *string `json:"parent_task_id"`
	SourceID     string  `json:"source_id"`
	StartedAt    *string `json:"started_at"`
	Status       string  `json:"status"`
}

type terminalTaskMetadataPage struct {
	Records    []terminalTaskMetadataRecord `json:"records"`
	HasMore    bool                         `json:"has_more"`
	NextCursor *terminalTaskMetadataCursor  `json:"next_cursor"`
	Watermark  *terminalTaskMetadataCursor  `json:"watermark"`
}

func terminalTimestamp(value pgtype.Timestamptz) string {
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func terminalTimestampPtr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := terminalTimestamp(value)
	return &formatted
}

func parseTerminalTaskMetadataCursor(
	w http.ResponseWriter,
	completedAtRaw, idRaw, name string,
) (pgtype.Timestamptz, pgtype.UUID, bool) {
	if completedAtRaw == "" && idRaw == "" {
		return pgtype.Timestamptz{}, pgtype.UUID{}, true
	}
	if completedAtRaw == "" || idRaw == "" {
		writeError(w, http.StatusBadRequest, name+" cursor requires both fields")
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	completedAt, err := time.Parse(time.RFC3339Nano, completedAtRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name+"_completed_at")
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	id, err := util.ParseUUID(idRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name+"_id")
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	return pgtype.Timestamptz{Time: completedAt, Valid: true}, id, true
}

func taskMetadataCursor(
	completedAt pgtype.Timestamptz,
	sourceID pgtype.UUID,
) *terminalTaskMetadataCursor {
	return &terminalTaskMetadataCursor{
		CompletedAt: terminalTimestamp(completedAt),
		SourceID:    uuidToString(sourceID),
	}
}

// ListTerminalTaskMetadata returns a text-free, stable workspace task export.
func (h *Handler) ListTerminalTaskMetadata(w http.ResponseWriter, r *http.Request) {
	limit := maxTerminalTaskMetadataPage
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxTerminalTaskMetadataPage {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	afterCompletedAt, afterID, ok := parseTerminalTaskMetadataCursor(
		w,
		r.URL.Query().Get("after_completed_at"),
		r.URL.Query().Get("after_id"),
		"after",
	)
	if !ok {
		return
	}
	watermarkCompletedAt, watermarkID, ok := parseTerminalTaskMetadataCursor(
		w,
		r.URL.Query().Get("watermark_completed_at"),
		r.URL.Query().Get("watermark_id"),
		"watermark",
	)
	if !ok {
		return
	}
	if afterCompletedAt.Valid && !watermarkCompletedAt.Valid {
		writeError(w, http.StatusBadRequest, "after cursor requires a watermark")
		return
	}

	workspaceID, err := util.ParseUUID(h.resolveWorkspaceID(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	if !watermarkCompletedAt.Valid {
		watermark, watermarkErr := h.Queries.GetTerminalTaskMetadataWatermark(
			r.Context(),
			workspaceID,
		)
		if errors.Is(watermarkErr, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, terminalTaskMetadataPage{
				Records: []terminalTaskMetadataRecord{},
			})
			return
		}
		if watermarkErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to read task watermark")
			return
		}
		watermarkCompletedAt, watermarkID = watermark.CompletedAt, watermark.ID
	}
	if !afterCompletedAt.Valid {
		afterCompletedAt = pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
		afterID, _ = util.ParseUUID("00000000-0000-0000-0000-000000000000")
	}

	rows, err := h.Queries.ListTerminalTaskMetadata(
		r.Context(),
		db.ListTerminalTaskMetadataParams{
			WorkspaceID:          workspaceID,
			AfterCompletedAt:     afterCompletedAt,
			AfterID:              afterID,
			WatermarkCompletedAt: watermarkCompletedAt,
			WatermarkID:          watermarkID,
			RowLimit:             int32(limit + 1),
		},
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list terminal tasks")
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	records := make([]terminalTaskMetadataRecord, len(rows))
	for index, row := range rows {
		records[index] = terminalTaskMetadataRecord{
			Attempt:      row.Attempt,
			CompletedAt:  terminalTimestamp(row.CompletedAt),
			CreatedAt:    terminalTimestamp(row.CreatedAt),
			DispatchedAt: terminalTimestampPtr(row.DispatchedAt),
			IsLeaderTask: row.IsLeaderTask,
			IssueID:      uuidToString(row.IssueID),
			Kind:         row.Kind,
			MaxAttempts:  row.MaxAttempts,
			ParentTaskID: uuidToPtr(row.ParentTaskID),
			SourceID:     uuidToString(row.SourceID),
			StartedAt:    terminalTimestampPtr(row.StartedAt),
			Status:       row.Status,
		}
	}
	var nextCursor *terminalTaskMetadataCursor
	if hasMore {
		last := rows[len(rows)-1]
		nextCursor = taskMetadataCursor(last.CompletedAt, last.SourceID)
	}
	writeJSON(w, http.StatusOK, terminalTaskMetadataPage{
		Records:    records,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Watermark:  taskMetadataCursor(watermarkCompletedAt, watermarkID),
	})
}
