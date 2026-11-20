package terminal

import (
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
	// Antes do PowerShell 7, parênteses podem restaurar $? para true. Só
	// atribuímos LASTEXITCODE ao grupo se o comando final for um executável.
	success := fmt.Sprintf("Write-Host '%s0'", m.endTag)
	if strings.HasSuffix(strings.TrimRight(strings.TrimSuffix(original, powershellDirectoryCatch), "; \r\n\t"), ")") {
		const grouped = `& {
			$line = $MyInvocation.Line;
			$start = $line.IndexOf('%s') + %d;
			$end = $line.IndexOf('%s', $start);
			if ($end -le $start) { return $false };
			$tokens = $null;
			$errors = $null;
			$ast = [System.Management.Automation.Language.Parser]::ParseInput($line.Substring($start, $end - $start), [ref]$tokens, [ref]$errors);
			$last = $ast.EndBlock.Statements[-1];
			if ($last -is [System.Management.Automation.Language.TryStatementAst]) { $last = $last.Body.Statements[-1] };
			if ($last.PipelineElements.Count -ne 1 -or $last.PipelineElements[0].Expression -isnot [System.Management.Automation.Language.ParenExpressionAst]) { return $false };
			while ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -eq 1 -and $last.PipelineElements[0].Expression -is [System.Management.Automation.Language.ParenExpressionAst]) { $last = $last.PipelineElements[0].Expression.Pipeline };
			if ($last.PipelineElements.Count -ne 1) { return $false };
			$cmd = $last.PipelineElements[0];
			if ($cmd -isnot [System.Management.Automation.Language.CommandAst]) { return $false };
			$name = $cmd.GetCommandName();
			if (!$name -and $cmd.CommandElements[0] -is [System.Management.Automation.Language.VariableExpressionAst]) {
				$value = $ExecutionContext.SessionState.PSVariable.GetValue($cmd.CommandElements[0].VariablePath.UserPath);
				if ($value -is [System.Management.Automation.CommandInfo]) { return $value.CommandType -eq 'Application' };
				if ($value -is [string]) { $name = $value }
			};
			return ($name -and $ExecutionContext.InvokeCommand.GetCommand($name, 'All').CommandType -eq 'Application')
		}`
		groupBoundary := "<#group-" + m.id + "#>"
		command += " " + groupBoundary
		group := fmt.Sprintf(singlePowerShellLine(grouped), m.startTag, len(m.startTag)+3, groupBoundary)
		success = fmt.Sprintf("if ($LASTEXITCODE -and (%s)) { Write-Host ('%s' + $LASTEXITCODE) } else { %s }", group, m.endTag, success)
	}
	return fmt.Sprintf("Write-Host '%s'; %s; if ($?) { %s } elseif ($LASTEXITCODE) { Write-Host ('%s' + $LASTEXITCODE) } elseif ($LASTEXITCODE -eq 0 -and (%s)) { Write-Host '%s0' } else { Write-Host '%s1' }", m.startTag, command, success, m.endTag, check, m.endTag, m.endTag)
}

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
