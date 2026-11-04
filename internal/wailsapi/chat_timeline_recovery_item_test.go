package wailsapi

import (
	"reflect"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestTimelineAPIRecoveryItemExactSnapshotAndRefresh(t *testing.T) {
	f := newTimelineAPIFixture(t)
	executionID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	id := executionID.String()
	execution, err := f.projection.Admit(f.ctx, f.conversation, f.message, id, func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = database.NewChatTurnRepository(database.DB()).BeginExecution(f.ctx, f.conversation, f.message, "", database.TurnModeNew, id, "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	execution.StartStream(0).Append("texto não persistido 🦜")
	repo := database.NewChatTurnRepository(database.DB())
	if err := execution.CommitTerminal(database.TurnInterrupted, "cancelled", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.message, id, database.TurnInterrupted, "cancelled", "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	var original *apidto.CanonicalItem
	for _, item := range opened.Items {
		if item.RecoveryReference != nil {
			copy := item
			original = &copy
			break
		}
	}
	if original == nil {
		t.Fatal("snapshot sem item recuperável")
	}
	request := apidto.TimelineRecoveryItemRequest{Reference: *original.RecoveryReference, ItemID: original.ItemID, Epoch: opened.Epoch, Revision: original.Revision}
	if _, err := f.api.ReadRecoveryItem(opened.WindowID, request); err == nil {
		t.Fatal("leitura sem reserva")
	}
	for range 2 {
		receipt, err := f.api.ReserveRecoveryItem(opened.WindowID, request)
		if err != nil || receipt.Identity != request || receipt.ConversationID != f.conversation {
			t.Fatal(receipt, err)
		}
	}
	if _, err := f.api.Refresh(opened.WindowID, apidto.RefreshTimelineRequest{WindowRevision: opened.WindowRevision}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := f.api.ReadRecoveryItem(opened.WindowID, request)
		if err != nil || !reflect.DeepEqual(got, original) {
			t.Fatalf("reserva diferente do snapshot: %+v %v", got, err)
		}
		got.Metadata.Model = "alterado"
	}
	f.session.mu.Lock()
	f.session.session = "revoked"
	f.session.mu.Unlock()
	if _, err := f.api.ReadRecoveryItem(opened.WindowID, request); err == nil {
		t.Fatal("sessão revogada leu")
	}
}
