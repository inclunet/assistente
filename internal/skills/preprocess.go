package skills

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"text/template"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
	"assistente/internal/configdir"
)

const (
	// DefaultCommandTimeout é o timeout padrão para execução de !commands (5 segundos).
	DefaultCommandTimeout = 5 * time.Second
	// MaxCommandOutputSize é o tamanho máximo da saída de um comando (100KB).
	MaxCommandOutputSize = 100 * 1024
)

// PreprocessCommands processa linhas com prefixo `!` no conteúdo de um skill.
// Cada linha `!command` ou “ !`command` “ é executada como shell command e substituída pelo output.
// Compatível com a spec oficial do Claude Code.
//
// Formatos suportados:
//   - !command → formato simples
//   - !`command` → formato oficial Claude Code (com backticks)
//
// Regras:
//   - Apenas linhas que começam com `!` (opcionalmente com espaço antes) são processadas
//   - O `!` (e backticks opcionais) são removidos; o restante é executado como comando shell
//   - O output substitui a linha inteira
//   - Se o comando falha, a linha é substituída por um comentário de erro
//   - Timeout de 5s por comando por padrão
//
// Segurança (default-deny, ver AEP-0060):
//   - allowedCommands vem de tools.bashCommands.allowed do skill. Se nil ou
//     vazia, NENHUM !command é executado (a linha vira comentário de bloqueio).
//   - Quando há allowlist, o comando inteiro é avaliado por internal/commandpolicy
//     (o mesmo avaliador da tool run_command): comandos compostos (`;`, `&&`,
//     `||`) são decompostos e CADA átomo precisa estar permitido; features de
//     shell (pipe, redirecionamento, substituição de comando, env inline,
//     sintaxe ambígua) forçam confirm — e como não há usuário para confirmar
//     no preprocess, qualquer decisão diferente de approve bloqueia.
func PreprocessCommands(content string, allowedCommands []string) string {
	if content == "" {
		return ""
	}

	policy := buildSkillPolicy(allowedCommands)

	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "!") || len(trimmed) < 2 {
			result = append(result, line)
			continue
		}

		// Extrai o comando (remove o !)
		cmd := strings.TrimSpace(trimmed[1:])
		if cmd == "" {
			result = append(result, line)
			continue
		}

		// Suporta sintaxe com backticks: !`command` → remove backticks
		cmd = stripBackticks(cmd)

		// Verifica se o comando é permitido pela política (default-deny)
		if allowed, reason := evaluateSkillCommand(cmd, policy); !allowed {
			result = append(result, fmt.Sprintf("<!-- command blocked: %s (%s) -->", cmd, reason))
			continue
		}

		// Executa o comando
		output, err := executeCommand(cmd)
		if err != nil {
			result = append(result, fmt.Sprintf("<!-- command failed: %s — %v -->", cmd, err))
			continue
		}

		// Adiciona output (sem trailing newline extra)
		output = strings.TrimRight(output, "\n\r")
		if output != "" {
			result = append(result, output)
		}
	}

	return strings.Join(result, "\n")
}

// stripBackticks remove backticks envolvendo o comando.
// Suporta: `command` → command, “command“ → command
func stripBackticks(cmd string) string {
	// Remove backticks do início e fim
	if len(cmd) >= 2 && cmd[0] == '`' && cmd[len(cmd)-1] == '`' {
		cmd = cmd[1 : len(cmd)-1]
	}
	return strings.TrimSpace(cmd)
}

// buildSkillPolicy monta uma allowlist sintética default-deny a partir da
// lista de executáveis permitidos declarada pelo skill (tools.bashCommands.allowed).
// Cada executável vira uma regra estruturada `approve` com wildcard de cauda
// (qualquer subcomando/args), preservando a semântica anterior de restrição
// por executável (case-insensitive). Retorna nil quando a lista é nil/vazia,
// sinalizando bloqueio total (default-deny).
func buildSkillPolicy(allowedCommands []string) *allowlist.Allowlist {
	rules := make([]allowlist.CommandRule, 0, len(allowedCommands))
	for _, prog := range allowedCommands {
		prog = strings.TrimSpace(prog)
		if prog == "" {
			continue
		}
		rules = append(rules, allowlist.CommandRule{
			Program:     prog,
			Subcommands: []string{"*"},
			Decision:    "approve",
			Description: "permitido por tools.bashCommands do skill",
		})
	}
	if len(rules) == 0 {
		return nil
	}
	return &allowlist.Allowlist{
		Name:          "skill-preprocess",
		CommandRules:  rules,
		DefaultAction: "deny",
	}
}

