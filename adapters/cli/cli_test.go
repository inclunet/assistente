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

	e.Emit("chat:stream", ports.StreamEvent{Content: "Olá"})
	e.Emit("chat:stream", ports.StreamEvent{Content: " mundo"})
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
