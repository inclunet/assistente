package wailsapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestChatTimelineReopenDuringStreaming(t *testing.T) {
	f := newTimelineAPIFixture(t)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	executionID := id.String()
	execution, err := f.projection.Admit(f.ctx, f.conversation, f.message, executionID, func() (database.ChatTimelineVersion, error) {
		version, readErr := f.reader.ReadVersion(f.ctx, f.conversation)
		if readErr != nil {
			return version, readErr
		}
		_, writeErr := database.NewChatTurnRepository(database.DB()).BeginExecution(f.ctx, f.conversation, f.message, "", database.TurnModeNew, executionID, "")
		return version, writeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := execution.StartStream(0)
	stream.Append("primeiro")
	request := f.request()
	opened, err := f.api.Open(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.api.Close(opened.WindowID); err != nil {
		t.Fatal(err)
	}
	stream.Append(" fora da tela")
	events := make(chan apidto.TimelineEvent, 16)
	f.session.emit = func(ctx context.Context, event apidto.TimelineEvent) error {
		select {
		case events <- event:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	request.SurfaceID = "returned"
	returned, err := f.api.Open(request)
	if err != nil {
		t.Fatal(err)
	}
	returned, err = f.api.Navigate(returned.WindowID, apidto.TimelineWindowNavigation{Mode: apidto.TimelineWindowFollowTail, Epoch: returned.Epoch, WindowRevision: returned.WindowRevision, TimelineWindowPosition: request.TimelineWindowPosition})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(returned)
	if err != nil || !strings.Contains(string(encoded), "primeiro fora da tela") {
		t.Fatalf("snapshot não retomou conteúdo: %s, %v", encoded, err)
	}
	stream.Append(" depois de voltar")
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.WindowID != returned.WindowID || event.SurfaceID != request.SurfaceID || event.SyncRequired {
				t.Fatalf("entrega inválida: %+v", event)
			}
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "depois de voltar") {
				return
			}
		case <-timer.C:
			t.Fatal("janela reaberta não recebeu o próximo delta")
		}
	}
}
