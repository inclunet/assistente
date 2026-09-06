package workspace

import (
	"context"
	"log/slog"

	"assistente/internal/contextprovider"
)

const legacySurfaceAdapterEvent = "legacy_surface_context_adapter_applied"

type normalizedSurfaceContext struct {
	SurfaceType     string
	SurfaceID       string
	Title           string
	Mode            string
	Selection       map[string]any
	Focus           map[string]any
	Content         map[string]any
	Metadata        map[string]any
	SnapshotVersion string
	CapturedAt      string
	StaleAfterMs    string
	Incomplete      bool
}

func normalizeSurfaceContext(surface *contextprovider.Surface) *normalizedSurfaceContext {
	if surface == nil {
		return nil
	}
	ctx := surface.Context
	surfaceType := firstNonEmpty(stringFromMap(ctx, "surfaceType"), surface.Type)
	surfaceID := stringFromMap(ctx, "surfaceId")
	snapshotVersion := stringFromMap(ctx, "snapshotVersion")
	incomplete := surfaceType == "" || surfaceID == "" || snapshotVersion == ""

	if surfaceType == "" {
		return nil
	}
	if surfaceID == "" {
		surfaceID = firstNonEmpty(
			stringFromMap(surface.State, "sessionId"),
			stringFromMap(surface.State, "tasklistId"),
			stringFromMap(surface.State, "draftId"),
			stringFromMap(surface.State, "filePath"),
			surfaceType,
		)
	}
	if snapshotVersion == "" {
		snapshotVersion = "legacy:" + surfaceType + ":" + surfaceID
	}

	normalized := &normalizedSurfaceContext{
		SurfaceType:     surfaceType,
		SurfaceID:       surfaceID,
		Title:           firstNonEmpty(stringFromMap(ctx, "title"), surface.Title),
		Mode:            stringFromMap(ctx, "mode"),
		Selection:       mapFromMap(ctx, "selection"),
		Focus:           mapFromMap(ctx, "focus"),
		Content:         mapFromMap(ctx, "content"),
		Metadata:        mapFromMap(ctx, "metadata"),
		SnapshotVersion: snapshotVersion,
		CapturedAt:      stringFromMap(ctx, "capturedAt"),
		StaleAfterMs:    numberStringFromMap(ctx, "staleAfterMs"),
		Incomplete:      incomplete,
	}

	if incomplete {
		adaptLegacySurfaceContext(surface, normalized)
		logLegacySurfaceAdapterApplied()
	}
	return normalized
}

func adaptLegacySurfaceContext(surface *contextprovider.Surface, normalized *normalizedSurfaceContext) {
	if normalized == nil || surface == nil {
		return
	}
	if normalized.Mode == "" {
		normalized.Mode = stringFromMap(surface.Context, "mode")
	}
	if normalized.Selection == nil {
		if selectedText := stringFromMap(surface.Context, "selectedText"); selectedText != "" {
			normalized.Selection = map[string]any{
				"kind":     "text",
				"text":     selectedText,
				"explicit": true,
			}
		}
	}
	if normalized.Focus == nil {
		if cursorContext := stringFromMap(surface.Context, "cursorContext"); cursorContext != "" {
			normalized.Focus = map[string]any{
				"kind": "cursor",
				"text": cursorContext,
			}
		}
	}
	if normalized.Content == nil {
		switch {
		case stringFromMap(surface.Context, "historyPreview") != "":
			normalized.Content = map[string]any{
				"kind":         "terminal_output",
				"recentOutput": stringFromMap(surface.Context, "historyPreview"),
			}
		case stringFromMap(surface.Context, "tasksPreview") != "":
			normalized.Content = map[string]any{
				"kind":    "tasklist_summary",
				"summary": stringFromMap(surface.Context, "tasksPreview"),
			}
		}
	}
	if normalized.Metadata == nil {
		normalized.Metadata = map[string]any{}
	}
	for _, key := range []string{"filePath", "draftId", "tasklistId", "sessionId"} {
		if value := stringFromMap(surface.State, key); value != "" {
			metadataKey := key
			if key == "tasklistId" {
				metadataKey = "taskListId"
			}
			normalized.Metadata[metadataKey] = value
		}
	}
	normalized.Metadata["legacySurfaceContext"] = true
}

// logLegacySurfaceAdapterApplied usa somente valores fixos e não recebe o
// contexto da requisição para não registrar conteúdo, paths, payloads ou IDs.
func logLegacySurfaceAdapterApplied() {
	slog.LogAttrs(
		context.Background(),
		slog.LevelInfo,
		legacySurfaceAdapterEvent,
		slog.String("component", "context_compatibility"),
		slog.String("adapter", "incomplete_surface_context"),
		slog.Int("schema_version", 1),
	)
}
