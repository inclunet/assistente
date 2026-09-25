package commanddeck

import (
	"errors"
	"testing"
)

var testModel = Model{ID: "streamdeck-mock-2x2", Name: "Stream Deck Mock 2x2", Rows: 2, Columns: 2, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}

func TestRendererForcesFullFrameAfterOpenAndReconnect(t *testing.T) {
	renderer := NewRenderer()
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	frame := Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{
		0: {Title: "Anterior", State: "idle"},
		3: {Title: "Próxima", ImageID: "next", ImageRGBA: []byte{1, 2, 3}, State: "ready", Announce: "Próxima aba"},
	}}
	plan, err := renderer.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.FullFrame || len(plan.Updates) != 4 {
		t.Fatalf("primeiro render deve ser completo: %+v", plan)
	}
	if plan.Updates[3].ImageHash == "" {
		t.Fatalf("imagem não recebeu hash/cache: %+v", plan.Updates[3])
	}
	again, err := renderer.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	if again.FullFrame || len(again.Updates) != 0 {
		t.Fatalf("render idempotente deve virar diff vazio: %+v", again)
	}
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	reconnected, err := renderer.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !reconnected.FullFrame || len(reconnected.Updates) != 4 {
		t.Fatalf("reconexão deve invalidar diff: %+v", reconnected)
	}
}

func TestRendererSendsOnlyChangedKeysAfterFullFrame(t *testing.T) {
	renderer := NewRenderer()
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	frame := Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{
		0: {Title: "A", State: "idle"},
		1: {Title: "B", State: "idle"},
	}}
	if _, err := renderer.Render(frame); err != nil {
		t.Fatal(err)
	}
	frame.Keys[1] = KeyView{Title: "B", State: "running"}
	frame.Keys[2] = KeyView{Title: "C", State: "idle"}
	plan, err := renderer.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	if plan.FullFrame || len(plan.Updates) != 2 || plan.Updates[0].Index != 1 || plan.Updates[1].Index != 2 {
		t.Fatalf("diff incremental inesperado: %+v", plan)
	}
}

func TestRendererClearsKeysRemovedFromSnapshot(t *testing.T) {
	renderer := NewRenderer()
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	frame := Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{
		0: {Title: "A"}, 1: {Title: "B", ImageID: "b", ImageRGBA: []byte{1}},
	}}
	if _, err := renderer.Render(frame); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []int{1, 0} {
		delete(frame.Keys, removed)
		plan, err := renderer.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		if plan.FullFrame || len(plan.Updates) != 1 || plan.Updates[0].Index != removed {
			t.Fatalf("remoção deve enviar somente tecla removida: %+v", plan)
		}
		view := plan.Updates[0].View
		if view.Title != "" || view.ImageID != "" || len(view.ImageRGBA) != 0 || view.State != "" || view.Announce != "" {
			t.Fatalf("tecla removida deve ficar vazia: %+v", view)
		}
		again, err := renderer.Render(frame)
		if err != nil || len(again.Updates) != 0 {
			t.Fatalf("remoção deve ser idempotente: %+v, %v", again, err)
		}
	}
}

func TestRendererRejectsInvalidFramesAndDevices(t *testing.T) {
	renderer := NewRenderer()
	if _, err := renderer.Render(Frame{Device: "deck-a", Model: testModel}); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("render sem abrir dispositivo err=%v", err)
	}
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	_, err := renderer.Render(Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{4: {Title: "fora"}}})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("índice fora deveria falhar: %v", err)
	}
	_, err = renderer.Render(Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {ImageRGBA: []byte{1}}}})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("imagem sem id deveria falhar: %v", err)
	}
	otherModel := testModel
	otherModel.ID = "streamdeck-other"
	_, err = renderer.Render(Frame{Device: "deck-a", Model: otherModel, Keys: map[int]KeyView{0: {Title: "x"}}})
	if !errors.Is(err, ErrInvalidModel) {
		t.Fatalf("modelo trocado deveria falhar: %v", err)
	}
}

func TestRendererClonesImageBytesInPlan(t *testing.T) {
	renderer := NewRenderer()
	if err := renderer.OpenDevice("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	image := []byte{1, 2, 3}
	plan, err := renderer.Render(Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "A", ImageID: "a", ImageRGBA: image}}})
	if err != nil {
		t.Fatal(err)
	}
	image[0] = 9
	if plan.Updates[0].View.ImageRGBA[0] != 1 {
		t.Fatalf("plano compartilhou slice mutável: %+v", plan.Updates[0].View.ImageRGBA)
	}
}
