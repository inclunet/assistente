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

	// Content chega acumulado como no BaseStreamHandler real
	e.Emit("chat:stream", ports.StreamEvent{Content: "Olá"})
	e.Emit("chat:stream", ports.StreamEvent{Content: "Olá mundo"})
	e.Emit("chat:stream", ports.StreamEvent{Done: true})

	if got := out.String(); got != "Olá mundo\n" {
		t.Errorf("saída esperada %q, obteve %q", "Olá mundo\n", got)
	}
	if errOut.Len() > 0 {
		t.Errorf("stderr deveria estar vazio, obteve %q", errOut.String())
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
		ConversationID: 1,
		Iteration:      1,
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
		ConversationID: 1,
		Iteration:      2,
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
		ConversationID: 1,
		Iteration:      1,
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
		ConversationID: 1,
		Iteration:      1,
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
