package commandcontract

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestEnvelopeClonePreservesMarkerAndDetachesReferences(t *testing.T) {
	if got := (Envelope{}).Clone(); !reflect.DeepEqual(got, Envelope{}) {
		t.Fatal("clone alterou envelope vazio de marker")
	}
	base := resolvedDirectFixture()
	base.Arguments = rawPointer(`{}`)
	base.BindingIDs = []string{"binding.original"}
	base.ContextCapturedAtByProvider = &map[string]time.Time{"workspace": base.ReceivedAt}
	occurred := base.ReceivedAt
	base.SourceOccurredAt = &occurred
	before := base.Clone()
	got := base.Clone()
	*got.UserID = "changed"
	*got.Arguments = json.RawMessage(`{"changed":true}`)
	got.BindingIDs[0] = "changed"
	(*got.ContextCapturedAtByProvider)["workspace"] = occurred.Add(time.Hour)
	*got.SourceOccurredAt = occurred.Add(time.Hour)
	if !reflect.DeepEqual(base, before) {
		t.Fatal("alterar clone reescreveu referências do envelope original")
	}
}
