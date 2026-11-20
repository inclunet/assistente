package terminal

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	markerPrefix = "__ASSISTENTE_"
	markerStart  = markerPrefix + "START_"
	markerEnd    = markerPrefix + "END_"
	markerSuffix = "__"
)

// ansiEscape remove sequências ANSI escape do texto.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\x1b\[[\?]?[0-9;]*[a-zA-Z]`)

// CommandMarker gera e parseia markers para detectar início/fim de comandos em PTY.
// Cada execução recebe um UUID único para evitar conflitos.
type CommandMarker struct {
	id        string
	startTag  string
	endTag    string
	inlineEnd bool
}

func newCommandMarkerForShell(shell string) *CommandMarker {
	id := uuid.New().String()[:8] // usa só os primeiros 8 chars para markers mais curtos
	return &CommandMarker{
		inlineEnd: shell == "powershell",
		id:        id,
		startTag:  markerStart + id + markerSuffix,
		endTag:    markerEnd + id + markerSuffix,
	}
}

// WrapCommand envolve um comando com markers de início/fim.
// O shell type determina a sintaxe usada (bash vs powershell).
func (m *CommandMarker) WrapCommand(command, shellType string) string {
	switch shellType {
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return m.wrapPowerShell(command)
	default:
		return m.wrapBash(command)
	}
}

// wrapBash gera comando wrapped para bash/sh/zsh.
func (m *CommandMarker) wrapBash(command string) string {
	// printf em vez de echo para evitar problemas com \n
	// A quebra inicial separa o marker de prompts sem newline (por exemplo sh),
	// inclusive quando o comando foi enviado antes de o shell exibir o prompt.
	return fmt.Sprintf(
		"printf '\\n%%s\\n' '%s'; %s; printf '\\n%%s%%s\\n' '%s' $?",
		m.startTag, command, m.endTag,
	)
}

// wrapPowerShell gera comando wrapped para PowerShell.
func (m *CommandMarker) wrapPowerShell(command string) string {
	// No Windows PowerShell 5.1, stderr redirecionado pode deixar $? falso
	// mesmo com exit 0. Um NativeCommandError anterior não identifica o
	// comando que acabou de terminar. Delimite o trecho entre a posição do
	// erro e o fim do código do usuário e peça ao parser nativo para conferir.
	original := command
	boundary := "<#" + m.id + "#>"
	if body, ok := strings.CutSuffix(command, powershellDirectoryCatch); ok {
		command = body + " " + boundary + powershellDirectoryCatch
	} else {
		command += " " + boundary
	}
	const nativeStderr = `& {
    $e = $Error[0];
    if (!$e -or $e.FullyQualifiedErrorId -notlike 'NativeCommandError*') { return $false };
    $i = $e.InvocationInfo;
    $line = $i.Line;
    if (!$line) { return $false };
    $end = $line.IndexOf('%s');
    $start = $i.OffsetInLine - 1;
    if ($start -lt 0 -or $end -le $start) { return $false };
    $tokens = $null;
    $errors = $null;
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($line.Substring($start, $end - $start), [ref]$tokens, [ref]$errors);
    $stmts = $ast.EndBlock.Statements;
    if ($errors.Count -or $stmts.Count -ne 1) { return $false };
    $stages = $stmts[0].PipelineElements;
    if (!$stages.Count -or $stages[0] -isnot [System.Management.Automation.Language.CommandAst]) { return $false };
    for ($n = 1; $n -lt $stages.Count; $n++) {
        $stage = $stages[$n];
        if ($stage.Extent.Text -ine 'Out-String' -or (Microsoft.PowerShell.Core\Get-Command Out-String).ImplementingType.FullName -ne 'Microsoft.PowerShell.Commands.OutStringCommand') { return $false }
    };
    return $true
}`
	// O ConPTY recebe uma única linha de entrada.
	check := fmt.Sprintf(singlePowerShellLine(nativeStderr), boundary)
	status := fmt.Sprintf("if ($?) { Write-Host '%s0' } elseif ($LASTEXITCODE) { Write-Host ('%s' + $LASTEXITCODE) } elseif ($LASTEXITCODE -eq 0 -and (%s)) { Write-Host '%s0' } else { Write-Host '%s1' }", m.endTag, m.endTag, check, m.endTag, m.endTag)
	// O ConPTY pode colar o eco do comando na primeira saída. Inicie a
	// linha explicitamente para que o parser reconheça o marcador real.
	start := fmt.Sprintf("Write-Host ([Environment]::NewLine + '%s')", m.startTag)
	// Uma linha comentada pode engolir o delimitador do try ou o marcador.
	// Entregue o texto ao parser como dado, sem interpolá-lo na linha do PTY.
	hasHash := strings.Contains(original, "#")
	grouped := strings.HasSuffix(strings.TrimRight(strings.TrimSuffix(original, powershellDirectoryCatch), "; \r\n\t"), ")")
	if hasHash || grouped {
		body := "{" + command + "}"
		tail := ""
		if hasHash {
			prefix := strings.TrimSuffix(original, powershellDirectoryCatch)
			encoded := base64.StdEncoding.EncodeToString([]byte(prefix))
			body = fmt.Sprintf("([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')))", encoded)
			tail = " '" + strings.ReplaceAll(" "+boundary+strings.TrimPrefix(original, prefix), "'", "''") + "'"
		}
		// return pode sair do bloco antes de status; erro de sintaxe pode impedir
		// até a criação do bloco. A moldura externa conclui esses dois caminhos.
		done := "$__asstEnded" + m.id
		return fmt.Sprintf("%s; %s=$false; try { . (%s %s {%s; %s=$true}%s); if (!$?) { if (!%s) { Write-Host '%s1' } } elseif (!%s) { Write-Host '%s0' } } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; Write-Host '%s1' } finally { Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction SilentlyContinue }", start, done, singlePowerShellLine(prepareGroupedPowerShell), body, status, done, tail, done, m.endTag, done, m.endTag, m.endTag, done[1:])
	}
	return fmt.Sprintf("%s; %s; %s", start, command, status)
}

