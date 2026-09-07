package acpinstall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
	"assistente/internal/osutil"
)

// uvOutputLimit é quanto da saída do uv é guardado para a mensagem de erro —
// o mesmo teto do npm, e pelo mesmo motivo.
const uvOutputLimit = npmOutputLimit

// realUV roda o `uv` encontrado na máquina: cria o venv e instala o pacote
// pinado nele (D6). Duas etapas, e não `uvx` a cada turno: spawnar `uvx` no
// `session/new` faria a abertura de uma conversa depender do PyPI estar de pé.
type realUV struct {
	runtime acp.UVRuntime
}

// NewUV monta o executor do uv a partir do runtime encontrado na máquina.
func NewUV(runtime acp.UVRuntime) UV {
	return realUV{runtime: runtime}
}

// lazyUV procura o runtime no momento do uso, e não na montagem do instalador.
// Quem instalou o uv depois de abrir o app não deveria ter de reabri-lo para o
// catálogo passar a oferecer instalação.
type lazyUV struct {
	lookup func() acp.UVRuntime
}

func (l lazyUV) Install(ctx context.Context, dir, spec string) error {
	return NewUV(l.lookup()).Install(ctx, dir, spec)
}

func (l lazyUV) Describe(dir, spec string) string {
	return NewUV(l.lookup()).Describe(dir, spec)
}

// Install roda `uv venv` e `uv pip install --python <dir> <spec>` (D6).
func (u realUV) Install(ctx context.Context, dir, spec string) error {
	if !u.runtime.Found || u.runtime.UV == "" {
		return ErrNoUV
	}
	if dir == "" || spec == "" {
		return ErrNoUV
	}

	if err := u.run(ctx, dir, "venv", dir); err != nil {
		return err
	}
	return u.run(ctx, dir, "pip", "install", "--python", dir, spec)
}

// Describe é a linha de comando que Install vai executar, para o diálogo de
// confirmação mostrar o que será executado antes de qualquer byte ser baixado
// (D3). As duas etapas aparecem juntas: é o que de fato roda.
func (u realUV) Describe(dir, spec string) string {
	if !u.runtime.Found || u.runtime.UV == "" || dir == "" || spec == "" {
		return ""
	}
	venv := quoteCommandLine([]string{u.runtime.UV, "venv", dir})
	pip := quoteCommandLine([]string{u.runtime.UV, "pip", "install", "--python", dir, spec})
	return venv + " && " + pip
}

func (u realUV) run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, u.runtime.UV, args...)
	cmd.Dir = dir
	osutil.HideConsoleWindow(cmd)

	var output bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &output, limit: uvOutputLimit}
	cmd.Stderr = cmd.Stdout

	logging.Infof(ctx, component, "rodando uv %s", strings.Join(args, " "))
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if detail := acp.SanitizeLabel(strings.TrimSpace(output.String())); detail != "" {
		return fmt.Errorf("o uv não concluiu %s: %w — %s", strings.Join(args, " "), err, detail)
	}
	return fmt.Errorf("o uv não concluiu %s: %w", strings.Join(args, " "), err)
}