// evaluateSkillCommand decide se um !command de skill pode ser executado.
//
// Default-deny: policy nil (skill sem allowlist de bash) bloqueia tudo.
// Com policy, o comando inteiro passa por commandpolicy.Evaluate: comandos
// compostos são decompostos e cada átomo é avaliado individualmente; features
// de shell não suportadas (pipe, redirecionamento, substituição de comando,
// heredoc, background, env inline, sintaxe ambígua) resultam em confirm.
// Como o preprocess não tem fluxo de confirmação, só approve executa.
//
// O reason retornado usa exclusivamente EvaluationResult.Reasons (canal safe,
// sem patterns/descrições da allowlist), pois o comentário de bloqueio é
// injetado no system prompt enviado ao LLM.
func evaluateSkillCommand(cmd string, policy *allowlist.Allowlist) (allowed bool, reason string) {
	if policy == nil {
		return false, "skill does not declare allowed bash commands"
	}

	result := commandpolicy.Evaluate(cmd, policy)
	if result.Decision == allowlist.DecisionApprove {
		return true, ""
	}

	if len(result.Reasons) == 0 {
		return false, "not allowed by command policy"
	}
	return false, strings.Join(result.Reasons, "; ")
}

// ProcessTemplate processa o conteúdo de um skill como Go text/template.
// Fornece a função `include` no FuncMap, que permite incluir arquivos
// dinamicamente via configdir.Resolver (resolução multi-diretório).
//
// Uso na SKILL.md:
//
//	{{ include "memory/memory.md" }}
//
// O path usa o formato "namespace/arquivo", onde namespace é o subdiretório
// dentro de .assistente/ (ex: "memory" → .assistente/memory/).
// Se o arquivo não existir, retorna string vazia (silencioso).
// Se o template tiver erro de parse, retorna o conteúdo original (fallback seguro).
//
// data: contexto disponível no template (ex.: perfil ativo, flags de toolcalling, etc.).
// Se nil, o template é executado sem contexto.
func ProcessTemplate(content string, data any) string {
	if content == "" || !strings.Contains(content, "{{") {
		return content
	}

	funcMap := template.FuncMap{
		"include": includeFile,
		"now":     currentTimestamp,
	}

	tmpl, err := template.New("skill").Funcs(funcMap).Parse(content)
	if err != nil {
		log.Printf("[Skills/Template] Parse error (returning original): %v", err)
		return content
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("[Skills/Template] Execute error (returning original): %v", err)
		return content
	}

	return buf.String()
}

func currentTimestamp() string {
	return time.Now().Format("2006-01-02 15:04 (Monday)")
}

// includeFile lê um arquivo via configdir.Resolver e retorna seu conteúdo.
// O path segue o formato "namespace/arquivo" (ex: "memory/memory.md").
// O primeiro segmento é o subdiretório dentro de .assistente/.
func includeFile(path string) string {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		log.Printf("[Skills/Template] include: invalid path %q (expected 'namespace/file')", path)
		return ""
	}

	namespace := parts[0]
	filename := parts[1]

	resolver := configdir.NewResolver(namespace)
	data, resolved, err := resolver.Read(filename)
	if err != nil {
		log.Printf("[Skills/Template] include %q: not found (searched: %v)", path, resolver.GetSearchPaths())
		return ""
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		log.Printf("[Skills/Template] include %q: file empty (%s)", path, resolved.Path)
		return ""
	}

	if len(content) > MaxCommandOutputSize {
		content = content[:MaxCommandOutputSize] + "\n<!-- include truncated -->"
	}

	log.Printf("[Skills/Template] include %q: loaded %d bytes from %s", path, len(content), resolved.Path)
	return content
}

// executeCommand executa um comando shell com timeout.
func executeCommand(cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
	defer cancel()

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout after %v", DefaultCommandTimeout)
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("exit %v: %s", err, errMsg)
		}
		return "", err
	}

	output := stdout.String()

	// Limita o tamanho do output
	if len(output) > MaxCommandOutputSize {
		output = output[:MaxCommandOutputSize] + "\n<!-- output truncated -->"
	}

	return output, nil
}
