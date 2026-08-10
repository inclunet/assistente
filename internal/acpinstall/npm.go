package acpinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
	"assistente/internal/osutil"
)

// npmOutputLimit é quanto da saída do npm é guardado para a mensagem de erro.
// O npm é verboso quando falha, e o que interessa está no fim; um despejo
// inteiro viraria anúncio de leitor de telas recitando log.
const npmOutputLimit = 4 << 10

// nodeNPM roda o npm pelo `node`, com o `npm-cli.js` como script.
//
// Não é preciosismo: no Windows o `npm` do PATH é `npm.cmd`, e o app não cria
// processo a partir de arquivo de lote (AEP-0084 D15). Passar por um intérprete
// deixaria o npm como processo neto, e cancelar a instalação mataria o
// intérprete enquanto o npm continuaria escrevendo no disco — exatamente o
// resíduo que o D13 manda não deixar.
type nodeNPM struct {
	runtime acp.NodeRuntime
}

// NewNPM monta o executor do npm a partir do runtime encontrado na máquina.
func NewNPM(runtime acp.NodeRuntime) NPM {
	return nodeNPM{runtime: runtime}
}

// lazyNPM procura o runtime no momento do uso, e não na montagem do instalador.
// Quem instalou o Node depois de abrir o app não deveria ter de reabri-lo para o
// catálogo passar a oferecer instalação.
type lazyNPM struct {
	lookup func() acp.NodeRuntime
}

func (l lazyNPM) Install(ctx context.Context, prefix, spec string) error {
	return NewNPM(l.lookup()).Install(ctx, prefix, spec)
}

func (l lazyNPM) Describe(prefix, spec string) string {
	return NewNPM(l.lookup()).Describe(prefix, spec)
}

// Install roda `npm install --prefix <prefix> <spec>` (D6).
//
// Uma vez, no momento pedido, e não `npx` a cada execução: spawnar `npx` no
// `session/new` faria a abertura de uma conversa depender do registro npm estar
// de pé, e o primeiro turno pagaria o download inteiro.
func (n nodeNPM) Install(ctx context.Context, prefix, spec string) error {
	command, args, ok := n.command(prefix, spec)
	if !ok {
		return ErrNoNPM
	}

	cmd := exec.CommandContext(ctx, command, args...)
	// O diretório de trabalho é o próprio prefixo: rodar o npm de dentro do
	// diretório do app impede que um `package.json` de algum diretório acima
	// entre na conta.
	cmd.Dir = prefix
	// Sem isso, no Windows o npm abre uma janela de console que rouba o foco e
	// faz o leitor de telas anunciar o caminho do executável.
	osutil.HideConsoleWindow(cmd)

	var output bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &output, limit: npmOutputLimit}
	cmd.Stderr = cmd.Stdout

	logging.Infof(ctx, component, "instalando o pacote npm %s em %s", spec, prefix)
	err := cmd.Run()
	if err == nil {
		return nil
	}
	// Cancelamento não é falha do npm, e dizer que o npm falhou faria a tela
	// pedir para conferir proxy quando quem interrompeu foi a pessoa.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	// O erro do npm é repassado, e não traduzido em algo genérico: Node velho
	// demais para o pacote, proxy corporativo e registro npm privado são
	// problemas da máquina, e quem vai resolvê-los precisa da mensagem original.
	if detail := acp.SanitizeLabel(strings.TrimSpace(output.String())); detail != "" {
		return fmt.Errorf("o npm não instalou %s: %w — %s", spec, err, detail)
	}
	return fmt.Errorf("o npm não instalou %s: %w", spec, err)
}

// Describe é a linha de comando que Install vai executar, para o diálogo de
// confirmação mostrar o que será executado antes de qualquer byte ser baixado
// (D3).
func (n nodeNPM) Describe(prefix, spec string) string {
	command, args, ok := n.command(prefix, spec)
	if !ok {
		return ""
	}
	return quoteCommandLine(append([]string{command}, args...))
}

// command monta o comando do npm. Os argumentos extras não mudam o que é
// instalado: eles calam a auditoria e o pedido de doação, que são texto que só
// serviria para poluir a mensagem de erro de uma instalação que falhou.
func (n nodeNPM) command(prefix, spec string) (string, []string, bool) {
	command, prefixArgs, ok := n.runtime.NPMCommand()
	if !ok || prefix == "" || spec == "" {
		return "", nil, false
	}
	args := append([]string{}, prefixArgs...)
	args = append(args, "install", "--prefix", prefix, spec, "--no-audit", "--no-fund")
	return command, args, true
}

// quoteCommandLine escreve a linha de comando de um jeito que dá para ler e
// copiar. Caminho com espaço é o caso comum no Windows (`C:\Program Files\...`).
func quoteCommandLine(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.ContainsAny(part, " \t") {
			part = `"` + part + `"`
		}
		quoted = append(quoted, part)
	}
	return strings.Join(quoted, " ")
}

// limitedWriter guarda os últimos limit bytes e descarta o começo. O npm gasta
// as primeiras linhas com progresso e diz o que deu errado no fim, então é o fim
// que serve para quem vai resolver. O processo não pode travar por causa de um
// cano que ninguém mais lê — o excesso é contado como escrito.
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf == nil || w.limit <= 0 {
		return len(p), nil
	}
	escrito := len(p)
	if len(p) > w.limit {
		p = p[len(p)-w.limit:]
	}
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	if excedente := w.buf.Len() - w.limit; excedente > 0 {
		w.buf.Next(excedente)
	}
	w.buf.Next(bytesDeContinuacao(w.buf.Bytes()))
	return escrito, nil
}

// bytesDeContinuacao conta os bytes do meio de um caractere que sobraram no
// começo depois do corte. Sem descartá-los, a mensagem abriria com o rabo de uma
// letra acentuada — um losango de interrogação no meio de um anúncio.
func bytesDeContinuacao(b []byte) int {
	n := 0
	for n < len(b) && b[n]&0xC0 == 0x80 {
		n++
	}
	return n
}

// HandshakeUnsupported é o handshake que recusa: ele existe para quem monta o
// instalador sem ter como sondar o agente, e devolve um erro que diz isso em vez
// de deixar a instalação passar sem prova.
func HandshakeUnsupported(_ context.Context, _ string, _ []string, _ map[string]string) error {
	return errors.New("não há como conferir o agente instalado: o serviço de agentes de código não está disponível")
}
