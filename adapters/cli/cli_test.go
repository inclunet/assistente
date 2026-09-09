package cli_test

import (
	"bytes"
	"testing"

	"assistente/adapters/cli"
	"assistente/internal/core/ports"
)

func TestEmitterAdapter_StreamEvent(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("chat:stream", ports.StreamEvent{Delta: "Olá", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{Delta: " mundo", Sequence: 1})
	e.Emit("chat:stream", ports.StreamEvent{Done: true})

	if got := out.String(); got != "Olá mundo\n" {
		t.Errorf("saída esperada %q, obteve %q", "Olá mundo\n", got)
	}
	if errOut.Len() > 0 {
		t.Errorf("stderr deveria estar vazio, obteve %q", errOut.String())
	}
}

func TestEmitterAdapter_StreamContinuationIncludesBaseContent(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("chat:stream", ports.StreamEvent{
		BaseContent: "resposta parcial",
		Delta:       " continuada",
		Reset:       true,
		Sequence:    0,
	})
	e.Emit("chat:stream", ports.StreamEvent{Done: true})

	if got := out.String(); got != "resposta parcial continuada\n" {
		t.Fatalf("saída=%q", got)
	}
}

func TestEmitterAdapter_StreamRetryStartsNewLineAndValidatesSequence(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("chat:stream", ports.StreamEvent{Delta: "tentativa parcial", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{Delta: " ignorado", Sequence: 2})
	e.Emit("chat:stream", ports.StreamEvent{BaseContent: "base inválida", Delta: " ignorado", Reset: true, Sequence: 2})
	e.Emit("chat:stream", ports.StreamEvent{Delta: " válida", Sequence: 1})
	e.Emit("chat:stream", ports.StreamEvent{Delta: "recuperada", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{Done: true})

	if got := out.String(); got != "tentativa parcial válida\nrecuperada\n" {
		t.Fatalf("saída=%q", got)
	}
}

func TestEmitterAdapter_StreamScopedRejectsMissingOrDifferentConversation(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))
	e.WaitDone("conv-1")

	e.Emit("chat:stream", ports.StreamEvent{TurnID: "turn-empty", Delta: "sem conversa", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{ConversationId: "conv-2", TurnID: "turn-2", Delta: "outra conversa", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{ConversationId: "conv-1", TurnID: "turn-1", Delta: "correta", Reset: true, Sequence: 0})
	e.Emit("chat:stream", ports.StreamEvent{ConversationId: "conv-1", TurnID: "turn-1", Done: true})

	if got := out.String(); got != "correta\n" {
		t.Fatalf("saída=%q", got)
	}
}

func TestEmitterAdapter_StreamError(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("chat:stream", ports.StreamEvent{Error: "falha no provedor"})

	if out.Len() > 0 {
		t.Errorf("stdout deveria estar vazio, obteve %q", out.String())
	}
	if got := errOut.String(); got != "\nErro: falha no provedor\n" {
		t.Errorf("stderr esperado %q, obteve %q", "\nErro: falha no provedor\n", got)
	}
}

func TestEmitterAdapter_VerboseLogsEvents(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(
		cli.WithOutput(&out),
		cli.WithErrOutput(&errOut),
		cli.WithVerbose(true),
	)

	e.Emit("profile:changed", nil)

	if out.Len() > 0 {
		t.Errorf("stdout deveria estar vazio para eventos não-chat")
	}
	if !bytes.Contains(errOut.Bytes(), []byte("[event] profile:changed")) {
		t.Errorf("stderr deveria conter log do evento em modo verbose, obteve %q", errOut.String())
	}
}

func TestEmitterAdapter_SilentIgnoresNonChat(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("profile:changed", nil)
	e.Emit("workspace:update", nil)

	if out.Len() > 0 || errOut.Len() > 0 {
		t.Errorf("modo silencioso não deveria produzir output para eventos não-chat")
	}
}

func TestDialogAdapter_ReturnsError(t *testing.T) {
	d := cli.DialogAdapter{}

	_, err := d.OpenFileDialog(ports.OpenFileOptions{})
	if err == nil {
		t.Error("OpenFileDialog deveria retornar erro no CLI")
	}

	_, err = d.SaveFileDialog(ports.SaveFileOptions{})
	if err == nil {
		t.Error("SaveFileDialog deveria retornar erro no CLI")
	}
}

func TestWindowAdapter_ShowIsNoop(t *testing.T) {
	w := cli.WindowAdapter{}
	w.Show() // não deve panic
}

func TestEmitterAdapter_SegmentDone_DefaultMode(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	// Iteração intermediária com 2 tools — deve imprimir linha resumo
	e.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID: "1",
		Iteration:      0,
		HasMore:        true,
		ToolsInIteration: []ports.ToolSummary{
			{Name: "search_web", Status: "ok", DurationMs: 900},
			{Name: "read_file", Status: "ok", DurationMs: 200},
		},
	})

	if out.Len() > 0 {
		t.Errorf("stdout deveria estar vazio, obteve %q", out.String())
	}
	want := "[tools] iteração 1: 2 tools (search_web, read_file) — 1100ms\n"
	if got := errOut.String(); got != want {
		t.Errorf("stderr esperado %q, obteve %q", want, got)
	}
}

func TestEmitterAdapter_SegmentDone_FinalIteration_Silent(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	// Iteração final (HasMore=false) — não deve imprimir nada
	e.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID: "1",
		Iteration:      1,
		HasMore:        false,
	})

	if out.Len() > 0 || errOut.Len() > 0 {
		t.Errorf("iteração final não deveria produzir output, obteve stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestEmitterAdapter_SegmentDone_NoTools_Silent(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	// Iteração intermediária sem tools — não deve imprimir nada
	e.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID: "1",
		Iteration:      0,
		HasMore:        true,
	})

	if out.Len() > 0 || errOut.Len() > 0 {
		t.Errorf("sem tools não deveria produzir output, obteve stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestEmitterAdapter_SegmentDone_Verbose(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(
		cli.WithOutput(&out),
		cli.WithErrOutput(&errOut),
		cli.WithVerbose(true),
	)

	e.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID: "1",
		Iteration:      0,
		HasMore:        true,
		ToolsInIteration: []ports.ToolSummary{
			{Name: "search_web", Status: "ok", DurationMs: 500},
		},
	})

	if out.Len() > 0 {
		t.Errorf("stdout deveria estar vazio, obteve %q", out.String())
	}
	want := "[segment] iteração 1 concluída, 1 tool\n"
	if got := errOut.String(); got != want {
		t.Errorf("stderr esperado %q, obteve %q", want, got)
	}
}

func TestEmitterAdapter_DoneOutputLimitSempreAvisa(t *testing.T) {
	var out, errOut bytes.Buffer
	e := cli.NewEmitterAdapter(cli.WithOutput(&out), cli.WithErrOutput(&errOut))

	e.Emit("chat:done", ports.DoneEvent{
		ConversationID: "1",
		Reason:         "output_limit",
	})

	if out.Len() > 0 {
		t.Errorf("stdout deveria estar vazio, obteve %q", out.String())
	}
	if got, want := errOut.String(), "[done] output_limit\n"; got != want {
		t.Errorf("stderr esperado %q, obteve %q", want, got)
	}
}
