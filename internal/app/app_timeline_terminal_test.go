package app

import (
	"testing"

	"assistente/internal/apidto"
)

type timelineControlCapture struct {
	names  []string
	values []any
}

func (c *timelineControlCapture) Emit(name string, value any) {
	c.names = append(c.names, name)
	c.values = append(c.values, value)
}

func TestTimelineTerminalControlRejectsOldLocalGeneration(t *testing.T) {
	capture := &timelineControlCapture{}
	a := &App{emitter: capture}
	a.timelineAuthGeneration.Store(3)
	emitter := &timelineWailsEmitter{principal: capturedTimelineSession{app: a, generation: 3}}
	event := apidto.TimelineClosedEvent{ConversationID: "conversation", SurfaceID: "surface", WindowID: "window"}
	// Sem lease vigente: controle não autoriza nem carrega conteúdo.
	emitter.NotifyClosed(event)
	if len(capture.names) != 1 || capture.names[0] != apidto.TimelineClosedEventName || capture.values[0] != event {
		t.Fatal("controle terminal ausente ou fora do contrato", capture)
	}
	a.timelineAuthGeneration.Add(1)
	emitter.NotifyClosed(event)
	if len(capture.names) != 1 {
		t.Fatal("controle antigo atravessou troca local de sessão")
	}
}
