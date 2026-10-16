package app

import (
	"context"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandinput"
	"assistente/internal/database"
)

// Mede o caminho produtivo até a emissão ao frontend, sem abrir HID/Wails.
// Não mede transporte, renderização ou o teclado LOCAL_UI (que não usa IPC).
// As janelas são independentes: Input inclui uma nova resolução e os gates;
// portanto seus percentis NÃO podem ser subtraídos ou somados aos da resolução.
func TestCommandDeckAppLatency(t *testing.T) {
	if os.Getenv("COMMAND_APP_LATENCY") != "1" {
		t.Skip("qualificação opt-in: COMMAND_APP_LATENCY=1")
	}
	for _, mutate := range []bool{false, true} {
		name := "steady"
		if mutate {
			name = "after_layer_mutation"
		}
		t.Run(name, func(t *testing.T) {
			a := deckConfiguredFixture(t)
			p := a.commandProduct.Load()
			ctx := context.Background()
			settings, err := a.GetCommandSettings("pt-BR")
			if err != nil {
				t.Fatal(err)
			}
			layerID := ""
			for _, layer := range settings.Layers {
				if !layer.Builtin && layer.Active {
					layerID = layer.ID
					break
				}
			}
			if layerID == "" {
				t.Fatal("camada da fixture ausente")
			}
			emitted := 0
			a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
				if name != "command:deck-local-ui" {
					return
				}
				event := value.(CommandDeckLocalUIEvent)
				if event.CommandID != "navigation.settings.open" || event.Generation == "" || event.SessionID != p.principal.SessionID {
					t.Errorf("handoff inesperado: %+v", event)
				}
				emitted++
			})
			newController := func() *commandDeckController {
				if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
					t.Fatal(err)
				}
				configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
				if err != nil {
					t.Fatal(err)
				}
				c := &commandDeckController{p: p, ctx: ctx, versions: versions,
					identities: deckTriggerIdentities(configuration, p.registry, versions),
					pressed:    map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{},
					generation: p.deckInputGeneration()}
				c.opened("test-deck")
				return c
			}
			c := newController()
			defer func() { c.reset("") }()
			const warmup, samples = 10, 100
			resolution, ingress, rebuild := []time.Duration{}, []time.Duration{}, []time.Duration{}
			press := commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown}
			release := press
			release.Kind = commandinput.KeyUp
			for i := 0; i < warmup+samples; i++ {
				if mutate {
					start := time.Now()
					if _, err := a.SetCommandLayerActive(layerID, false); err != nil {
						t.Fatal(err)
					}
					if _, _, ok := p.resolveDeckPress(ctx, c.identities, c.versions, "test-deck", 0); ok {
						t.Fatal("projeção aposentada resolveu após mutação")
					}
					if _, err := a.SetCommandLayerActive(layerID, true); err != nil {
						t.Fatal(err)
					}
					c.reset("")
					c = newController()
					if i >= warmup {
						rebuild = append(rebuild, time.Since(start))
					}
				}
				start := time.Now()
				binding, _, ok := p.resolveDeckPress(ctx, c.identities, c.versions, "test-deck", 0)
				resolved := time.Since(start)
				if !ok || binding.commandID != "navigation.settings.open" {
					t.Fatal("resolução não selecionou comando produtivo")
				}
				before := emitted
				start = time.Now()
				ack, err := c.Input(ctx, press)
				elapsed := time.Since(start)
				if err != nil || !ack.Accepted || ack.InvocationID != "" || emitted != before+1 {
					t.Fatalf("ingresso: ack=%+v err=%v eventos=%d", ack, err, emitted-before)
				}
				if _, err := c.Input(ctx, release); err != nil {
					t.Fatal(err)
				}
				if i >= warmup {
					resolution = append(resolution, resolved)
					ingress = append(ingress, elapsed)
				}
			}
			var count int64
			if err := database.DB().Table("command_invocations").Where("command_id = ?", "navigation.settings.open").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("navegação local gravou invocações: count=%d err=%v", count, err)
			}
			t.Logf("go=%s os=%s arch=%s GOMAXPROCS=%d warmup=%d; SQLite temporário; sem HID/transporte/render", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0), warmup)
			for _, window := range []struct {
				name   string
				values []time.Duration
			}{{"resolution", resolution}, {"input_to_emit", ingress}, {"layer_mutation_and_rebuild", rebuild}} {
				if len(window.values) == 0 {
					continue
				}
				sort.Slice(window.values, func(i, j int) bool { return window.values[i] < window.values[j] })
				zeroSamples := sort.Search(len(window.values), func(i int) bool { return window.values[i] > 0 })
				percentile := func(p int) time.Duration { return window.values[(p*len(window.values)+99)/100-1] }
				t.Logf("window=%s n=%d zero_samples=%d p50=%s p95=%s p99=%s max=%s", window.name, len(window.values), zeroSamples, percentile(50), percentile(95), percentile(99), window.values[len(window.values)-1])
				if zeroSamples > 0 {
					t.Log("amostras zero estão abaixo da resolução observável do relógio; não comprovam orçamento submilissegundo")
				}
			}
		})
	}
}