// No Windows PowerShell 5.1, parênteses restauram $? para true mesmo após uma
// falha. No pipeline final independente, tire apenas os parênteses externos de
// agrupamento de um comando. O alvo, os argumentos e as subexpressões ficam
// intactos. O script preparado roda uma vez no escopo original, com a leitura
// de $? logo após a operação. O parser trabalha no texto original para que os
// offsets coincidam mesmo quando o comando foi envolvido no try de diretório.
const prepareGroupedPowerShell = `& {
	param($body, $status, $tail);
	$source = $body.ToString();
	$tokens = $null;
	$errors = $null;
	if ($tail) {
		$null = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
		$comment = $tokens | Microsoft.PowerShell.Core\Where-Object { $_.Kind -eq 'Comment' -and $_.Text.StartsWith('#') -and !$source.Substring($_.Extent.EndOffset).Trim() } | Microsoft.PowerShell.Utility\Select-Object -Last 1;
		if ($comment) { $source = $comment.Text + [Environment]::NewLine + $source.Substring(0, $comment.Extent.StartOffset) };
		$source += $tail
	};
	$ast = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
	$last = $ast.EndBlock.Statements[-1];
	if ($last -is [System.Management.Automation.Language.TryStatementAst]) { $last = $last.Body.Statements[-1] };
	$chars = $source.ToCharArray();
	while ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -eq 1) {
		$element = $last.PipelineElements[0];
		if ($element -isnot [System.Management.Automation.Language.CommandExpressionAst]) { break };
		$group = $element.Expression;
		if ($element.Redirections.Count -or $group -isnot [System.Management.Automation.Language.ParenExpressionAst]) { break };
		$chars[$group.Extent.StartOffset] = ' ';
		$chars[$group.Extent.EndOffset - 1] = ' ';
		$last = $group.Pipeline
	};
	if ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) { $source = -join $chars };
	return [scriptblock]::Create($source + '; ' + $status.ToString())
}`

// Preserva espaços dentro de literais do script ao remover o recuo das linhas.
func singlePowerShellLine(script string) string {
	lines := strings.Split(script, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, " ")
}

// ParseResult representa o resultado do parsing de output com markers.
type ParseResult struct {
	// Output é o conteúdo entre os markers (o output real do comando)
	Output string

	// ExitCode é o código de saída extraído do end marker
	ExitCode int

	// Found indica se os markers foram encontrados no output
	Found bool

	// Raw é o output completo não processado
	Raw string
}

// ParseOutput extrai o output e exit code do texto bruto do PTY.
// O parser busca markers apenas no início de linhas para ignorar o echo
// da linha de comando que o PTY produz (onde os markers aparecem embutidos
// dentro do texto do comando, não em linhas próprias).
func (m *CommandMarker) ParseOutput(raw string) ParseResult {
	result := ParseResult{Raw: raw}

	// Remove ANSI escapes para facilitar o parsing
	cleaned := StripANSI(raw)

	// Normaliza line endings (Windows PTY usa \r\n)
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

	// Encontra o start marker no INÍCIO de uma linha (após \n ou no início do texto).
	// Isso ignora ocorrências dentro do echo da linha de comando.
	startIdx := -1
	nlStart := strings.Index(cleaned, "\n"+m.startTag)
	if nlStart != -1 {
		startIdx = nlStart + 1 // pula o \n
	} else if strings.HasPrefix(cleaned, m.startTag) {
		startIdx = 0
	}
	if startIdx == -1 {
		return result
	}

	// Pula o start marker + newline
	contentStart := startIdx + len(m.startTag)
	if contentStart < len(cleaned) && cleaned[contentStart] == '\n' {
		contentStart++
	}

	// Encontra o end marker no início de uma linha (última ocorrência).
	endIdx := -1
	nlEnd := strings.LastIndex(cleaned, "\n"+m.endTag)
	if nlEnd != -1 {
		endIdx = nlEnd + 1 // pula o \n
	} else if strings.HasPrefix(cleaned, m.endTag) {
		endIdx = 0
	}
	if m.inlineEnd {
		// Só o marcador desta execução depois do start real pode fechar o comando.
		// A linha de status completa, validada abaixo, distingue o eco e leituras parciais.
		endIdx = strings.LastIndex(cleaned, m.endTag)
	}
	if endIdx == -1 || endIdx < contentStart {
		return result
	}

	// O PTY pode dividir o próprio código de saída entre duas leituras.
	status, _, complete := strings.Cut(cleaned[endIdx+len(m.endTag):], "\n")
	if !complete {
		return result
	}
	code, err := strconv.Atoi(strings.TrimSpace(status))
	if err != nil {
		return result
	}
	result.ExitCode = code
	result.Found = true

	// Extrai o output entre os markers
	if contentStart < endIdx {
		result.Output = cleaned[contentStart:endIdx]
		// Remove trailing newline
		result.Output = strings.TrimRight(result.Output, "\n")
	}

	return result
}

// StartTag retorna o start marker para detecção durante streaming.
func (m *CommandMarker) StartTag() string {
	return m.startTag
}

// EndTag retorna o end marker para detecção durante streaming.
func (m *CommandMarker) EndTag() string {
	return m.endTag
}

// StripANSI remove todas as sequências ANSI escape de uma string.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
