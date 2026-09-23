package commandconfig

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
)

func TestProjectCompleteMaterializaSuppressComArgumentsKeyVazia(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	row.LayerRefKind, row.LayerRef = "builtin", "application.defaults"
	row.ReplacesDefaultID, row.ReplacesDefaultVersion, row.ReplacesDefaultFingerprint = stringPtr("builtin.tab.new"), stringPtr("1"), stringPtr("fp-v1")
	row.CommandID = nil
	row.Arguments = "{}"
	row.Effect = "suppress"
	row.Presentation = `{"version":1,"image_ref":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	snapshot := Snapshot{Scope: Scope{UserID: user}, Bindings: []Binding{row}}
	options := completeProjectionOptions(completeProjectionRegistry(t))

	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || result.Status != commandbindings.Suppressed || len(result.BindingIDs) != 1 || result.BindingIDs[0] != row.ID {
		t.Fatalf("supressão não materializada: result=%+v err=%v", result, err)
	}
	if imageRef := configuration.ImageForBindings(result.BindingIDs); imageRef != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("imagem da supressão elegível não materializada: %q", imageRef)
	}
}
