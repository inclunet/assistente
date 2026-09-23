package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"assistente/internal/commanddeck"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func TestCommandDeckFeedbackStatusLabelsAreLocalized(t *testing.T) {
	want := map[string]map[string]string{
		"pt-BR": {"waiting": "Aguardando", "running": "Em execução", "succeeded": "Concluído", "failed": "Falhou", "denied": "Negado", "cancelled": "Cancelado", "timed_out": "Tempo esgotado", "outcome_unknown": "Resultado desconhecido"},
		"en":    {"waiting": "Waiting", "running": "Running", "succeeded": "Succeeded", "failed": "Failed", "denied": "Denied", "cancelled": "Cancelled", "timed_out": "Timed out", "outcome_unknown": "Outcome unknown"},
		"es":    {"waiting": "En espera", "running": "En ejecución", "succeeded": "Completado", "failed": "Fallido", "denied": "Denegado", "cancelled": "Cancelado", "timed_out": "Tiempo agotado", "outcome_unknown": "Resultado desconocido"},
	}
	for locale, labels := range want {
		for state, label := range labels {
			if got := commandDeckFeedbackStatusLabel(locale, state); got != label {
				t.Errorf("%s/%s = %q, want %q", locale, state, got, label)
			}
		}
	}
	if got := commandDeckFeedbackStatusLabel("fr", "running"); got != "Running" {
		t.Fatalf("locale fallback = %q", got)
	}
}

func TestCommandDeckFeedbackTransitionsChangeFrameAndResetToReady(t *testing.T) {
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72}
	renderer := commanddeck.NewRenderer()
	if err := renderer.OpenDevice("deck", model); err != nil {
		t.Fatal(err)
	}

	binding := commandDeckBinding{commandID: "command", title: "Run command"}
	ready := commandDeckKeyView(binding, "en", model)
	if ready.State != "ready" || ready.Announce != "Run command" {
		t.Fatalf("ready view = %+v", ready)
	}
	if len(ready.ImageRGBA) != model.KeyImageW*model.KeyImageH*4 {
		t.Fatalf("ready image length = %d", len(ready.ImageRGBA))
	}
	if _, err := renderer.Render(commanddeck.Frame{Device: "deck", Model: model, Keys: map[int]commanddeck.KeyView{0: ready}}); err != nil {
		t.Fatal(err)
	}

	previous := ready.ImageRGBA
	for _, state := range []string{"waiting", "running", "succeeded", "failed", "denied", "cancelled", "timed_out", "outcome_unknown"} {
		binding.feedbackState = state
		binding.feedbackInvocationID = "inv-" + state
		view := commandDeckKeyView(binding, "pt-BR", model)
		if view.State != state || view.Announce == binding.title || !bytes.Contains([]byte(view.Announce), []byte(commandDeckFeedbackStatusLabel("pt-BR", state))) {
			t.Fatalf("%s view = %+v", state, view)
		}
		if len(view.ImageRGBA) != model.KeyImageW*model.KeyImageH*4 {
			t.Fatalf("%s image length = %d", state, len(view.ImageRGBA))
		}
		if bytes.Equal(previous, view.ImageRGBA) {
			t.Fatalf("%s did not change raster", state)
		}
		plan, err := renderer.Render(commanddeck.Frame{Device: "deck", Model: model, Keys: map[int]commanddeck.KeyView{0: view}})
		if err != nil || len(plan.Updates) != 1 || plan.Updates[0].Index != 0 {
			t.Fatalf("%s diff = %+v err=%v", state, plan, err)
		}
		previous = view.ImageRGBA
	}

	binding.feedbackState = ""
	binding.feedbackInvocationID = ""
	reset := commandDeckKeyView(binding, "pt-BR", model)
	if reset.State != "ready" || reset.Announce != binding.title {
		t.Fatalf("reset view = %+v", reset)
	}
	plan, err := renderer.Render(commanddeck.Frame{Device: "deck", Model: model, Keys: map[int]commanddeck.KeyView{0: reset}})
	if err != nil || len(plan.Updates) != 1 {
		t.Fatalf("ready reset diff = %+v err=%v", plan, err)
	}
}

func TestCommandDeckFeedbackRasterKeepsCustomContentGeometry(t *testing.T) {
	for _, size := range []int{16, 40, 72, 96} {
		model := commanddeck.Model{KeyImageW: size, KeyImageH: size}
		view := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "A title", icon: "star", feedbackState: "running", feedbackInvocationID: "inv"}, "es", model)
		if len(view.ImageRGBA) != size*size*4 {
			t.Fatalf("size %d image length = %d", size, len(view.ImageRGBA))
		}
	}
}

func TestCommandDeckFeedbackSmallRasterKeepsTitlePixelsWithOrWithoutStatus(t *testing.T) {
	model := commanddeck.Model{KeyImageW: 16, KeyImageH: 16}
	withoutStatus := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "Title"}, "en", model)
	withStatus := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "Title", feedbackState: "running", feedbackInvocationID: "inv"}, "en", model)
	if !hasCommandDeckTitlePixels(withoutStatus.ImageRGBA) || !hasCommandDeckTitlePixels(withStatus.ImageRGBA) {
		t.Fatal("16x16 raster lost title pixels")
	}
	if !bytes.Equal(withoutStatus.ImageRGBA, withStatus.ImageRGBA) {
		t.Fatal("status footer occupied a 16x16 raster without room")
	}
}

func hasCommandDeckTitlePixels(pixels []byte) bool {
	for index := 0; index+3 < len(pixels); index += 4 {
		if pixels[index] != 24 || pixels[index+1] != 24 || pixels[index+2] != 24 || pixels[index+3] != 255 {
			return true
		}
	}
	return false
}

func TestCommandDeckFeedbackOverlayClearsRetiredInstance(t *testing.T) {
	p, state, versions, cancel := newDeckFeedbackTestState(t)
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	instanceCtx, instanceCancel := context.WithCancel(ctx)
	controller := &commandDeckController{
		p: p, ctx: ctx, versions: versions, generation: 7,
		instances: map[string]commandDeckInstance{
			"deck": {id: "instance-1", ctx: instanceCtx, cancel: instanceCancel},
		},
	}
	identity := "streamdeck.key:deck:key:1"
	state.mu.Lock()
	state.registerDeckFeedbackLocked("invocation-1", state.occurrences["invocation-1"])
	state.mu.Unlock()
	source := commandDeckMap{"deck": {0: {identity: identity, title: "Run", feedbackState: "failed", feedbackInvocationID: "old"}}}
	current := controller.overlayDeckFeedback(ctx, source)
	if got := current["deck"][0].feedbackState; got != "waiting" {
		t.Fatalf("live feedback = %q", got)
	}
	controller.opened("deck")
	refreshed := controller.overlayDeckFeedback(ctx, source)
	if got := refreshed["deck"][0]; got.feedbackState != "" || got.feedbackInvocationID != "" {
		t.Fatalf("reconnect carried old feedback: %+v", got)
	}
}

func TestCommandDeckFeedbackFooterTruncationKeepsUTF8(t *testing.T) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 8, DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	got := fitCommandDeckText(font.Drawer{Face: face}, strings.Repeat("á🎛", 32), 20)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 after footer truncation: %q", got)
	}
}
