package commanddeck

import (
	"context"
	"os"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbridge"
)

type manualController struct {
	events []commandadapter.Event
}

func (m *manualController) Input(_ context.Context, event commandadapter.Event) (commandbridge.InvocationAck, error) {
	m.events = append(m.events, event)
	return commandbridge.InvocationAck{Accepted: true, InvocationID: "manual"}, nil
}

func (m *manualController) Lock(context.Context) error   { return nil }
func (m *manualController) Logout(context.Context) error { return nil }

func TestManualStreamDeckPhysicalRoundTrip(t *testing.T) {
	if os.Getenv("ASSISTENTE_STREAMDECK_MANUAL") != "1" {
		t.Skip("defina ASSISTENTE_STREAMDECK_MANUAL=1 para executar com hardware físico")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	controller := &manualController{}
	adapter, err := NewDeviceAdapter(NewManager(NewRenderer(), BackoffPolicy{}), controller)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(NewStreamDeckDriver(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	results := runtime.DiscoverDetailed(ctx)
	if len(results) == 0 {
		t.Fatal("nenhum Stream Deck enumerado")
	}
	var opened DeviceID
	for _, result := range results {
		if result.Opened {
			opened = result.Device
			break
		}
		t.Logf("falha ao abrir %s: %v", result.Device, result.Err)
	}
	if opened == "" {
		t.Fatalf("nenhum Stream Deck abriu: %+v", results)
	}
	snapshot, err := adapter.manager.Snapshot(opened)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Stream Deck aberto: %s modelo=%s keys=%d", opened, snapshot.Model.Name, snapshot.Model.KeyCount())
	if err := runtime.Render(ctx, Frame{Device: opened, Model: snapshot.Model, Keys: map[int]KeyView{0: {
		Title:     "Teste",
		ImageID:   "manual-red",
		ImageRGBA: solidRGBA(snapshot.Model.KeyImageW, snapshot.Model.KeyImageH, 255, 0, 0),
		State:     "manual",
		Announce:  "Teste manual",
	}}}); err != nil {
		t.Fatal(err)
	}
	t.Log("pressione a primeira tecla do Stream Deck em até 30s")
	for len(controller.events) == 0 {
		if err := runtime.PollOne(ctx, opened); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("evento recebido: %+v", controller.events[0])
}

func solidRGBA(width, height int, r, g, b byte) []byte {
	pixels := make([]byte, width*height*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = r
		pixels[i+1] = g
		pixels[i+2] = b
		pixels[i+3] = 255
	}
	return pixels
}
