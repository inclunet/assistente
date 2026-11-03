package wailsapi

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/chat"
)

func TestTimelineEventUsesPublicReplayMapping(t *testing.T) {
	updates := timelineUpdates([]chat.CanonicalBatch{{ConversationID: "conversation", Epoch: "epoch", Sequence: "9007199254740993", WindowPatches: []chat.CanonicalWindowPatch{{WindowID: "window", PreviousRevision: "1", Revision: "2", OperationIndexes: []int{0}}}}})
	event := apidto.TimelineEvent{ConversationID: "conversation", WindowID: "window", Epoch: "epoch", SnapshotRevision: "1", Updates: updates}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"conversationId":"conversation"`, `"windowId":"window"`, `"sequence":"9007199254740993"`, `"snapshotRevision":"1"`, `"operations":[]`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("campo ausente %s: %s", field, encoded)
		}
	}
	if strings.Contains(string(encoded), "operationIndexes") {
		t.Fatal("roteamento interno vazou")
	}
	event.Updates = timelineUpdates(nil)
	event.SyncRequired, event.Reason = true, "journal_gap"
	encoded, err = json.Marshal(event)
	if err != nil || !strings.Contains(string(encoded), `"updates":[]`) || !strings.Contains(string(encoded), `"syncRequired":true`) {
		t.Fatalf("sync sem envelope válido: %s %v", encoded, err)
	}
}
