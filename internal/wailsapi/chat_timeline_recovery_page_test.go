package wailsapi

import (
	"assistente/internal/apidto"
	"assistente/internal/database"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestTimelineAPIReadRecoveryPages(t *testing.T) {
	f := newTimelineAPIFixture(t)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	execution, err := f.projection.Admit(f.ctx, f.conversation, f.message, id.String(), func() (database.ChatTimelineVersion, error) {
		version, readErr := f.reader.ReadVersion(f.ctx, f.conversation)
		if readErr != nil {
			return version, readErr
		}
		_, writeErr := database.NewChatTurnRepository(database.DB()).BeginExecution(f.ctx, f.conversation, f.message, "", database.TurnModeNew, id.String(), "")
		return version, writeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	execution.StartStream(0).Append(strings.Repeat("x", 64*1024) + "fim")
	opened, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	var reference *apidto.TimelineRecoveryReference
	for _, item := range opened.Items {
		if item.RecoveryReference != nil {
			reference = item.RecoveryReference
			break
		}
	}
	if reference == nil {
		t.Fatal("snapshot não publicou referência ativa")
	}
	request := apidto.TimelineRecoveryPageRequest{Reference: *reference}
	first, err := f.api.ReadRecoveryPage(opened.WindowID, request)
	if err != nil || !first.Found || len(first.Chunks) != 1 || len(first.Chunks[0].Text) != 64*1024 || first.NextCursor == "" {
		t.Fatalf("primeira página: %+v %v", first, err)
	}
	reservation := apidto.TimelineRecoveryReservationRequest{Reference: *reference, ContentVersion: first.ContentVersion}
	if _, err := f.api.ReserveRecovery(opened.WindowID, reservation); err == nil {
		t.Fatal("reservou execução ativa")
	}
	repo := database.NewChatTurnRepository(database.DB())
	if err := execution.CommitTerminal(database.TurnInterrupted, "cancelled", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.message, id.String(), database.TurnInterrupted, "cancelled", "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.api.ReserveRecovery(opened.WindowID, reservation)
	if err != nil || receipt.ContentVersion != first.ContentVersion || receipt.Reference != *reference {
		t.Fatal(receipt, err)
	}
	request.Cursor = first.NextCursor
	last, err := f.api.ReadRecoveryPage(opened.WindowID, request)
	if err != nil || !last.Found || last.ContentVersion != first.ContentVersion || last.NextCursor != "" || len(last.Chunks) != 1 || last.Chunks[0].Text != "fim" || last.Chunks[0].ByteOffset != 64*1024 {
		t.Fatalf("última página: %+v %v", last, err)
	}
	if result, err := f.api.ReadRecoveryPage("wrong-window", request); err == nil || result != nil {
		t.Fatal("capability inválida aceita")
	}
	for range 2 {
		if err := f.api.ReleaseRecovery(opened.WindowID, reservation); err != nil {
			t.Fatal(err)
		}
	}
	f.session.mu.Lock()
	f.session.session = "new-session"
	f.session.mu.Unlock()
	if _, err := f.api.ReserveRecovery(opened.WindowID, reservation); err == nil {
		t.Fatal("sessão antiga reservou")
	}
	if err := f.api.ReleaseRecovery(opened.WindowID, reservation); err == nil {
		t.Fatal("sessão antiga liberou")
	}
	if result, err := f.api.ReadRecoveryPage(opened.WindowID, request); err == nil || result != nil {
		t.Fatal("sessão antiga aceita")
	}
}
